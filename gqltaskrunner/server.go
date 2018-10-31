package gqltaskrunner

import (
	"context"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/buildkite/terminal"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/thunder/graphql"
	"github.com/samsarahq/thunder/graphql/introspection"
	"github.com/samsarahq/thunder/graphql/schemabuilder"
	"github.com/samsarahq/thunder/reactive"
)

type Server struct {
	Executor    *taskrunner.Executor
	Tasks       []*taskrunner.TaskHandler
	Broadcaster *Broadcaster
}

func NewServer(executor *taskrunner.Executor) *Server {
	return &Server{
		Executor: executor,
	}
}

func NewBroadcaster(events <-chan taskrunner.ExecutorEvent) *Broadcaster {
	return &Broadcaster{
		events:    events,
		resources: make(map[*taskrunner.Task]*reactive.Resource),
	}
}

type Broadcaster struct {
	mu        sync.Mutex
	events    <-chan taskrunner.ExecutorEvent
	resources map[*taskrunner.Task]*reactive.Resource
}

func (b *Broadcaster) Run() {
	go func() {
		for event := range b.events {
			name := event.TaskHandler().Definition()
			func() {
				b.mu.Lock()
				defer b.mu.Unlock()
				if b.resources[name] == nil {
					b.resources[name] = reactive.NewResource()
				}
			}()
			b.resources[name].Strobe()
		}
	}()
}

func (b *Broadcaster) ResourceFor(task *taskrunner.Task) *reactive.Resource {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.resources[task] == nil {
		b.resources[task] = reactive.NewResource()
	}
	return b.resources[task]
}

func (s *Server) SchemaBuilderSchema() *schemabuilder.Schema {
	schema := schemabuilder.NewSchema()

	s.registerQuery(schema)
	s.registerMutation(schema)
	s.registerTaskHandler(schema)

	return schema
}

func (s *Server) registerQuery(schema *schemabuilder.Schema) {
	object := schema.Query()

	object.FieldFunc("tasks", func() []*taskrunner.TaskHandler {
		return s.Executor.Tasks()
	})
}

func (s *Server) registerMutation(schema *schemabuilder.Schema) {
	object := schema.Mutation()

	object.FieldFunc("restartTask", func(args struct{ Name string }) {
		var task *taskrunner.Task
		for _, handler := range s.Executor.Tasks() {
			if handler.Definition().Name == args.Name {
				task = handler.Definition()
			}
		}

		s.Executor.Invalidate(task, taskrunner.UserRestart{})
	})
}

func (s *Server) registerTaskHandler(schema *schemabuilder.Schema) {
	object := schema.Object("TaskHandler", taskrunner.TaskHandler{})

	object.FieldFunc("name", func(task *taskrunner.TaskHandler) string {
		return task.Definition().Name
	})

	object.FieldFunc("state", func(ctx context.Context, task *taskrunner.TaskHandler) taskrunner.TaskHandlerExecutionState {
		reactive.AddDependency(ctx, s.Broadcaster.ResourceFor(task.Definition()), nil)

		return task.State()
	})

	object.FieldFunc("logs", func(ctx context.Context, task *taskrunner.TaskHandler, args struct {
		Html *bool
	}) string {
		reactive.AddDependency(ctx, task.LiveLogger().Resource, nil)

		if args.Html != nil && *args.Html {
			return string(terminal.Render(task.LiveLogger().Logs.Bytes()))
		}
		return task.LiveLogger().Logs.String()
	})
}

func (s *Server) Schema() *graphql.Schema {
	return s.SchemaBuilderSchema().MustBuild()
}

func (server *Server) Run(ctx context.Context) error {
	graphqlSchema := server.Schema()
	introspection.AddIntrospectionToSchema(graphqlSchema)

	router := mux.NewRouter()
	router.Handle("/graphql", graphql.Handler(graphqlSchema))
	httpServer := http.Server{
		Addr:    ":3031",
		Handler: router,
	}

	events, done := server.Executor.Subscribe()
	defer done()

	server.Broadcaster = NewBroadcaster(events)
	server.Broadcaster.Run()

	go func() {
		select {
		case <-ctx.Done():
			httpServer.Close()
		}
	}()

	if err := httpServer.ListenAndServe(); err != nil {
		if ctx.Err() != context.Canceled {
			return err
		}
	}

	return nil
}

func (s *Server) handler(schema *graphql.Schema) http.Handler {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		socket, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("upgrader.Upgrade: %v", err)
			return
		}
		defer socket.Close()

		graphql.CreateConnection(r.Context(), socket, schema, graphql.WithMinRerunInterval(time.Millisecond*10))
	})
}

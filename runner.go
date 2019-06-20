package taskrunner

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/samsarahq/go/oops"
	"golang.org/x/sync/errgroup"
)

var (
	configFile     string
	nonInteractive bool
	listTasks      bool
	watch          bool
	describeTasks  bool
)

// Runtime represents the external interface of an Executor's runtime. It is how taskrunner
// extensions can register themselves to taskrunner's lifecycle.
type Runtime struct {
	subscriptions   []func(events <-chan ExecutorEvent) error
	onStartHooks    []func(ctx context.Context, executor *Executor) error
	onStopHooks     []func(ctx context.Context, executor *Executor) error
	executorOptions []ExecutorOption

	registry *Registry
	flags    *flag.FlagSet
}

func newRuntime() *Runtime {
	r := &Runtime{
		registry: DefaultRegistry,
		flags:    flag.NewFlagSet("taskrunner", 0),
	}

	r.flags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: taskrunner [task...]\n")
		r.flags.PrintDefaults()
	}
	r.flags.StringVar(&configFile, "config", "", "Configuration file to use")
	r.flags.BoolVar(&nonInteractive, "non-interactive", false, "Non-interactive mode (only applies when running the default set of tasks)")
	r.flags.BoolVar(&listTasks, "list", false, "List all tasks")
	r.flags.BoolVar(&watch, "watch", false, "Run in watch mode (only applies when passing custom tasks)")
	r.flags.BoolVar(&describeTasks, "describe", false, "Describe all tasks")
	r.flags.BoolVar(&describeTasks, "desc", false, "Shorthand for -describe")

	return r
}

// OnStart is run after taskrunner has built up its task execution list.
func (r *Runtime) OnStart(f func(context.Context, *Executor) error) {
	r.onStartHooks = append(r.onStartHooks, f)
}

// Subscribe provides an events stream channel that is populated after taskrunner has loaded. The
// channel is read-only and is automatically closed on exit.
func (r *Runtime) Subscribe(f func(events <-chan ExecutorEvent) error) {
	r.subscriptions = append(r.subscriptions, f)
}

// OnStop is before taskrunner exits (either because its tasks have exited or because of a SIGINT).
func (r *Runtime) OnStop(f func(context.Context, *Executor) error) {
	r.onStopHooks = append(r.onStopHooks, f)
}

// WithFlag allows for external registration of flags.
func (r *Runtime) WithFlag(f func(flags *flag.FlagSet)) { f(r.flags) }

type RunOption func(options *Runtime)

func ExecutorOptions(opts ...ExecutorOption) RunOption {
	return func(r *Runtime) {
		r.executorOptions = append(r.executorOptions, opts...)
	}
}

func Run(options ...RunOption) {
	runtime := newRuntime()
	for _, option := range options {
		option(runtime)
	}

	if err := runtime.flags.Parse(os.Args[1:]); err != nil {
		return
	}

	var config *Config
	var err error
	if configFile == "" {
		config, err = ReadUserConfig()
	} else {
		config, err = ReadConfig(configFile)
	}

	if err != nil {
		log.Fatalf("config error: unable to read config:\n%v\n", err)
	}

	tasks := runtime.registry.Tasks()

	if listTasks {
		outputString := "Run specified tasks with `taskrunner taskname1 taskname2`\nTasks available:"
		for _, task := range tasks {
			outputString = outputString + "\n\t" + task.Name
		}
		fmt.Println(outputString)
		return
	}

	if describeTasks {
		var outputString string
		for _, task := range tasks {
			outputString = fmt.Sprintf("%s\n\t%s\t %s", outputString, task.Name, task.Description)
		}
		fmt.Println(outputString)
		return
	}

	log.Println("Using config", config.ConfigFilePath())
	executor := NewExecutor(config, tasks, runtime.executorOptions...)

	var desiredTasks []string
	if len(runtime.flags.Args()) == 0 {
		desiredTasks = config.DesiredTasks
		config.Watch = !nonInteractive
	} else {
		desiredTasks = runtime.flags.Args()
		config.Watch = watch
	}

	if len(tasks) == 0 {
		log.Fatalln("No tasks specified")
	}
	log.Println("Desired tasks:", strings.Join(desiredTasks, ", "))
	log.Printf("Watch mode: %t", config.Watch)

	ctx, cancel := context.WithCancel(context.Background())
	onInterruptSignal(cancel)

	g, ctx := errgroup.WithContext(ctx)
	for i := range runtime.subscriptions {
		sub := runtime.subscriptions[i]
		ch := executor.Subscribe()
		g.Go(func() error {
			return sub(ch)
		})
	}

	g.Go(func() error {
		err := executor.Run(ctx, desiredTasks, runtime)

		// We only care about propagating errors up to the errgroup
		// if it's a well-known executor error, or the underlying task failed AND
		// we're not in watch mode.
		if oops.Cause(err) == errUndefinedTaskName || !config.Watch {
			return err
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		log.Fatalf("run error:\n%v\n", err)
	}
}

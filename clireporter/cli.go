package clireporter

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/samsarahq/taskrunner"
)

type cli struct {
	Executor *taskrunner.Executor
}

func StdoutOption(r *taskrunner.Runtime) {
	option(newLogger(os.Stdout))(r)
}

func FileAppendOption(r *taskrunner.Runtime) {
	if err := os.MkdirAll("./trlogs", 0755); err != nil {
		log.Fatal(err)
	}

	f, err := os.OpenFile("./trlogs/taskrunner.log", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		log.Fatal(err)
	}

	logger := newLogger(f)
	logger.SetSeparator("|")

	option(logger)(r)
}

func FileByDateOption(r *taskrunner.Runtime) {
	if err := os.MkdirAll("./trlogs", 0755); err != nil {
		log.Fatal(err)
	}

	path := fmt.Sprintf("./trlogs/%s-%s.log", "taskrunner", time.Now().UTC().Format(time.RFC3339))
	f, err := os.OpenFile(path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		log.Fatal(err)
	}

	logger := newLogger(f)
	logger.SetSeparator("|")

	option(logger)(r)
}

func option(logger *logger) func(*taskrunner.Runtime) {
	return func(r *taskrunner.Runtime) {
		r.OnStart(func(ctx context.Context, executor *taskrunner.Executor) error {
			logger.registerTasks(executor.Tasks())
			return nil
		})

		r.Subscribe(func(events <-chan taskrunner.ExecutorEvent) error {
			for event := range events {
				var task *taskrunner.Task
				if handler := event.TaskHandler(); handler != nil {
					task = handler.Definition()
				}
				switch event := event.(type) {
				case *taskrunner.TaskLogEvent:
					logger.Write(task, event.Message)
				case *taskrunner.TaskInvalidatedEvent:
					logger.Writef(task, "Invalidating %s for %d reasons:", event.TaskHandler().Definition().Name, len(event.Reasons))
					for _, reason := range event.Reasons {
						logger.Write(task, fmt.Sprintf("- %s", reason.Description()))
					}
				case *taskrunner.TaskStartedEvent:
					logger.Write(task, "Started")
				case *taskrunner.TaskCompletedEvent:
					logger.Writef(task, "Completed (%0.2fs)", float64(event.Duration)/float64(time.Second))
				case *taskrunner.TaskFailedEvent:
					logger.Writef(task, "Failed\n%v", event.Error)
				case *taskrunner.TaskDiagnosticEvent:
					logger.Writef(task, "Warning: %s", event.Error.Error())
				case *taskrunner.TaskStoppedEvent:
					logger.Write(task, "Stopped")
				}
			}

			return nil
		})
	}
}

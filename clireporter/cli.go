package clireporter

import (
	"fmt"
	"time"

	"github.com/samsarahq/taskrunner"
)

type cli struct {
	Executor *taskrunner.Executor
}

func Option(r *taskrunner.Runtime) {
	r.Subscribe(func(events <-chan taskrunner.ExecutorEvent) error {
		for event := range events {
			switch event := event.(type) {
			case *taskrunner.TaskInvalidatedEvent:
				fmt.Fprintf(event.TaskHandler().LogStdout(), "Invalidating %s for %d reasons:", event.TaskHandler().Definition().Name, len(event.Reasons))
				for _, reason := range event.Reasons {
					fmt.Fprintf(event.TaskHandler().LogStdout(), "- %s", reason.Description())
				}
			case *taskrunner.TaskStartedEvent:
				fmt.Fprintf(event.TaskHandler().LogStdout(), "Started")
			case *taskrunner.TaskCompletedEvent:
				fmt.Fprintf(event.TaskHandler().LogStdout(), "Completed (%0.2fs)", float64(event.Duration)/float64(time.Second))
			case *taskrunner.TaskFailedEvent:
				fmt.Fprintf(event.TaskHandler().LogStdout(), "Failed")
				fmt.Fprintln(event.TaskHandler().LogStdout(), event.Error)
			case *taskrunner.TaskDiagnosticEvent:
				fmt.Fprintf(event.TaskHandler().LogStdout(), "Warning: %s", event.Error.Error())
			case *taskrunner.TaskStoppedEvent:
				fmt.Fprintf(event.TaskHandler().LogStdout(), "Stopped")
			}
		}

		return nil
	})
}

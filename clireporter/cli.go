package clireporter

import (
	"context"
	"fmt"
	"time"

	"github.com/samsarahq/taskrunner"
)

var verbose bool

type cli struct {
	Executor *taskrunner.Executor
}

func init() {
	taskrunner.Flags.BoolVar(&verbose, "v", false, "Show verbose output")
}

func Option(r *taskrunner.RunOptions) {
	r.ReporterFns = append(r.ReporterFns, func(ctx context.Context, executor *taskrunner.Executor) error {
		New(executor).Run(ctx)
		return nil
	})
}

func New(executor *taskrunner.Executor) *cli {
	return &cli{
		Executor: executor,
	}
}

func (c *cli) Run(ctx context.Context) {
	events, done := c.Executor.Subscribe()
	go func() {
		<-ctx.Done()
		done()
	}()

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
		case *taskrunner.TaskRunShellEvent:
			if verbose {
				fmt.Fprintf(event.TaskHandler().LogStdout(), "$: %s", event.Message)
			}
		case *taskrunner.TaskStoppedEvent:
			fmt.Fprintf(event.TaskHandler().LogStdout(), "Stopped")
		}
	}
}

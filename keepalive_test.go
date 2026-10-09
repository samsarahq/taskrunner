package taskrunner_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/config"
	"github.com/samsarahq/taskrunner/shell"
	"github.com/stretchr/testify/assert"
)

// A KeepAlive task that exits cleanly must be restarted: a long-running task
// whose process was killed underneath a wrapper (for example, nx reporting
// success for a killed dev server) surfaces to the executor as a successful
// exit, and leaving it done leaves the developer with a dead server.
func TestExecutorKeepAliveRestartsOnCleanExit(t *testing.T) {
	conf := &config.Config{}

	var calls int32
	task := &taskrunner.Task{
		Name: "keepalive",
		Run: func(ctx context.Context, shellRun shell.ShellRun) error {
			atomic.AddInt32(&calls, 1)
			// Exit cleanly immediately: the first exit must already trigger a
			// restart, not only exits after an error.
			return nil
		},
		KeepAlive: true,
	}

	executor := taskrunner.NewExecutor(conf, []*taskrunner.Task{task})
	events := executor.Subscribe()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- executor.Run(ctx, []string{"keepalive"}, &taskrunner.Runtime{})
	}()

	// Wait until the task has restarted at least once, proving the clean exit
	// led to a re-execution.
	deadline := time.After(10 * time.Second)
	for atomic.LoadInt32(&calls) < 2 {
		select {
		case event := <-events:
			if event.Kind() == taskrunner.ExecutorEventKind_TaskCompleted {
				if atomic.LoadInt32(&calls) < 2 {
					continue
				}
			}
		case <-deadline:
			cancel()
			t.Fatalf("expected the KeepAlive task to be restarted after a clean exit, calls so far: %d", atomic.LoadInt32(&calls))
		}
	}

	cancel()
	assert.NoError(t, <-done)
}

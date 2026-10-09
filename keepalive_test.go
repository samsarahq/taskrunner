package taskrunner_test

import (
	"context"
	"errors"
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

// The error-exit restart path predates the clean-exit one; assert it still
// holds so the widened condition cannot regress either branch in isolation.
//
// No executor.Subscribe here: a subscription holds a channel the executor
// publishes into, and cancel() closes those channels while the invalidation
// loop can still be publishing (executor.go publishEvent send on a closed
// channel — an upstream shutdown ordering gap, see the note in the PR). The
// clean-exit test above only drains events before cancelling; polling calls
// keeps this one off that closed-channel window.
func TestExecutorKeepAliveRestartsOnErrorExit(t *testing.T) {
	conf := &config.Config{}

	var calls int32
	task := &taskrunner.Task{
		Name: "keepalive",
		Run: func(ctx context.Context, shellRun shell.ShellRun) error {
			atomic.AddInt32(&calls, 1)
			return errors.New("exit")
		},
		KeepAlive: true,
	}

	executor := taskrunner.NewExecutor(conf, []*taskrunner.Task{task})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() {
		done <- executor.Run(ctx, []string{"keepalive"}, &taskrunner.Runtime{})
	}()

	deadline := time.After(10 * time.Second)
	for atomic.LoadInt32(&calls) < 2 {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("expected the KeepAlive task to be restarted after an error exit, calls so far: %d", atomic.LoadInt32(&calls))
		case <-time.After(50 * time.Millisecond):
		}
	}

	cancel()
	// Run surfaces the task's error even though the task was kept alive.
	assert.Error(t, <-done)
}

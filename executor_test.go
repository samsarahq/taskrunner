package taskrunner_test

import (
	"context"
	"testing"
	"time"

	"github.com/ChenJesse/taskrunner"
	"github.com/ChenJesse/taskrunner/config"
	"github.com/ChenJesse/taskrunner/shell"
	"github.com/stretchr/testify/assert"
)

type mockFn struct {
	Calls []time.Time
}

func (m *mockFn) Fn() {
	m.Calls = append(m.Calls, time.Now())
}

func (m *mockFn) ExpectCalls(t *testing.T, n int) {
	t.Helper()
	assert.Equal(t, n, len(m.Calls), "expected %d calls but received %d", n, len(m.Calls))
}

func (m *mockFn) Reset() {
	m.Calls = nil
}

func TestExecutorSimple(t *testing.T) {
	config := &config.Config{}
	mockA := &mockFn{}
	mockB := &mockFn{}

	taskA := &taskrunner.Task{
		Name: "A",
		Run: func(ctx context.Context, shellRun shell.ShellRun) error {
			mockA.Fn()
			return nil
		},
	}

	taskB := &taskrunner.Task{
		Name: "B",
		Run: func(ctx context.Context, shellRun shell.ShellRun) error {
			mockB.Fn()
			return nil
		},
		Dependencies: []*taskrunner.Task{taskA},
	}

	tasks := []*taskrunner.Task{taskA, taskB}

	ctx := context.Background()
	executor := taskrunner.NewExecutor(config, tasks)

	for _, testcase := range []struct {
		Name string
		Test func(t *testing.T)
	}{
		{
			"single task",
			func(t *testing.T) {
				executor.Run(ctx, []string{"A"}, &taskrunner.Runtime{})
				mockA.ExpectCalls(t, 1)
				mockB.ExpectCalls(t, 0)
			},
		},
		{
			"dependent task",
			func(t *testing.T) {
				ctx = context.Background()
				executor.Run(ctx, []string{"B"}, &taskrunner.Runtime{})

				mockA.ExpectCalls(t, 1)
				mockB.ExpectCalls(t, 1)

				assert.True(t, mockA.Calls[0].UnixNano() < mockB.Calls[0].UnixNano(), "expected B to be called after A")
			},
		},
	} {
		t.Run(testcase.Name, testcase.Test)
		mockA.Reset()
		mockB.Reset()
	}
}

type TestInvalidationEvent struct{}

func (f TestInvalidationEvent) Reason() taskrunner.InvalidationReason {
	return taskrunner.InvalidationReason_Invalid
}

func (f TestInvalidationEvent) Description() string {
	return "the test case decided to invalidate the task"
}

// consumeUntil consumes the events channel until an event matching
// the specified kind appears.
func consumeUntil(t *testing.T, events <-chan taskrunner.ExecutorEvent, kind taskrunner.ExecutorEventKind) taskrunner.ExecutorEvent {
	for event := range events {
		if event.Kind() == kind {
			return event
		}
	}

	t.Fatalf("channel was closed before event was observed")
	return nil
}

func TestExecutorInvalidations(t *testing.T) {
	config := &config.Config{}
	mockA := &mockFn{}
	mockB := &mockFn{}

	taskA := &taskrunner.Task{
		Name: "A",
		Run: func(ctx context.Context, shellRun shell.ShellRun) error {
			mockA.Fn()
			return nil
		},
	}

	taskB := &taskrunner.Task{
		Name: "B",
		Run: func(ctx context.Context, shellRun shell.ShellRun) error {
			mockB.Fn()
			return nil
		},
		Dependencies: []*taskrunner.Task{taskA},
	}

	tasks := []*taskrunner.Task{taskA, taskB}

	for _, testcase := range []struct {
		Name string
		Test func(t *testing.T)
	}{
		{
			"dependency invalidated",
			func(t *testing.T) {
				executor := taskrunner.NewExecutor(config, tasks, taskrunner.WithWatchMode(true))
				ctx, cancel := context.WithCancel(context.Background())
				events := executor.Subscribe()

				go func() {
					assert.Equal(t, taskA, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskA")
					assert.Equal(t, taskB, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskB")
					mockA.ExpectCalls(t, 1)
					mockB.ExpectCalls(t, 1)

					executor.Invalidate(taskA, TestInvalidationEvent{})

					assert.Equal(t, taskA, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskA")
					assert.Equal(t, taskB, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskB")
					mockA.ExpectCalls(t, 2)
					mockB.ExpectCalls(t, 2)

					cancel()
				}()

				executor.Run(ctx, []string{"B"}, &taskrunner.Runtime{})
			},
		},
		{
			"leaf invalidated",
			func(t *testing.T) {
				executor := taskrunner.NewExecutor(config, tasks)
				ctx, cancel := context.WithCancel(context.Background())
				events := executor.Subscribe()

				go func() {
					assert.Equal(t, taskA, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskA")
					assert.Equal(t, taskB, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskB")
					mockA.ExpectCalls(t, 1)
					mockB.ExpectCalls(t, 1)

					executor.Invalidate(taskB, TestInvalidationEvent{})

					assert.Equal(t, taskB, consumeUntil(t, events, taskrunner.ExecutorEventKind_TaskCompleted).TaskHandler().Definition(), "expected first task to be taskB")
					mockA.ExpectCalls(t, 1)
					mockB.ExpectCalls(t, 2)

					cancel()
				}()

				executor.Run(ctx, []string{"B"}, &taskrunner.Runtime{})
			},
		},
	} {
		t.Run(testcase.Name, testcase.Test)
		mockA.Reset()
		mockB.Reset()
	}
}

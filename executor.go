package taskrunner

import (
	"context"
	"sync"
	"time"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/taskrunner/shell"
	"golang.org/x/sync/errgroup"
	"mvdan.cc/sh/interp"
)

// Executor constructs and executes a DAG for the tasks specified and
// desired. It maintains the state of execution for individual tasks,
// accepts invalidation events, and schedules re-executions when
// necessary.
type Executor struct {
	ctx    context.Context
	config *Config

	// tasks is set of desired tasks to be evaluated in the executor DAG.
	tasks taskSet

	// mu locks on executor evaluations, preventing
	// multiple plans or multiple passes from running concurrently.
	mu sync.Mutex

	// wg blocks until the executor has completed all tasks.
	wg errgroup.Group

	// invalidationCh is used to coalesce incoming invalidations into
	// a single event.
	invalidationCh chan struct{}

	// taskRegistry contains all available tasks registered to the executor,
	// whether or not they are desired.
	taskRegistry map[string]*Task

	shellRunOptions []shell.RunOption

	// eventsCh keeps track of subscribers to events for this executor.
	eventsChs []chan ExecutorEvent
}

type ExecutorOption func(*Executor)

func ShellRunOptions(opts ...shell.RunOption) ExecutorOption {
	return func(e *Executor) {
		e.shellRunOptions = append(e.shellRunOptions, opts...)
	}
}

// NewExecutor initializes a new executor.
func NewExecutor(config *Config, tasks []*Task, opts ...ExecutorOption) *Executor {
	executor := &Executor{
		config:         config,
		invalidationCh: make(chan struct{}, 1),
		taskRegistry:   make(map[string]*Task),
	}

	for _, opt := range opts {
		opt(executor)
	}

	for _, task := range tasks {
		executor.taskRegistry[task.Name] = task
	}

	return executor
}

// Config returns the taskrunner configuration.
func (e *Executor) Config() *Config { return e.config }

// Subscribe returns a channel of executor-level events. Each invocation
// of Events() returns a new channel. The done function should be called
// to unregister this channel.
func (e *Executor) Subscribe() (events <-chan ExecutorEvent) {
	ch := make(chan ExecutorEvent, 1024)
	e.eventsChs = append(e.eventsChs, ch)
	return ch
}

func (e *Executor) publishEvent(event ExecutorEvent) {
	for _, ch := range e.eventsChs {
		ch <- event
	}
}

// runInvalidationLoop kicks off a background goroutine that plans and
// runs re-executions after invalidations occur. It coalesces invalidations
// every second.
func (e *Executor) runInvalidationLoop() {
	timer := time.NewTimer(time.Second)
	timer.Stop()

	go func() {
		for {
			select {
			case <-e.invalidationCh:
				timer.Reset(time.Second)

			case <-timer.C:
				e.evaluateInvalidationPlan()
				go e.runPass()
			}
		}
	}()
}

// evaluateInvalidationPlan find all tasks that have pending invalidations
// and kicks off their re-execution.
func (e *Executor) evaluateInvalidationPlan() {
	// Wait for potential side effects from the last evaluation to complete.
	// For instance, if task A depends on task B and task B changes a file that
	// task A needs, then we want to wait for the file events from task B to propagate
	// before evaluating the new plan. We must rely on timing because we cannot
	// follow and wait for the execution to come back through fswatch.
	time.Sleep(time.Millisecond * 1000)
	e.mu.Lock()
	defer e.mu.Unlock()

	var toInvalidate []*taskExecution
	for _, execution := range e.tasks {
		if len(execution.pendingInvalidations) == 0 || execution.state == taskExecutionState_invalid {
			continue
		}

		var reasons []InvalidationEvent
		for reason := range execution.pendingInvalidations {
			reasons = append(reasons, reason)
		}

		e.publishEvent(&TaskInvalidatedEvent{
			simpleEvent: execution.simpleEvent(),
			Reasons:     reasons,
		})

		toInvalidate = append(toInvalidate, execution)
	}

	for _, execution := range toInvalidate {
		execution.invalidate(e.ctx)
	}
}

// Invalidate marks a task and its dependencies as invalidated. If any
// tasks become invalidated from this call, Invalidate() will also
// schedule a re-execution of the DAG.
func (e *Executor) Invalidate(task *Task, event InvalidationEvent) {
	execution := e.tasks[task]
	if didInvalidate := execution.Invalidate(event); !didInvalidate {
		return
	}

	e.invalidationCh <- struct{}{}
}

func (e *Executor) Run(ctx context.Context, taskNames []string, runtime *Runtime) error {
	e.ctx = ctx
	defer func() {
		for _, ch := range e.eventsChs {
			close(ch)
		}
	}()
	e.runInvalidationLoop()
	if e.config.Watch {
		e.runWatch(ctx)
	}

	// Build up the DAG for task executions.
	taskSet := make(taskSet)
	for _, taskName := range taskNames {
		task := e.taskRegistry[taskName]
		if task == nil {
			return oops.Errorf("task %s is not defined", taskName)
		}
		taskSet.add(ctx, task)
	}

	e.tasks = taskSet

	// Run all onStartHooks before starting, after the DAG has been created.
	for _, hook := range runtime.onStartHooks {
		if err := hook(ctx, e); err != nil {
			return err
		}
	}
	e.runPass()

	// Wait on all tasks to exit before stopping.
	if err := e.wg.Wait(); err != nil {
		return err
	}

	// Run all onStopHooks after stopping.
	for _, hook := range runtime.onStopHooks {
		if err := hook(ctx, e); err != nil {
			return err
		}
	}

	return nil
}

func (e *Executor) shellRun(ctx context.Context, command string, opts ...shell.RunOption) error {
	options := []shell.RunOption{
		func(r *interp.Runner) {
			loggerI := ctx.Value(loggerKey{})
			if loggerI == nil {
				return
			}

			logger := loggerI.(*Logger)

			r.Stdout = logger.Stdout
			r.Stderr = logger.Stderr
		},
	}
	options = append(options, e.shellRunOptions...)
	options = append(options, opts...)
	return shell.Run(ctx, command, options...)
}

func (e *Executor) taskExecution(t *Task) *taskExecution { return e.tasks[t] }
func (e *Executor) provideEventLogger(t *Task) *Logger {
	stderr := &eventLogger{
		executor: e,
		task:     t,
		stream:   TaskLogEventStderr,
	}
	stdout := *stderr
	stdout.stream = TaskLogEventStdout
	return &Logger{
		Stderr: stderr,
		Stdout: &stdout,
	}
}

// runPass kicks off tasks that are in an executable state.
func (e *Executor) runPass() {
	e.mu.Lock()
	defer e.mu.Unlock()

	for task, execution := range e.tasks {
		if execution.ShouldExecute() {
			execution.state = taskExecutionState_running

			func(task *Task, execution *taskExecution) {
				e.wg.Go(func() error {
					liveLogger, err := execution.liveLogger.Provider(task)
					if err != nil {
						panic(err)
					}

					logger := MergeLoggers(liveLogger, e.provideEventLogger(task))

					ctx := context.WithValue(execution.ctx, loggerKey{}, logger)

					e.publishEvent(&TaskStartedEvent{
						simpleEvent: execution.simpleEvent(),
					})

					started := time.Now()

					err = task.Run(ctx, e.shellRun)

					if ctx.Err() == context.Canceled {
						e.publishEvent(&TaskStoppedEvent{
							simpleEvent: execution.simpleEvent(),
						})
					} else if err != nil {
						execution.state = taskExecutionState_error
						e.publishEvent(&TaskFailedEvent{
							simpleEvent: execution.simpleEvent(),
							Error:       err,
						})
					} else {
						e.publishEvent(&TaskCompletedEvent{
							simpleEvent: execution.simpleEvent(),
							Duration:    time.Since(started),
						})
						execution.state = taskExecutionState_done
					}

					// It's important that we flush the error/done states before
					// terminating the channel. It's also important that possible
					// invalidations occur after exit so that those channels do not block,
					// waiting for this to complete.
					execution.terminalCh <- struct{}{}

					if task.KeepAlive && execution.state == taskExecutionState_error {
						e.Invalidate(task, KeepAliveStopped{})
					}

					if err == nil {
						e.evaluateInvalidationPlan()
					}

					e.runPass()
					if err != nil && ctx.Err() != context.Canceled {
						return err
					}

					return nil
				})
			}(task, execution)

		}
	}
}

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

	// subscriptionsMu locks on operations with event subscriptions, unsubscriptions,
	// and publishing.
	subscriptionsMu sync.Mutex

	// wg blocks until the executor has completed all tasks.
	wg errgroup.Group

	// invalidationCh is used to coalesce incoming invalidations into
	// a single event.
	invalidationCh chan struct{}

	// taskRegistry contains all available tasks registered to the executor,
	// whether or not they are desired.
	taskRegistry map[string]*Task

	shellRunOptions []shell.RunOption

	// eventsCh keeps track of subscribers to events for this executor. It's access
	// should be guarded by subMu.
	eventsChs map[chan ExecutorEvent]struct{}
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
		eventsChs:      make(map[chan ExecutorEvent]struct{}),
	}

	for _, opt := range opts {
		opt(executor)
	}

	for _, task := range tasks {
		executor.taskRegistry[task.Name] = task
	}

	return executor
}

// Subscribe returns a channel of executor-level events. Each invocation
// of Events() returns a new channel. The done function should be called
// to unregister this channel.
func (e *Executor) Subscribe() (events <-chan ExecutorEvent, done func()) {
	e.subscriptionsMu.Lock()
	defer e.subscriptionsMu.Unlock()

	once := sync.Once{}

	ch := make(chan ExecutorEvent, 1024)
	e.eventsChs[ch] = struct{}{}
	return ch, func() {
		once.Do(func() {
			e.subscriptionsMu.Lock()
			defer e.subscriptionsMu.Unlock()
			close(ch)
			delete(e.eventsChs, ch)
		})
	}
}

func (e *Executor) publishEvent(event ExecutorEvent) {
	e.subscriptionsMu.Lock()
	defer e.subscriptionsMu.Unlock()

	for eventsCh := range e.eventsChs {
		eventsCh <- event
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

func (e *Executor) Run(ctx context.Context, taskNames ...string) error {
	e.ctx = ctx
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

	// Find longest task name length for padding
	var longestTaskNameLength int
	for task := range taskSet {
		length := len(task.Name)
		if length > longestTaskNameLength {
			longestTaskNameLength = length
		}
	}
	SetDefaultPadding(longestTaskNameLength)

	e.tasks = taskSet
	e.runPass()

	// Wait on all tasks to exit before stopping.
	return e.wg.Wait()
}

func (e *Executor) shellRun(simpleEvent func() *simpleEvent) func(ctx context.Context, command string, opts ...shell.RunOption) error {
	return func(ctx context.Context, command string, opts ...shell.RunOption) error {
		e.publishEvent(&TaskRunShellEvent{
			simpleEvent: simpleEvent(),
			Message:     command,
		})

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
					logger, err := e.config.LogProvider()(task)
					if err != nil {
						// Default to stdout/stderr.
						e.publishEvent(&TaskDiagnosticEvent{
							simpleEvent: execution.simpleEvent(),
							Error:       oops.Wrapf(err, "failed to initialize log provider"),
						})
					}

					liveLogger, err := execution.liveLogger.Provider(task)
					if err != nil {
						panic(err)
					}

					logger = MergeLoggers(logger, liveLogger)

					ctx := context.WithValue(execution.ctx, loggerKey{}, logger)

					e.publishEvent(&TaskStartedEvent{
						simpleEvent: execution.simpleEvent(),
					})

					started := time.Now()

					err = task.Run(ctx, e.shellRun(execution.simpleEvent))

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

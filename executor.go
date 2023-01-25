package taskrunner

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/taskrunner/config"
	"github.com/samsarahq/taskrunner/shell"
	"github.com/samsarahq/taskrunner/watcher"
	"go.uber.org/multierr"
	"golang.org/x/sync/errgroup"
	"mvdan.cc/sh/interp"
)

// Executor constructs and executes a DAG for the tasks specified and
// desired. It maintains the state of execution for individual tasks,
// accepts invalidation events, and schedules re-executions when
// necessary.
type Executor struct {
	ctx    context.Context
	config *config.Config

	// tasks is set of desired tasks to be evaluated in the executor DAG.
	tasks taskSet

	// watchMode is whether or not executor.Run should watch for file changes.
	watchMode bool

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

	// taskFlagsRegistry contains all supported flags per task
	// registered to the executor, whether or not they are desired.
	taskFlagsRegistry map[string]map[string]TaskFlag

	// taskFlagArgs contains all desired options grouped by desired tasks
	// passed into CLI.
	taskFlagArgs map[string][]string

	shellRunOptions []shell.RunOption

	// eventsCh keeps track of subscribers to events for this executor.
	eventsChs []chan ExecutorEvent

	// watcherEnhancers are enhancer functions to replace the default watcher.
	watcherEnhancers []WatcherEnhancer
}

// WatcherEnhancer is a function to modify or replace a watcher.
type WatcherEnhancer func(watcher.Watcher) watcher.Watcher

var errUndefinedTaskName = errors.New("undefined task name")

type ExecutorOption func(*Executor)

var flagTypeToGetter = map[string]string{
	StringTypeFlag: "StringVal",
	BoolTypeFlag:   "BoolVal",
	IntTypeFlag:    "IntVal",
}

// WithWatcherEnhancer adds a watcher enhancer to run when creating the enhancer.
func WithWatcherEnhancer(we WatcherEnhancer) ExecutorOption {
	return func(e *Executor) {
		e.watcherEnhancers = append(e.watcherEnhancers, we)
	}
}

// WithWatchMode controls the file watching mode.
func WithWatchMode(watchMode bool) ExecutorOption {
	return func(e *Executor) {
		e.watchMode = watchMode
	}
}

func ShellRunOptions(opts ...shell.RunOption) ExecutorOption {
	return func(e *Executor) {
		e.shellRunOptions = append(e.shellRunOptions, opts...)
	}
}

// NewExecutor initializes a new executor.
func NewExecutor(config *config.Config, tasks []*Task, opts ...ExecutorOption) *Executor {
	executor := &Executor{
		config:            config,
		invalidationCh:    make(chan struct{}, 1),
		taskRegistry:      make(map[string]*Task),
		taskFlagsRegistry: make(map[string]map[string]TaskFlag),
		taskFlagArgs:      make(map[string][]string),
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
func (e *Executor) Config() *config.Config { return e.config }

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

			case <-e.ctx.Done():
				return

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
	if e.watchMode {
		e.runWatch(ctx)
	}

	// Build up the DAG for task executions.
	taskSet := make(taskSet)
	for _, taskName := range taskNames {
		task := e.taskRegistry[taskName]
		if task == nil {
			return oops.Wrapf(errUndefinedTaskName, "task %s is not defined", taskName)
		}
		taskSet.add(ctx, task)
	}

	e.tasks = taskSet

	var errors error
	// Run all onStartHooks before starting, after the DAG has been created.
	for _, hook := range runtime.onStartHooks {
		if err := hook(ctx, e); err != nil {
			errors = multierr.Append(errors, err)
		}
	}
	if errors != nil {
		return errors
	}

	e.runPass()

	// Wait on all tasks to exit before stopping.
	errors = multierr.Append(errors, e.wg.Wait())

	// Run all onStopHooks after stopping.
	for _, hook := range runtime.onStopHooks {
		if err := hook(ctx, e); err != nil {
			errors = multierr.Append(errors, err)
		}
	}

	return errors
}

// ShellRun executes a shell.Run with some default options:
// Commands for tasks are automatically logged (stderr and stdout are forwarded).
// Commands run in a consistent environment (configurable on a taskrunner level).
// Commands run in taskrunner's working directory.
func (e *Executor) ShellRun(ctx context.Context, command string, opts ...shell.RunOption) error {
	options := []shell.RunOption{
		func(r *interp.Runner) {
			logger := LoggerFromContext(ctx)
			if logger == nil {
				return
			}

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

func (e *Executor) parseTaskOptionsListToMap(taskName string, flags []string) map[string]FlagArg {
	taskFlagsMap := make(map[string]FlagArg)

	for _, flag := range flags {
		var key string
		var val string
		splitFlag := strings.Split(flag, "=")

		if len(splitFlag) == 2 {
			// If the passed flag has an "=" in it, assume that it is a variable flag
			// (e.g. --var="val")
			key = splitFlag[0]
			val = splitFlag[1]
		} else if len(splitFlag) == 1 {
			// If the passed flag does not have an "=" in it, assume that it is a bool flag
			// (e.g. --true)
			key = splitFlag[0]
		} else {
			// If the passed flag has >1 "=" in it, this is invalid syntax
			panic(fmt.Sprintf("Invalid flag syntax for %s: `%s`", taskName, flag))
		}

		key, err := e.getVerifiedFlagKey(taskName, splitFlag[0])
		if err != nil {
			panic(fmt.Sprintf("Unsupported flag passed to %s: `%s`. See error: %s", taskName, key, err))
		}

		// At this point, we should have verified that this flag is supported in `getVerifiedFlagKey`
		taskFlag := e.taskFlagsRegistry[taskName][key]
		flagValErrMsg := fmt.Sprintf("The value for this flag is `%s`. Please use `%s`", taskFlag.ValueType, flagTypeToGetter[taskFlag.ValueType])
		flagArg := FlagArg{
			Value: val,
			BoolVal: func() bool {
				if taskFlag.ValueType != "bool" {
					panic(flagValErrMsg)
				}

				return true
			},
			IntVal: func() int {
				if taskFlag.ValueType != "int" {
					panic(flagValErrMsg)
				}

				num, err := strconv.Atoi(val)
				if err != nil {
					// Handle error - val was invalid int
				}
				return num
			},
			StringVal: func() string {
				if taskFlag.ValueType != "string" {
					panic(flagValErrMsg)
				}

				return val
			},
		}
		taskFlagsMap[key] = flagArg

		// Allow readers to index into flag map via either LongName or ShortName
		// regardless of which arg was passed in if both names are available
		if len(key) == 1 && key != taskFlag.LongName && taskFlag.LongName != "" {
			// If only one char was passed through, check whether a LongName is available
			// and also register it in the map
			key = taskFlag.LongName
			taskFlagsMap[key] = flagArg
		} else if len(key) > 1 && key == taskFlag.LongName && taskFlag.ShortName != 0 {
			// If >1 char was passed through, check whether a ShortName is available
			// and also register it in the map
			key = string(taskFlag.ShortName)
			taskFlagsMap[key] = flagArg
		}
	}

	return taskFlagsMap
}

func (e *Executor) getVerifiedFlagKey(taskName string, flagKey string) (string, error) {
	var strippedKey string
	if strings.HasPrefix(flagKey, "--") {
		// If the flag key is prefixed with "--", we expect it to be the flag LongName
		strippedKey = string(flagKey[2:])
		if len(strippedKey) <= 1 {
			return flagKey, errors.New(fmt.Sprintf("Unknown flag: `%s`", strippedKey))
		}
	} else if strings.HasPrefix(flagKey, "-") {
		// If the flag key is prefixed with "-", we expect it to be the flag ShortName
		strippedKey = string(flagKey[1:])
		if len(strippedKey) != 1 {
			return flagKey, errors.New(fmt.Sprintf("Did you mean `--%s` (with two dashes)?", strippedKey))
		}
	} else {
		// If the flag is not prefixed with any dashes, this is invalid syntax
		return flagKey, errors.New(fmt.Sprintf("Options must be specified with either `-` or `--`"))
	}

	// Check that task is valid
	if _, ok := e.taskFlagsRegistry[taskName]; ok {
		// Check that flag is supported
		if _, ok = e.taskFlagsRegistry[taskName][strippedKey]; ok {
			return strippedKey, nil
		}
	}

	return flagKey, errors.New(fmt.Sprintf("Unsupported flag: %s", flagKey))
}

// runPass kicks off tasks that are in an executable state.
func (e *Executor) runPass() {
	if e.ctx.Err() != nil {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	for task, execution := range e.tasks {
		if execution.ShouldExecute() {
			execution.state = taskExecutionState_running

			func(task *Task, execution *taskExecution) {
				e.wg.Go(func() error {
					logger := e.provideEventLogger(task)

					ctx := context.WithValue(execution.ctx, loggerKey{}, logger)

					e.publishEvent(&TaskStartedEvent{
						simpleEvent: execution.simpleEvent(),
					})

					started := time.Now()
					var duration time.Duration
					var err error

					if task.RunWithFlags != nil {
						taskFlagsMap := make(map[string]FlagArg)

						if passedFlags, ok := e.taskFlagArgs[task.Name]; ok {
							taskFlagsMap = e.parseTaskOptionsListToMap(task.Name, passedFlags)
						}
						err = task.RunWithFlags(ctx, e.ShellRun, taskFlagsMap)
						duration = time.Since(started)
					} else if task.Run != nil {
						err = task.Run(ctx, e.ShellRun)
						duration = time.Since(started)
					}

					if ctx.Err() == context.Canceled {
						// Only move ourselves to permanently canceled if taskrunner is shutting down. Note
						// that the invalidation codepath already set the state as invalid, so there is
						// no else statement.
						if e.ctx.Err() != nil {
							execution.state = taskExecutionState_canceled
						}
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
						execution.state = taskExecutionState_done
						e.publishEvent(&TaskCompletedEvent{
							simpleEvent: execution.simpleEvent(),
							Duration:    duration,
						})
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

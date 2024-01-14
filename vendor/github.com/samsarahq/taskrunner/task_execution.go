package taskrunner

import (
	"context"
	"sync"
	"time"
)

type taskExecutionState int

const (
	// taskExecutionState_invalid tasks are not running and are queued to be started
	// on the next invalidation plan.
	taskExecutionState_invalid = iota
	// taskExecutionState_running tasks are currently executing. They may transition to
	// error on failure or done on succesful completion. Tasks may also be canceled.
	taskExecutionState_running
	// taskExecutionState_error tasks were running and failed. They may transition to invalid.
	taskExecutionState_error
	// taskExecutionState_done tasks were running and completed without errors. They may transition to invalid.
	taskExecutionState_done
	// taskExecutionState_canceled tasks were running and were stopped by the
	// user. Tasks cannot leave canceled state.
	taskExecutionState_canceled
)

// taskExecution is a node in the Executor's DAG. It holds the state
// for a single task's executions and is reused across task executions.
type taskExecution struct {
	mu         sync.Mutex
	definition *Task

	ctx        context.Context
	cancel     func()
	state      taskExecutionState
	terminalCh chan struct{}

	dependencies []*taskExecution
	dependents   []*taskExecution

	pendingInvalidations map[InvalidationEvent]struct{}
}

func (e *taskExecution) simpleEvent() *simpleEvent {
	return &simpleEvent{
		taskHandler: NewTaskHandler(e),
		timestamp:   time.Now(),
	}
}

// ShouldExecute returns true if the taskExecution is marked
// invalidated AND all of its dependencies have successfully completed.
func (e *taskExecution) ShouldExecute() bool {
	if e.state != taskExecutionState_invalid {
		return false
	}

	ready := true
	for _, dep := range e.dependencies {
		if dep.state != taskExecutionState_done {
			ready = false
		}
	}
	return ready
}

// invalidate stops the task if running, resets the execution
// state for the task, then invalidates all dependents.
func (e *taskExecution) invalidate(executionCtx context.Context) {
	if e.state == taskExecutionState_invalid || e.state == taskExecutionState_canceled {
		return
	}

	e.cancel()
	e.state = taskExecutionState_invalid
	e.ctx, e.cancel = context.WithCancel(executionCtx)
	<-e.terminalCh
	e.terminalCh = make(chan struct{}, 1)
	e.pendingInvalidations = make(map[InvalidationEvent]struct{})
}

// Invalidate marks a taskExecution as invalid. It does not produce
// side effects by itself.
func (e *taskExecution) Invalidate(event InvalidationEvent) bool {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state == taskExecutionState_invalid {
		return false
	}

	if e.definition.ShouldInvalidate != nil && !e.definition.ShouldInvalidate(event) {
		return false
	}

	e.pendingInvalidations[event] = struct{}{}

	for _, dep := range e.dependents {
		dep.Invalidate(DependencyChange{
			Source: e.definition,
		})
	}

	return true
}

type taskSet map[*Task]*taskExecution

func (s taskSet) add(executionCtx context.Context, task *Task) (*taskExecution, []*taskExecution) {
	if s[task] != nil {
		return s[task], s[task].dependencies
	}

	ctx, cancel := context.WithCancel(executionCtx)
	self := &taskExecution{
		definition:           task,
		ctx:                  ctx,
		cancel:               cancel,
		state:                taskExecutionState_invalid,
		terminalCh:           make(chan struct{}, 1),
		pendingInvalidations: make(map[InvalidationEvent]struct{}),
	}

	var dependencies []*taskExecution
	for _, dep := range task.Dependencies {
		depExec, depExecDeps := s.add(ctx, dep)
		depExec.dependents = append(depExec.dependents, self)
		dependencies = append(dependencies, depExecDeps...)
		dependencies = append(dependencies, depExec)
	}

	s[task] = self
	self.dependencies = dependencies

	return self, dependencies
}

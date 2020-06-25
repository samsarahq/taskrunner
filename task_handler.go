package taskrunner

import (
	"sort"
)

func NewTaskHandler(execution *taskExecution) *TaskHandler {
	return &TaskHandler{
		execution: execution,
	}
}

type TaskHandler struct {
	execution *taskExecution
}

func (h *TaskHandler) Definition() *Task {
	return h.execution.definition
}

func (h *TaskHandler) Invalidate(reason InvalidationEvent) {
	h.execution.Invalidate(reason)
}

type TaskHandlerExecutionState string

const (
	TaskHandlerExecutionState_Invalid TaskHandlerExecutionState = "invalid"
	TaskHandlerExecutionState_Running                           = "running"
	TaskHandlerExecutionState_Error                             = "error"
	TaskHandlerExecutionState_Done                              = "done"
)

var TaskHandlerExecutionStateMap = map[taskExecutionState]TaskHandlerExecutionState{
	taskExecutionState_invalid: TaskHandlerExecutionState_Invalid,
	taskExecutionState_running: TaskHandlerExecutionState_Running,
	taskExecutionState_error:   TaskHandlerExecutionState_Error,
	taskExecutionState_done:    TaskHandlerExecutionState_Done,
}

func (h *TaskHandler) State() TaskHandlerExecutionState {
	return TaskHandlerExecutionStateMap[h.execution.state]
}

func (e *Executor) Tasks() []*TaskHandler {
	handlers := make([]*TaskHandler, len(e.tasks))
	var i int
	for _, execution := range e.tasks {
		handlers[i] = NewTaskHandler(execution)
		i++
	}
	sort.Slice(handlers, func(a, b int) bool {
		return handlers[a].execution.definition.Name < handlers[b].execution.definition.Name
	})
	return handlers
}

package taskrunner

import "time"

type ExecutorEventKind string

const (
	ExecutorEventKind_TaskStarted     ExecutorEventKind = "task.started"
	ExecutorEventKind_TaskCompleted                     = "task.completed"
	ExecutorEventKind_TaskFailed                        = "task.failed"
	ExecutorEventKind_TaskStopped                       = "task.stopped"
	ExecutorEventKind_TaskInvalidated                   = "task.invalidated"
	ExecutorEventKind_TaskDiagnostic                    = "task.diagnostic"
	ExecutorEventKind_TaskLog                           = "task.log"
	ExecutorEventKind_ExecutorSetup                     = "executor.setup"
)

type ExecutorEvent interface {
	Timestamp() time.Time
	Kind() ExecutorEventKind
	TaskHandler() *TaskHandler
}

type simpleEvent struct {
	timestamp   time.Time
	taskHandler *TaskHandler
}

func (e *simpleEvent) Timestamp() time.Time {
	return e.timestamp
}

func (e *simpleEvent) TaskHandler() *TaskHandler {
	return e.taskHandler
}

type ExecutorSetupEvent struct {
	*simpleEvent
}

func (e *ExecutorSetupEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_ExecutorSetup
}

type TaskStartedEvent struct {
	*simpleEvent
}

func (e *TaskStartedEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskStarted
}

type TaskLogEventStream int

const (
	TaskLogEventStdout TaskLogEventStream = iota
	TaskLogEventStderr
)

type TaskLogEvent struct {
	*simpleEvent
	Message string
	Stream  TaskLogEventStream
}

func (e *TaskLogEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskLog
}

type TaskCompletedEvent struct {
	*simpleEvent
	Duration time.Duration
}

func (e *TaskCompletedEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskCompleted
}

type TaskFailedEvent struct {
	*simpleEvent
	Error error
}

func (e *TaskFailedEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskFailed
}

type TaskStoppedEvent struct {
	*simpleEvent
}

func (e *TaskStoppedEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskStopped
}

type TaskInvalidatedEvent struct {
	*simpleEvent
	Reasons []InvalidationEvent
}

func (e *TaskInvalidatedEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskInvalidated
}

type TaskDiagnosticEvent struct {
	*simpleEvent
	Error error
}

func (e *TaskDiagnosticEvent) Kind() ExecutorEventKind {
	return ExecutorEventKind_TaskDiagnostic
}

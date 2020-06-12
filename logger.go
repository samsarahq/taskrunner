package taskrunner

import (
	"context"
	"io"
)

// LoggerFromContext gets the logger from the context.
func LoggerFromContext(ctx context.Context) *Logger {
	if logger, ok := ctx.Value(loggerKey{}).(*Logger); ok {
		return logger
	}
	return nil
}

type loggerKey struct{}
type LogProvider func(task *Task) (*Logger, error)

type Logger struct {
	Stdout io.Writer
	Stderr io.Writer
}

// eventLogger is an io.Writer that publishes to the taskrunner
// executor-level logger.
type eventLogger struct {
	executor *Executor
	task     *Task
	stream   TaskLogEventStream
}

// Write implements io.Writer.
func (l *eventLogger) Write(p []byte) (int, error) {
	l.executor.publishEvent(&TaskLogEvent{
		simpleEvent: l.executor.taskExecution(l.task).simpleEvent(),
		Message:     string(p),
		Stream:      l.stream,
	})
	return len(p), nil
}

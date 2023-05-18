package taskrunner

import (
	"context"
	"fmt"
	"io"
	"os"
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

// Logf writes a log message to the stdout logger for the task.
// It will append a newline if one is not present at the end.
func Logf(ctx context.Context, f string, args ...interface{}) {
	logger := LoggerFromContext(ctx)
	if logger == nil {
		fmt.Fprintln(os.Stderr, "🟡 Warning: There was no logger found in context, so falling back to `fmt.Printf`")
		fmt.Printf(f, args...)
		return
	}

	s := fmt.Sprintf(f, args...)
	if len(s) == 0 || s[len(s)-1] != '\n' {
		s += "\n"
	}
	fmt.Fprint(logger.Stdout, s)
}

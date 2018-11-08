package taskrunner

import (
	"bytes"
	"context"
	"io"

	"github.com/samsarahq/thunder/reactive"
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

type eventLogger struct {
	executor *Executor
	task     *Task
	stream   TaskLogEventStream
}

func (l *eventLogger) Write(p []byte) (int, error) {
	l.executor.publishEvent(&TaskLogEvent{
		simpleEvent: l.executor.taskExecution(l.task).simpleEvent(),
		Message:     string(p),
		Stream:      l.stream,
	})
	return len(p), nil
}

type LiveLogger struct {
	Logs     *bytes.Buffer
	Resource *reactive.Resource
}

func NewLiveLogger() *LiveLogger {
	return &LiveLogger{
		Logs:     new(bytes.Buffer),
		Resource: reactive.NewResource(),
	}
}

func (l *LiveLogger) Provider(task *Task) (*Logger, error) {
	return &Logger{l, l}, nil
}

func (l *LiveLogger) Write(p []byte) (int, error) {
	_, err := l.Logs.Write(p)
	l.Resource.Strobe()
	return len(p), err
}

func MergeLoggers(loggers ...*Logger) *Logger {
	var stdouts []io.Writer
	var stderrs []io.Writer
	for _, logger := range loggers {
		stdouts = append(stdouts, logger.Stdout)
		stderrs = append(stderrs, logger.Stderr)
	}

	return &Logger{io.MultiWriter(stdouts...), io.MultiWriter(stderrs...)}
}

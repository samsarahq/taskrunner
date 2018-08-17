package taskrunner

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"io"
	"os"
	"strings"
	"time"

	"github.com/fatih/color"
	"github.com/samsarahq/thunder/reactive"
)

type loggerKey struct{}
type LogProvider func(task *Task) (*Logger, error)

type Logger struct {
	Stdout io.Writer
	Stderr io.Writer
}

type PrefixedWriter struct {
	io.Writer
	Prefix    string
	Separator string
}

var colors = []color.Attribute{
	color.FgGreen,
	color.FgYellow,
	color.FgBlue,
	color.FgMagenta,
	color.FgCyan,
}

var defaultPadding = 16

func SetDefaultPadding(padding int) {
	defaultPadding = padding
}

func (w *PrefixedWriter) Write(p []byte) (n int, err error) {
	separator := w.Separator
	if separator == "" {
		separator = "|"
	}

	h := fnv.New32a()
	h.Write([]byte(w.Prefix))
	c := colors[int(h.Sum32())%len(colors)]

	lines := strings.Split(strings.TrimRight(string(p), "\n"), "\n")
	prefixed := make([]string, len(lines))
	for i, line := range lines {
		prefixed[i] = fmt.Sprintf("%s %s", fmt.Sprintf("%s %s", color.New(c).Sprint(leftPad(w.Prefix, defaultPadding)), separator), line)
	}

	_, err = w.Writer.Write([]byte(strings.Join(prefixed, "\n") + "\n"))
	return len(p), err
}

func leftPad(s string, l int) string {
	padding := l - len(s)
	if padding < 0 {
		padding = 0
	}
	return fmt.Sprintf("%s%s", strings.Repeat(" ", padding), s)
}

func StdoutLogProvider(task *Task) (*Logger, error) {
	return &Logger{
		&PrefixedWriter{Writer: os.Stdout, Prefix: task.Name, Separator: ">"},
		&PrefixedWriter{Writer: os.Stderr, Prefix: task.Name, Separator: ">"},
	}, nil
}

func LogfilesAppendProvider(task *Task) (*Logger, error) {
	if err := os.MkdirAll("./trlogs", 0755); err != nil {
		return nil, err
	}

	key := strings.Replace(task.Name, "/", "-", -1)

	log, err := os.OpenFile(fmt.Sprintf("./trlogs/%s.log", key), os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if err != nil {
		return nil, err
	}

	return &Logger{log, log}, nil
}

func LogfilesByDateProvider(task *Task) (*Logger, error) {
	if err := os.MkdirAll("./trlogs", 0755); err != nil {
		return nil, err
	}

	key := fmt.Sprintf("%s-%s", strings.Replace(task.Name, "/", "-", -1), time.Now().UTC().Format(time.RFC3339))

	log, err := os.Create(fmt.Sprintf("./trlogs/%s.log", key))
	if err != nil {
		return nil, err
	}

	return &Logger{log, log}, nil
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

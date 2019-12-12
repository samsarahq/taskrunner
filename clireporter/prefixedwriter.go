package clireporter

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"strings"

	"github.com/fatih/color"
	"github.com/samsarahq/taskrunner"
)

type logger struct {
	w         io.Writer
	writers   map[*taskrunner.Task]*PrefixedWriter
	separator string
	padding   int
}

func newLogger(w io.Writer) *logger {
	return &logger{
		w:       w,
		writers: make(map[*taskrunner.Task]*PrefixedWriter),
		padding: defaultPadding,
	}
}

func (l *logger) SetSeparator(separator string) { l.separator = separator }
func (l *logger) Write(t *taskrunner.Task, msg string) {
	l.writers[t].Write([]byte(msg))
}
func (l *logger) Writef(t *taskrunner.Task, format string, v ...interface{}) {
	l.Write(t, fmt.Sprintf(format, v...))
}

func (l *logger) registerTasks(handlers []*taskrunner.TaskHandler) {
	var tasks []*taskrunner.Task
	var longestTaskNameLength int
	for _, handler := range handlers {
		task := handler.Definition()
		tasks = append(tasks, task)
		length := len(task.Name)
		if length > longestTaskNameLength {
			longestTaskNameLength = length
		}
	}

	for _, task := range tasks {
		l.writers[task] = &PrefixedWriter{
			Writer:    l.w,
			Prefix:    task.Name,
			Padding:   longestTaskNameLength,
			Separator: l.separator,
		}
	}
}

type PrefixedWriter struct {
	io.Writer
	Prefix    string
	Padding   int
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

func (w *PrefixedWriter) Write(p []byte) (n int, err error) {
	separator := w.Separator
	if separator == "" {
		separator = ">"
	}

	h := fnv.New32a()
	h.Write([]byte(w.Prefix))
	c := colors[int(h.Sum32())%len(colors)]

	var jsonObj interface{}
	err = json.Unmarshal([]byte(p), &jsonObj)
	if err == nil {
		jsonData, err := json.MarshalIndent(jsonObj, "", "  ")
		if err == nil {
			// Treat output as indented JSON
			p = jsonData
		}
	}

	unescaped := strings.ReplaceAll(strings.TrimRight(string(p), "\n"), "\\n", "\n")
	unescaped = strings.ReplaceAll(unescaped, "\\t", "  ")

	lines := strings.Split(unescaped, "\n")
	prefixed := make([]string, len(lines))
	for i, line := range lines {
		prefixed[i] = fmt.Sprintf("%s %s", fmt.Sprintf("%s %s", color.New(c).Sprint(leftPad(w.Prefix, w.Padding)), separator), line)
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

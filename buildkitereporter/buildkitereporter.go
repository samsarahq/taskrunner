// This package provides buildkite annotations of failed tasks per buildkite job. It is primarily
// meant to be run directly in CI. Docs:
// https://buildkite.com/docs/agent/v3/cli-annotate
package buildkitereporter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/shell"
)

type ReporterOption func(r *reporter)

// WithPath sets a save path for the buildkite annotation (default: "taskrunner.annotation.md").
func WithPath(path string) ReporterOption {
	return func(r *reporter) { r.path = path }
}

// WithMaxLines sets the maximum stderr lines per task in the annotation (default: 50).
func WithMaxLines(maxLines int) ReporterOption {
	return func(r *reporter) { r.maxLines = maxLines }
}

// WithStreams sets which log streams to listen to (default: stderr).
func WithStreams(streams ...taskrunner.TaskLogEventStream) ReporterOption {
	return func(r *reporter) { r.streams = streams }
}

// WithUpload is mutually exclusive vs WithPath. Uploads the buildkite annotation directly.
func WithUpload(r *reporter) { r.uploadDirectly = true }

// WithWarningUpload is mutually exclusive vs WithPath. Uploads the buildkite annotation directly as a warning.
// This contrasts with WithUpload which will upload as an error.
func WithWarningUpload(r *reporter) {
	r.uploadDirectly = true
	r.uploadWarning = true
}

func Option(opts ...ReporterOption) func(*taskrunner.Runtime) {
	reporter := newReporter()
	for _, opt := range opts {
		opt(reporter)
	}
	return func(r *taskrunner.Runtime) {
		r.OnStop(reporter.Finish)
		r.Subscribe(func(events <-chan taskrunner.ExecutorEvent) error {
			for ev := range events {
				switch event := ev.(type) {
				case *taskrunner.TaskLogEvent:
					// Append log to stream if its one of the ones we're listening to.
					for _, stream := range reporter.streams {
						if stream == event.Stream {
							reporter.AppendLogs(event.TaskHandler().Definition(), event.Message)
						}
					}
				}
			}
			return nil
		})
	}
}

type reporter struct {
	sync.Mutex
	path           string
	uploadDirectly bool
	uploadWarning  bool
	maxLines       int
	stderrs        map[*taskrunner.Task][]string
	streams        []taskrunner.TaskLogEventStream
}

func newReporter() *reporter {
	return &reporter{
		maxLines: 50,
		path:     "taskrunner.annotation.md",
		stderrs:  make(map[*taskrunner.Task][]string),
		streams:  []taskrunner.TaskLogEventStream{taskrunner.TaskLogEventStderr},
	}
}

func messageToLines(message string) []string {
	return strings.Split(strings.TrimSpace(message), "\n")
}

func lastNElements(slice []string, n int) []string {
	start := len(slice) - n
	if start < 0 {
		start = 0
	}

	return slice[start:]
}

// AppendLogs appends a log message to the reporter's cache, keeping at most N entries.
func (r *reporter) AppendLogs(task *taskrunner.Task, message string) {
	r.Lock()
	defer r.Unlock()
	r.stderrs[task] = lastNElements(
		append(r.stderrs[task], messageToLines(message)...),
		r.maxLines,
	)
}

// Finish writes or uploads the buildkite annotation.
func (r *reporter) Finish(ctx context.Context, executor *taskrunner.Executor) error {
	if r.uploadDirectly {
		return r.upload(ctx, executor.ShellRun, executor.Tasks())
	}
	return r.writeFile(executor.Tasks())
}

func (r *reporter) writeFile(tasks []*taskrunner.TaskHandler) error {
	annotation, failures := r.write(tasks)
	if failures == 0 {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(r.path), 0755); err != nil {
		return err
	}

	f, err := os.OpenFile(r.path, os.O_WRONLY|os.O_CREATE, 0660)
	defer f.Close()
	if err != nil {
		return err
	}

	if _, err := io.Copy(f, annotation); err != nil {
		return err
	}
	return nil
}

func (r *reporter) upload(ctx context.Context, shellRun shell.ShellRun, tasks []*taskrunner.TaskHandler) error {
	annotation, failures := r.write(tasks)
	if failures == 0 {
		return nil
	}

	style := "error"
	contextSuffix := ""
	if r.uploadWarning {
		style = "warning"
		contextSuffix = "-warning"
	}

	cmd := fmt.Sprintf("buildkite-agent annotate --style=%s --context=taskrunner%s --append", style, contextSuffix)

	fmt.Println("Uploading buildkite annotation")
	return shellRun(
		ctx,
		cmd,
		shell.Stdout(os.Stdout),
		shell.Stderr(os.Stderr),
		shell.Stdin(annotation),
	)
}

// writeFile creates a buildkite annotation.
func (r *reporter) write(tasks []*taskrunner.TaskHandler) (io.Reader, int) {
	var buf bytes.Buffer
	var failureCount int
	var failedTasks []*taskrunner.Task

	for _, task := range tasks {
		if task.State() == taskrunner.TaskHandlerExecutionState_Error {
			failureCount++
			failedTasks = append(failedTasks, task.Definition())
		}
	}

	fmt.Fprintf(&buf, "<h5>%s taskrunner: %d tasks failed</h5>\n", os.Getenv("BUILDKITE_LABEL"), failureCount)

	for _, task := range failedTasks {
		fmt.Fprintf(&buf, "<details><summary><code>%s</code></summary>\n<pre>", task.Name)
		for _, out := range r.stderrs[task] {
			fmt.Fprintf(&buf, "<code>%s</code>\n", out)
		}
		fmt.Fprintln(&buf, "</pre></details>")
	}

	return &buf, failureCount
}

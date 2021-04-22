package taskrunner

import (
	"context"

	"github.com/samsarahq/taskrunner/shell"
)

type Task struct {
	Name string

	// A one or two sentence description of what the task does
	Description string

	// XXX: Pass files in so that task can decide to do less work, i.e. if
	// you change a go file we can run go fmt on just that file.

	// Run is called when the task should be run. The function should gracefully
	// handle context cancelation.
	Run func(ctx context.Context, shellRun shell.ShellRun) error

	ShouldInvalidate func(event InvalidationEvent) bool

	// Dependencies specifies any tasks that this task depends on. When those
	// dependencies are invalidated, this task will also be invalidated.
	Dependencies []*Task

	// KeepAlive specifies whether or not this task is long lived.
	// If true, this task will be restarted when it exits, regardless of exit code.
	KeepAlive bool

	// Hidden specifies whether or not to show the task when -list is called.
	Hidden bool

	// Sources specifies globs that this task depends on. When those files change,
	// the task will be invalidated.
	Sources []string

	// Ignore is a set of globs subtracted from Sources.
	Ignore []string
}

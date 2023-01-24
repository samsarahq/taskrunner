package taskrunner

import (
	"context"
	"time"

	"github.com/samsarahq/taskrunner/shell"
)

type Task struct {
	Name string

	// A one or two sentence description of what the task does
	Description string

	// XXX: Pass files in so that task can decide to do less work, i.e. if
	// you change a go file we can run go fmt on just that file.

	// [DEPRECATED] This fn is deprecated in favor of RunWithFlags
	// Run is called when the task should be run. The function should gracefully
	// handle context cancelation.
	Run func(ctx context.Context, shellRun shell.ShellRun) error

	// RunWithFlags is called when the task should be run
	// and includes all flags passed to the task. The function
	// should gracefully handle context cancellation.
	RunWithFlags func(ctx context.Context, shellRun shell.ShellRun, flagArgs map[string]FlagArg) error

	// Flags is a list of supported flags for this task (e.g. --var="val", -bool).
	// Only the flags specified in this list will be considered valid arguments to a task.
	Flags []TaskFlag

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

	// IsGroup specifies whether this task is a pseudo-task that groups
	// other tasks underneath it.
	IsGroup bool
}

type TaskFlag struct {
	// Description should be a string that describes what effect passing the flag to a task has.
	// This description is shown automatically in the generated --help message.
	// This field must be non-empty.
	Description string

	// ShortName is the single character version of the flag. If not 0, the
	// option flag can be 'activated' using -<ShortName>. Either ShortName
	// or LongName needs to be non-empty.
	ShortName rune

	// LongName is the multi-character version of the flag. If not "", the flag can be
	// activated using --<LongName>. Either ShortName or LongName needs to be non-empty.
	LongName string

	// Default is the default value for the flag.
	Default string

	// ValueType describes what the flag type is (e.g. --flag [ValueType])
	// and can be one of either: StringTypeFlag, BoolTypeFlag,
	// IntTypeFlag, Float64TypeFlag, or DurationTypeFlag.
	// Its value is shown automatically in the generated --help message.
	// This field must be non-empty.
	ValueType string
}

const (
	StringTypeFlag   string = "string"
	BoolTypeFlag     string = "bool"
	IntTypeFlag      string = "int"
	Float64TypeFlag  string = "float64"
	DurationTypeFlag string = "duration"
)

type FlagArg struct {
	Value       interface{}
	StringVal   func() *string
	BoolVal     func() *bool
	IntVal      func() *int
	Float64Val  func() *float64
	DurationVal func() *time.Duration
}

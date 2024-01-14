# Taskrunner
A configurable taskrunner written in Go. Taskrunner can streamline your build system by creating reusable task definitions with names, descriptions, dependencies, and a run function written in Go. Taskrunner comes with a shell interpreter, so it can run arbitrary functions on the command line.

## Setup
Add taskrunner as a dependency `go get -v github.com/samsarahq/taskrunner`, and create the following `main.go` file:
```go
package main

import (
	"github.com/samsarahq/taskrunner"
	"github.com/ChenJesse/taskrunner/clireporter"
)

func main() {
	taskrunner.Run(clireporter.StdoutOption)
}
```

## Run a task
Taskrunner can be started by running `go run .`, it is possible to limit log output with `PRINT_LOG_LEVEL=error`. To run an individual task specify the task name `go run . my/task`. For a full list of tasks use `go run . -describe`, or for a list of names `go run . -list`. It may be a good idea to alias `taskrunner` to run taskrunner.

## Example tasks
Add a `tasks.go` file, this contains all task descriptions:
```go
package main

import (
	"context"

	"github.com/samsarahq/taskrunner"
	"github.com/ChenJesse/taskrunner/goextensions"
	"github.com/ChenJesse/taskrunner/shell"
)

// Run a simple task.
var myTask = taskrunner.Add(&taskrunner.Task{
	Name:         "my/task",
	RunWithFlags: func(ctx context.Context, shellRun shell.ShellRun, flags map[string]taskrunner.FlagArg) error {
		return shellRun(ctx, `echo Hello World`)
	},
})

// Run a task which performs different behaviour based on
// the flags passed to it via CLI.
var myTaskWithFlags = taskrunner.Add(&taskrunner.Task{
	Name:         "my/go/task",
	Description:  "Run a go file and re-run when it changes",
	RunWithFlags: func(ctx context.Context, shellRun shell.ShellRun, flags map[string]taskrunner.FlagArg) error {
		// Note: You can also check the ShortName string i.e. flags["b"].
		if flag, ok := flags["boolFlag"]; ok {
			// Note: FlagArgs include helper fns: BoolVal, StringVal, IntVal, Float64Val
			// and DurationVal. They cast the arg passed via CLI into the appropriate
			// type based on the flag ValueType.
			// The raw value passed via CLI is also avaialble via the FlagArg helper fn Value. 
			if flag.BoolVal() {
				return ShellRun(ctx, `echo Hello World: 1`)
			}
		}
			
		return shellRun(ctx, `echo Hello World: 2`)
	},
	Flags: []taskrunner.TaskFlag{
		{
			Description: "Passing the `--boolFlag/-b` flag has X effect on the task.",
			LongName: "boolFlag",
			ShortName: []rune("b")[0],
			Default: "false",
			// Note: BoolTypeFlag, StringTypeFlag, Float64TypeFlag and DurationFlag.
			ValueType: taskrunner.BoolTypeFlag,
		}
	},
})

// Run a task depending on another task.
var myDependentTask = taskrunner.Add(&taskrunner.Task{
	Name:         "my/dependent/task",
	Dependencies: []*taskrunner.Task{myTask},
	RunWithFlags: func(ctx context.Context, shellRun shell.ShellRun flags map[string]taskrunner.FlagArg) error {
		return shellRun(ctx, `echo Hello Again`)
	},
})


// Run a task which monitors a file and reruns on changes
// a change will also invalidate dependencies.
var myGoTask = taskrunner.Add(&taskrunner.Task{
	Name:         "my/go/task",
	Description:  "Run a go file and re-run when it changes",
	RunWithFlags: func(ctx context.Context, shellRun shell.ShellRun, flags map[string]taskrunner.FlagArg) error {
		return shellRun(ctx, `cd src/example && go run .`)
	},
	Sources: []string{"src/example/**/*.go"},
})

// Run a task using WrapWithGoBuild to automatically setup
// invalidation for the task according to the import graph
// of the specified package.
var builder = goextensions.NewGoBuilder()

var myWrappedTask = taskrunner.Add(&taskrunner.Task{
	Name: "my/wrapped/task",
        RunWithFlags: func(ctx context.Context, shellRun shell.ShellRun, flags map[string]taskrunner.FlagArg) error {
		return shellRun(ctx, `example`)
	},
}, builder.WrapWithGoBuild("example"))
```

## Default tasks to run
It is possible to add a `workspace.taskrunner.json` file, this contains the default tasks to run when taskrunner is run without any arguments.
```json
{
  "path": "./",
  "desiredTasks": [
    "my/task"
  ]
}
```

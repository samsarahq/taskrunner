# Taskrunner
A configurable taskrunner written in Go. Taskrunner can streamline your build system by creating reusable task definitions with names, descriptions, dependencies, and a run function written in Go. Taskrunner comes with a shell interpreter, so it can run arbitrary functions on the command line.

## Setup
Add taskrunner as a dependency `go get -v github.com/samsarahq/taskrunner`, and create the following `main.go` file:
```go
package main

import (
	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/clireporter"
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
	"github.com/samsarahq/taskrunner/goextensions"
	"github.com/samsarahq/taskrunner/shell"
)

// Run a simple task.
var myTask = taskrunner.Add(&taskrunner.Task{
	Name:         "my/task",
	Run: func(ctx context.Context, shellRun shell.ShellRun) error {
		return shellRun(ctx, `echo Hello World`)
	},
})

// Run a task depending on another task.
var myDependentTask = taskrunner.Add(&taskrunner.Task{
	Name:         "my/dependent/task",
	Dependencies: []*taskrunner.Task{myTask},
	Run: func(ctx context.Context, shellRun shell.ShellRun) error {
		return shellRun(ctx, `echo Hello Again`)
	},
})


// Run a task which monitors a file and reruns on changes
// a change will also invalidate dependencies.
var myGoTask = taskrunner.Add(&taskrunner.Task{
	Name:         "my/go/task",
	Description:  "Run a go file and re-run when it changes",
	Run: func(ctx context.Context, shellRun shell.ShellRun) error {
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
        Run: func(ctx context.Context, shellRun shell.ShellRun) error {
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

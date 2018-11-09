package goextensions

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/clireporter"
	"github.com/samsarahq/taskrunner/shell"
	"mvdan.cc/sh/interp"
)

var stdlibLookupMu = sync.Mutex{}
var stdlibLookup = make(map[string]*bool)

func isStdLib(pkg string) bool {
	stdlibLookupMu.Lock()
	defer stdlibLookupMu.Unlock()
	if stdlibLookup[pkg] != nil {
		return *stdlibLookup[pkg]
	}

	_, err := os.Stat(filepath.Join(os.Getenv("GOROOT"), "src", pkg))
	isStdLib := err == nil
	stdlibLookup[pkg] = &isStdLib
	return isStdLib
}

// GoBuilder is a batcher for `go build` runs. It coalesces builds into 500ms batches.
type GoBuilder struct {
	packages map[string]struct{}
	timer    *time.Timer
	requests chan string

	mu       sync.Mutex
	doneCh   chan struct{}
	shellRun shell.ShellRun
	ctx      context.Context

	err error
}

func NewGoBuilder() *GoBuilder {
	timer := time.NewTimer(time.Second)
	timer.Stop()

	ch := make(chan string, 128)
	packages := make(map[string]struct{})
	builder := &GoBuilder{
		timer:    timer,
		packages: packages,
		requests: ch,
	}

	go func() {
		for {
			select {
			case pkg := <-ch:
				builder.packages[pkg] = struct{}{}
				timer.Reset(time.Millisecond * 500)

			case <-timer.C:
				builder.build()
			}
		}
	}()

	return builder
}

func (b *GoBuilder) build() {
	b.mu.Lock()
	defer b.mu.Unlock()
	packages := make([]string, 0, len(b.packages))
	for pkg := range b.packages {
		packages = append(packages, pkg)
	}
	pkgList := strings.Join(packages, " ")
	b.packages = make(map[string]struct{})

	stdout := &clireporter.PrefixedWriter{Writer: os.Stdout, Prefix: "go/build/dev", Separator: ">"}
	fmt.Fprintf(stdout, "building packages: %s", pkgList)

	b.err = shell.Run(b.ctx, fmt.Sprintf("go install -v %s", pkgList), func(r *interp.Runner) {
		r.Stdout = stdout
		r.Stderr = &clireporter.PrefixedWriter{Writer: os.Stderr, Prefix: "go/build/dev", Separator: ">"}
	})
	fmt.Fprintln(stdout, "Completed")

	close(b.doneCh)
	b.doneCh = nil
}

// Build schedules a go build and waits for its completion. Note that since builds are batched,
// the returned error may or may not be related to the requested package.
func (b *GoBuilder) Build(ctx context.Context, shellRun shell.ShellRun, pkg string) error {
	b.requests <- pkg

	b.mu.Lock()
	b.shellRun = shellRun
	b.ctx = ctx
	if b.doneCh == nil {
		b.doneCh = make(chan struct{}, 0)
	}
	b.mu.Unlock()

	<-b.doneCh
	return b.err
}

// buildBinder provides hooks for finding the dependencies of a package
// and wiring up a shouldInvalidate implementation that only accepts relevant
// file changes.
type buildBinder struct {
	pkg             string
	pkgDependencies []string
}

func newBuildBinder(pkg string) *buildBinder {
	binder := &buildBinder{pkg: pkg}
	return binder
}

func (b *buildBinder) saveDependencies(ctx context.Context, shellRun shell.ShellRun) error {
	var buffer bytes.Buffer
	if err := shellRun(ctx, fmt.Sprintf("go list -f '{{ .Deps }}' %s", b.pkg), shell.Stdout(&buffer)); err != nil {
		return err
	}

	b.pkgDependencies = strings.Split(strings.TrimSuffix(strings.TrimPrefix(buffer.String(), "["), "]"), " ")
	return nil
}

func (b *buildBinder) shouldInvalidate(event taskrunner.InvalidationEvent) bool {
	switch event := event.(type) {
	case taskrunner.FileChange:
		// Bail out if we have not yet recorded any dependencies, i.e.
		// the task has yet to run, or an error occurred. We don't have enough
		// information to make a good judgment.
		if b.pkgDependencies == nil {
			return true
		}

		for _, dep := range append(b.pkgDependencies, b.pkg) {
			// Ignore dependencies that are part of the std lib.
			if isStdLib(dep) {
				continue
			}
			if ok := strings.Contains(event.File, dep); ok {
				return true
			}
		}
		return false
	default:
		return true
	}
}

// WrapWithGoBuild takes an input Task and builds a specified go package
// before running the task. If no task sources are specified, then
// WrapWithGoBuild will automatically setup invalidation for the task according
// to the import graph of the specified package.
func (builder *GoBuilder) WrapWithGoBuild(pkg string) taskrunner.TaskOption {
	return func(task *taskrunner.Task) *taskrunner.Task {

		newTask := *task

		buildBinder := newBuildBinder(pkg)
		newTask.Run = func(ctx context.Context, shellRun shell.ShellRun) error {
			if err := builder.Build(ctx, shellRun, pkg); err != nil {
				return err
			}

			if err := buildBinder.saveDependencies(ctx, shellRun); err != nil {
				return err
			}

			if task.Run != nil {
				return task.Run(ctx, shellRun)
			}

			return nil
		}

		newTask.ShouldInvalidate = func(event taskrunner.InvalidationEvent) bool {
			delegated := true
			if task.ShouldInvalidate != nil {
				delegated = task.ShouldInvalidate(event)
			}
			return buildBinder.shouldInvalidate(event) && delegated
		}

		return &newTask
	}
}

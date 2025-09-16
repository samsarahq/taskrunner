package goextensions

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
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

	_, err := os.Stat(filepath.Join(runtime.GOROOT(), "src", pkg))
	isStdLib := err == nil
	stdlibLookup[pkg] = &isStdLib
	return isStdLib
}

type request struct {
	Package string
	Logger  *taskrunner.Logger
}

// GoBuilder is a batcher for `go build` runs. It coalesces builds into 500ms batches.
type GoBuilder struct {
	packages map[string]struct{}
	loggers  map[*taskrunner.Logger]struct{}
	timer    *time.Timer
	requests chan request

	mu       sync.Mutex
	doneCh   chan struct{}
	shellRun shell.ShellRun
	ctx      context.Context

	err error

	// Options:
	LogToStdout     bool
	ModuleRoot      string
	ShellRunOptions []shell.RunOption
}

func NewGoBuilder() *GoBuilder {
	timer := time.NewTimer(time.Second)
	timer.Stop()

	ch := make(chan request, 128)
	builder := &GoBuilder{
		timer:    timer,
		packages: make(map[string]struct{}),
		loggers:  make(map[*taskrunner.Logger]struct{}),
		requests: ch,
	}

	go func() {
		for {
			select {
			case request := <-ch:
				builder.mu.Lock()
				builder.packages[request.Package] = struct{}{}
				if request.Logger != nil {
					builder.loggers[request.Logger] = struct{}{}
				}
				builder.mu.Unlock()
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

	var stdout, stderr io.Writer
	if b.LogToStdout {
		stdout = &clireporter.PrefixedWriter{Writer: os.Stdout, Prefix: "go/build/dev", Separator: ">"}
		stderr = &clireporter.PrefixedWriter{Writer: os.Stderr, Prefix: "go/build/dev", Separator: ">"}
	} else {
		stdouts := make([]io.Writer, 0, len(b.loggers))
		stderrs := make([]io.Writer, 0, len(b.loggers))
		for l := range b.loggers {
			stdouts = append(stdouts, l.Stdout)
			stderrs = append(stderrs, l.Stdout)
		}
		stdout = io.MultiWriter(stdouts...)
		stderr = io.MultiWriter(stderrs...)
	}

	fmt.Fprintf(stdout, "building packages: %s", pkgList)
	shellRunOptions := []shell.RunOption{func(r *interp.Runner) {
		r.Stdout = stdout
		r.Stderr = stderr
		if b.ModuleRoot != "" {
			r.Dir = b.ModuleRoot
		}
	}}
	shellRunOptions = append(shellRunOptions, b.ShellRunOptions...)
	b.err = shell.Run(b.ctx, fmt.Sprintf("go install -v %s", pkgList), shellRunOptions...)
	fmt.Fprintln(stdout, "done building packages")

	if b.doneCh != nil {
		close(b.doneCh)
		b.doneCh = nil
	}

	// Clear the package and logger list.
	b.packages = make(map[string]struct{})
	b.loggers = make(map[*taskrunner.Logger]struct{})
}

// Build schedules a go build and waits for its completion. Note that since builds are batched,
// the returned error may or may not be related to the requested package.
func (b *GoBuilder) Build(ctx context.Context, shellRun shell.ShellRun, pkg string) error {
	b.requests <- request{
		Package: pkg,
		Logger:  taskrunner.LoggerFromContext(ctx),
	}

	b.mu.Lock()
	b.shellRun = shellRun
	b.ctx = ctx
	if b.doneCh == nil {
		b.doneCh = make(chan struct{}, 0)
	}
	doneCh := b.doneCh
	b.mu.Unlock()

	<-doneCh

	b.mu.Lock()
	err := b.err
	b.mu.Unlock()
	return err
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

func (b *buildBinder) getPackageDependencies(ctx context.Context, root string, shellRun shell.ShellRun) ([]string, error) {
	var buffer bytes.Buffer
	if err := shellRun(ctx, fmt.Sprintf("go list -f '{{ .Deps }}' %s", b.pkg), shell.Stdout(&buffer), func(r *interp.Runner) {
		if root != "" {
			r.Dir = root
		}
	}); err != nil {
		return nil, err
	}
	allDependencies := strings.Split(strings.TrimSuffix(strings.TrimPrefix(buffer.String(), "["), "]\n"), " ")
	dependencies := make([]string, 0, len(allDependencies))
	for _, dep := range allDependencies {
		if !isStdLib(dep) {
			dependencies = append(dependencies, dep)
		}
	}
	// Add the current package to the dependencies.
	dependencies = append(dependencies, b.pkg)
	return dependencies, nil
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

		// It is possible that the invalidation event is for a non-go file.
		// The builder does not know how to handle this, and is only concerned
		// with invalidating go packages.
		if !strings.HasSuffix(event.File, ".go") {
			return true
		}

		for _, dep := range append(b.pkgDependencies, b.pkg) {
			dir := filepath.Dir(event.File)
			if strings.HasSuffix(dir, dep) {
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

		augmentTask := func(ctx context.Context, shellRun shell.ShellRun) (shell.ShellRun, error) {
			shellRun = injectShellRunOptions(shellRun, builder.ShellRunOptions)

			if err := builder.Build(ctx, shellRun, pkg); err != nil {
				return nil, err
			}

			dependencies, err := buildBinder.getPackageDependencies(ctx, builder.ModuleRoot, shellRun)
			if err != nil {
				return nil, err
			}
			buildBinder.pkgDependencies = dependencies

			sources := make([]string, 0, len(dependencies))
			// Watch all Go files in each dependent package.
			for _, dependency := range dependencies {
				sources = append(sources, "**/"+dependency+"/*.go")
			}
			newTask.Sources = append(task.Sources, sources...)

			return shellRun, nil
		}

		// task.Run is deprecated, so default to defining
		// newTask.RunWithFlags if task.Run is not defined.
		// If both Run and RunWithFlags are defined, an error will
		// be thrown in the from the registry when the task is added.
		if task.Run != nil {
			newTask.Run = func(ctx context.Context, shellRun shell.ShellRun) error {
				shellRun, err := augmentTask(ctx, shellRun)
				if err != nil {
					return err
				}

				if task.Run != nil {
					return task.Run(ctx, shellRun)
				}

				return nil
			}
		} else {
			newTask.RunWithFlags = func(ctx context.Context, shellRun shell.ShellRun, flags map[string]taskrunner.FlagArg) error {
				shellRun, err := augmentTask(ctx, shellRun)
				if err != nil {
					return err
				}

				if task.RunWithFlags != nil {
					return task.RunWithFlags(ctx, shellRun, flags)
				}

				return nil
			}
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

// injectShellRunOptions injects additional options into shellRun.
func injectShellRunOptions(shellRun shell.ShellRun, additionalOptions []shell.RunOption) shell.ShellRun {
	return func(ctx context.Context, command string, opts ...shell.RunOption) error {
		// Create a new options slice combining the builder options with the ones passed in.
		var optionsWithBuilderOptions []shell.RunOption
		optionsWithBuilderOptions = append(optionsWithBuilderOptions, opts...)
		optionsWithBuilderOptions = append(optionsWithBuilderOptions, additionalOptions...)
		return shellRun(ctx, command, optionsWithBuilderOptions...)
	}
}

package shell

import (
	"context"
	"fmt"
	"io"
	"strings"

	"mvdan.cc/sh/interp"
	"mvdan.cc/sh/syntax"
)

type RunOption func(*interp.Runner)

type ShellRun func(ctx context.Context, command string, options ...RunOption) error

// Stdout sets the output location for the stdout of a command.
func Stdout(writer io.Writer) RunOption {
	return func(r *interp.Runner) {
		r.Stdout = writer
	}
}

// Stderr sets the output location for the stderr of a command.
func Stderr(writer io.Writer) RunOption {
	return func(r *interp.Runner) {
		r.Stderr = writer
	}
}

// Stdin pipes in the contents provided by the reader to the command.
func Stdin(reader io.Reader) RunOption {
	return func(r *interp.Runner) {
		r.Stdin = reader
	}
}

// Env sets the environment variables for the command.
func Env(vars map[string]string) RunOption {
	return func(r *interp.Runner) {
		for k, v := range vars {
			r.Env.Set(k, v)
		}
	}
}

// Dir sets the working directory for the command.
func Dir(dir string) RunOption {
	return func(r *interp.Runner) {
		r.Dir = dir
	}
}

// Run executes a shell command.
func Run(ctx context.Context, command string, opts ...RunOption) error {
	p, err := syntax.NewParser().Parse(strings.NewReader(command), "")
	if err != nil {
		return err
	}

	r, err := interp.New()
	if err != nil {
		panic(fmt.Errorf("failed to set up interpreter: %s", err.Error()))
	}

	for _, opt := range opts {
		opt(r)
	}

	return r.Run(ctx, p)
}

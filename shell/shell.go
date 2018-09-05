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

func Stdout(writer io.Writer) RunOption {
	return func(r *interp.Runner) {
		r.Stdout = writer
	}
}

func Env(vars map[string]string) RunOption {
	return func(r *interp.Runner) {
		for k, v := range vars {
			r.Env.Set(k, v)
		}
	}
}

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

package shell

import (
	"context"
	"io"
	"os"
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

	env, err := interp.EnvFromList(os.Environ())
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	r := &interp.Runner{
		Dir: wd,
		Env: env,

		Exec: interp.DefaultExec,
		Open: interp.OpenDevImpls(interp.DefaultOpen),

		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}

	for _, opt := range opts {
		opt(r)
	}

	return r.Run(ctx, p)
}

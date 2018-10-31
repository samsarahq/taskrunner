package taskrunner

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"golang.org/x/sync/errgroup"
)

var (
	flags          = flag.NewFlagSet("taskrunner", 0)
	configFile     string
	nonInteractive bool
	listTasks      bool
)

func init() {
	flags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: taskrunner [task...]\n")
		flags.PrintDefaults()
	}
	flags.StringVar(&configFile, "config", "", "Configuration file to use")
	flags.BoolVar(&nonInteractive, "non-interactive", false, "Non-interactive mode")
	flags.BoolVar(&listTasks, "list", false, "List all tasks")
}

type RunOptions struct {
	ReporterFns     []func(ctx context.Context, executor *Executor) error
	ExecutorOptions []ExecutorOption
}

type RunOption func(options *RunOptions)

func ExecutorOptions(opts ...ExecutorOption) RunOption {
	return func(r *RunOptions) {
		r.ExecutorOptions = append(r.ExecutorOptions, opts...)
	}
}

func Run(tasks []*Task, options ...RunOption) {
	runOptions := &RunOptions{}
	for _, option := range options {
		option(runOptions)
	}

	if err := flags.Parse(os.Args[1:]); err != nil {
		return
	}

	var config *Config
	var err error
	if configFile == "" {
		config, err = ReadUserConfig()
	} else {
		config, err = ReadConfig(configFile)
	}

	if err != nil {
		log.Fatalf("config error: unable to read config:\n%v\n", err)
	}

	if listTasks {
		outputString := "Run specified tasks with `taskrunner taskname1 taskname2`\nTasks available:"
		for _, task := range tasks {
			outputString = outputString + "\n\t" + task.Name
		}
		fmt.Println(outputString)
		return
	}

	log.Println("Using config", config.ConfigFilePath())
	executor := NewExecutor(config, tasks, runOptions.ExecutorOptions...)

	desiredTasks := config.DesiredTasks
	config.Watch = !nonInteractive
	if len(flags.Args()) > 0 {
		config.Watch = false
		desiredTasks = flags.Args()
	}

	if len(tasks) == 0 {
		log.Fatalln("No tasks specified")
	}
	log.Println("Desired tasks:", strings.Join(desiredTasks, ", "))
	log.Printf("Watch mode: %t", config.Watch)

	ctx, cancel := context.WithCancel(context.Background())
	onInterruptSignal(cancel)

	g, ctx := errgroup.WithContext(ctx)

	// Reporters should use a different context because we want to stage
	// their cancellation after the executor itself has been completed.
	reporterCtx, cancelReporter := context.WithCancel(context.Background())
	for _, reporterFn := range runOptions.ReporterFns {
		g.Go(func() error {
			return reporterFn(reporterCtx, executor)
		})
	}

	g.Go(func() error {
		defer cancelReporter()
		err := executor.Run(ctx, desiredTasks...)

		// We only care about propagating errors up to the errgroup
		// if we were not in watch mode.
		if !config.Watch {
			return err
		}
		return nil
	})

	if err := g.Wait(); err != nil {
		log.Fatalf("run error:\n%v\n", err)
	}
}

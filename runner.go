package taskrunner

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/taskrunner/config"

	"golang.org/x/sync/errgroup"
)

var (
	configFile     string
	nonInteractive bool
	listTasks      bool
	listAllTasks   bool
	watch          bool
	describeTasks  bool
)

var logger RunnerLogger = log.New(os.Stderr, "", log.LstdFlags)

type RunnerLogger interface {
	Printf(format string, v ...interface{})
	Println(v ...interface{})
	Fatalf(format string, v ...interface{})
	Fatalln(v ...interface{})
}

// Runtime represents the external interface of an Executor's runtime. It is how taskrunner
// extensions can register themselves to taskrunner's lifecycle.
type Runtime struct {
	subscriptions   []func(events <-chan ExecutorEvent) error
	onStartHooks    []func(ctx context.Context, executor *Executor) error
	onStopHooks     []func(ctx context.Context, executor *Executor) error
	executorOptions []ExecutorOption

	registry *Registry
	flags    *flag.FlagSet
}

func newRuntime() *Runtime {
	r := &Runtime{
		registry: DefaultRegistry,
		flags:    flag.NewFlagSet("taskrunner", 0),
	}

	r.flags.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "[Usage]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        taskrunner [task...]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        taskrunner [task] --help\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        taskrunner [option]\n")
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintf(flag.CommandLine.Output(), "[Options]\n")
		r.flags.PrintDefaults()
		fmt.Fprintln(flag.CommandLine.Output())
		fmt.Fprintf(flag.CommandLine.Output(), "[Example Usage]\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        `taskrunner mytask`\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        `taskrunner mytask ---help`\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        `taskrunner mytask1 mytask2`\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        `taskrunner mytask --mode=dev`\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        `taskrunner --config ./customconfig.json mytask --mode=dev`\n")
		fmt.Fprintf(flag.CommandLine.Output(), "        `taskrunner --list`\n")
	}
	r.flags.StringVar(&configFile, "config", "", "Configuration file to use")
	r.flags.BoolVar(&nonInteractive, "non-interactive", false, "Non-interactive mode (only applies when running the default set of tasks)")
	r.flags.BoolVar(&listTasks, "list", false, "List all tasks except those marked \"Hidden\"")
	r.flags.BoolVar(&listAllTasks, "listAll", false, "List all tasks including those marked as \"Hidden\"")
	r.flags.BoolVar(&watch, "watch", false, "Run in watch mode (only applies when passing custom tasks)")
	r.flags.BoolVar(&describeTasks, "describe", false, "Describe all tasks")

	return r
}

// OnStart is run after taskrunner has built up its task execution list.
func (r *Runtime) OnStart(f func(context.Context, *Executor) error) {
	r.onStartHooks = append(r.onStartHooks, f)
}

// Subscribe provides an events stream channel that is populated after taskrunner has loaded. The
// channel is read-only and is automatically closed on exit.
func (r *Runtime) Subscribe(f func(events <-chan ExecutorEvent) error) {
	r.subscriptions = append(r.subscriptions, f)
}

// OnStop is before taskrunner exits (either because its tasks have exited or because of a SIGINT).
func (r *Runtime) OnStop(f func(context.Context, *Executor) error) {
	r.onStopHooks = append(r.onStopHooks, f)
}

// WithFlag allows for external registration of flags.
func (r *Runtime) WithFlag(f func(flags *flag.FlagSet)) { f(r.flags) }

type RunOption func(options *Runtime)

func ExecutorOptions(opts ...ExecutorOption) RunOption {
	return func(r *Runtime) {
		r.executorOptions = append(r.executorOptions, opts...)
	}
}

// groupTaskAndFlagArgs groups all tasks with their flag arguments.
// Notably, is not aware of flags passed to taskrunner itself.
// Those flags are handled separately via the flags package in the Run function.
// Only tasks defined in the registry are returned; the boolean return value
// represents whether unknown tasks were found.
func (r *Runtime) groupTaskAndFlagArgs(args []string) (map[string][]string, bool) {
	flagArgsPerTask := map[string][]string{}

	foundInvalidTasks := false
	var currTaskName string
	var currFlagsList []string
	for _, arg := range args {
		isFlag := strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--")
		if isFlag {
			// If we have identified a current task and we think the arg is a flag,
			// add it to the list of flags we are storing for the current task.
			// Notably, if the first flags are options to taskrunner, currTaskName will be ""
			// and we will not group those flags with any tasks.
			if currTaskName != "" {
				currFlagsList = append(currFlagsList, arg)
			}
		} else {
			// If this arg is a new task, store the flags we've collected for prev task.
			if currTaskName != "" {
				flagArgsPerTask[currTaskName] = currFlagsList
				currTaskName = ""
				currFlagsList = []string{}
			}

			// If the arg is a valid task, start tracking the task/flag group.
			if _, ok := r.registry.definitions[arg]; ok {
				currTaskName = arg
				currFlagsList = []string{}
			} else {
				foundInvalidTasks = true
				logger.Printf("Unrecognized task: %s, ignoring it and following flags\n", arg)
				// Note that in this case, currTaskName must be "" to ignore following flags.
			}
		}
	}

	// Ensure the we register the flags passed to the last task.
	if currTaskName != "" {
		flagArgsPerTask[currTaskName] = currFlagsList
	}

	// Return map of task to list of flags passed to it.
	return flagArgsPerTask, foundInvalidTasks
}

func Run(options ...RunOption) {
	runtime := newRuntime()
	for _, option := range options {
		option(runtime)
	}

	if err := runtime.flags.Parse(os.Args[1:]); err != nil {
		return
	}

	var c *config.Config
	var err error
	if configFile == "" {
		c, err = config.ReadDefaultConfig()
	} else {
		c, err = config.ReadConfig(configFile)
	}
	if err != nil {
		logger.Fatalf("Error: unable to read config: %v\n", err)
		return
	}
	logger.Printf("Using config at %s\n", c.ConfigPath)

	tasks := runtime.registry.Tasks()
	if len(tasks) == 0 {
		logger.Fatalln("Error: no task definitions found.")
		return
	}

	if listTasks && listAllTasks {
		logger.Fatalf("--list and --listAll cannot be specified at the same time. Please only use one.")
		return
	}

	if listTasks || listAllTasks {
		outputString := "Run specified tasks with `taskrunner taskname1 taskname2`\nTasks available:"
		for _, task := range tasks {
			if listTasks && task.Hidden {
				continue
			}
			outputString = outputString + "\n\t" + task.Name
		}
		fmt.Println(outputString)
		return
	}

	if describeTasks {
		w := tabwriter.NewWriter(os.Stdout, 0, 3, 0, ' ', 0)

		fmt.Fprintln(w, "\tTask name\tDescription")
		for _, task := range tasks {
			fmt.Fprintf(w, "\t%s\t%s\n", task.Name, task.Description)
		}
		if err := w.Flush(); err != nil {
			logger.Fatalf("unable to flush tabwriter: \n%v\n", err)
			return
		}
		return
	}

	taskAndFlagArgs := runtime.flags.Args() // command-line arguments that are not flags for taskrunner itself
	taskFlagGroups, foundInvalidTasks := runtime.groupTaskAndFlagArgs(taskAndFlagArgs)
	var desiredTasks []string
	for taskName := range taskFlagGroups {
		desiredTasks = append(desiredTasks, taskName)
	}
	watchMode := watch
	if len(desiredTasks) == 0 {
		if foundInvalidTasks { // tasks were specified, but all invalid
			logger.Fatalln("Error: invalid target task(s)")
			return
		} else { // no tasks were specified
			desiredTasks = c.DesiredTasks
			watchMode = !nonInteractive
		}
	}
	logger.Println("Desired tasks:", strings.Join(desiredTasks, ", "))
	logger.Printf("Watch mode: %t\n", watchMode)

	// Set task/option groups on executor
	ExecutorOptions(func(e *Executor) {
		e.taskFlagArgs = taskFlagGroups
	})(runtime)
	// Set supported task options registry on executor
	ExecutorOptions(func(e *Executor) {
		e.taskFlagsRegistry = runtime.registry.flagsByTask
	})(runtime)
	executorOptions := append([]ExecutorOption{WithWatchMode(watchMode)}, runtime.executorOptions...)
	executor := NewExecutor(c, tasks, executorOptions...)

	ctx, cancel := context.WithCancel(context.Background())
	onInterruptSignal(cancel)

	g, ctx := errgroup.WithContext(ctx)
	for i := range runtime.subscriptions {
		sub := runtime.subscriptions[i]
		ch := executor.Subscribe()
		g.Go(func() error {
			if err := sub(ch); err != nil {
				return oops.Wrapf(err, "subscription error")
			}
			return nil
		})
	}

	g.Go(func() error {
		err := executor.Run(ctx, desiredTasks, runtime)

		// We only care about propagating errors up to the errgroup
		// if it's a well-known executor error, or the underlying task failed AND
		// we're not in watch mode.
		if oops.Cause(err) == errUndefinedTaskName || !watchMode {
			return oops.Wrapf(err, "running executor")
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		logger.Fatalf("run error:\n%v\n", err)
		return
	}
}

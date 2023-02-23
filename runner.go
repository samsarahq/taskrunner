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
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: taskrunner [task...]\n")
		r.flags.PrintDefaults()
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
// Notably, this function does not group flags passed to taskrunner itself.
// Those flags are stored via the flags package and are handled separately.
func (r *Runtime) groupTaskAndFlagArgs(args []string) map[string][]string {
	flagArgsPerTask := map[string][]string{}

	var currTaskName string
	var currFlagsList []string
	for _, arg := range args {
		if currTaskName != "" && (strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "--")) {
			// If we have identified a current task and we think the arg is a flag,
			// add it to the list of flags we are storing for the current task.
			// Notably, if the first flags are options to taskrunner, currTaskName will be ""
			// and we will not group those flags with any tasks.
			currFlagsList = append(currFlagsList, arg)
		} else {
			if currTaskName != "" {
				// If this arg is a new task, store the flags we've collected for prev task.
				flagArgsPerTask[currTaskName] = currFlagsList
			}
			currTaskName = arg
			currFlagsList = []string{}
		}
	}

	// Ensure the we register the flags passed to the last task.
	if currTaskName != "" {
		flagArgsPerTask[currTaskName] = currFlagsList
	}

	// Return map of task to list of flags passed to it.
	return flagArgsPerTask
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
		log.Fatalf("config error: unable to read config:\n%v\n", err)
	}

	tasks := runtime.registry.Tasks()

	if listTasks && listAllTasks {
		log.Fatalf("--list and --listAll cannot be specified at the same time. Please only use one.")
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
			log.Fatalf("unable to flush tabwriter: \n%v\n", err)
		}
		return
	}

	log.Println("Using config", c.ConfigPath)
	taskFlagGroups := runtime.groupTaskAndFlagArgs(os.Args[1:])
	var desiredTasks []string
	for taskName := range taskFlagGroups {
		desiredTasks = append(desiredTasks, taskName)
	}
	var watchMode bool
	if len(desiredTasks) == 0 {
		desiredTasks = c.DesiredTasks
		watchMode = !nonInteractive
	} else {
		watchMode = watch
	}

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

	if len(tasks) == 0 {
		log.Fatalln("No tasks specified")
	}
	log.Println("Desired tasks:", strings.Join(desiredTasks, ", "))
	log.Printf("Watch mode: %t", watchMode)

	ctx, cancel := context.WithCancel(context.Background())
	onInterruptSignal(cancel)

	g, ctx := errgroup.WithContext(ctx)
	for i := range runtime.subscriptions {
		sub := runtime.subscriptions[i]
		ch := executor.Subscribe()
		g.Go(func() error {
			return sub(ch)
		})
	}

	g.Go(func() error {
		err := executor.Run(ctx, desiredTasks, runtime)

		// We only care about propagating errors up to the errgroup
		// if it's a well-known executor error, or the underlying task failed AND
		// we're not in watch mode.
		if oops.Cause(err) == errUndefinedTaskName || !watchMode {
			return err
		}

		return nil
	})

	if err := g.Wait(); err != nil {
		log.Fatalf("run error:\n%v\n", err)
	}
}

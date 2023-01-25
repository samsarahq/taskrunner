package taskrunner

import (
	"fmt"
	"sort"
)

// DefaultRegistry is where tasks are registered to through the package-level functions Add, Group
// and Tasks.
var DefaultRegistry = NewRegistry()

// NewRegistry creates a new task registry.
func NewRegistry() *Registry {
	return &Registry{
		definitions: make(map[string]*Task),
		flagsByTask: make(map[string]map[string]TaskFlag),
	}
}

// Registry is an object that keeps track of task definitions.
type Registry struct {
	definitions map[string]*Task
	flagsByTask map[string]map[string]TaskFlag
}

type TaskOption func(*Task) *Task

// Add registers a task, failing if the name has already been taken.
func (r *Registry) Add(t *Task, opts ...TaskOption) *Task {
	for _, opt := range opts {
		t = opt(t)
	}
	if _, ok := r.definitions[t.Name]; ok {
		panic(fmt.Sprintf("Duplicate task registered: %s", t.Name))
	}
	r.definitions[t.Name] = t

	r.flagsByTask[t.Name] = map[string]TaskFlag{}
	for _, flag := range t.Flags {
		// Validate that the following task TaskFlag fields are provided:
		// - LongName or ShortName
		// - Disallow --help/-h flags
		// - Description
		// - ValueType
		if flag.LongName == "" && flag.ShortName == 0 {
			panic(fmt.Sprintf("Neither LongName nor ShortName are defined for an flag on %s. At least one name must be defined.", t.Name))
		}

		if flag.LongName != "" && flag.LongName == "help" {
			panic("The long flag name `help` is reserved.")
		}

		if flag.ShortName != 0 && string(flag.ShortName) == "h" {
			panic("The short lag name `h` is reserved.")
		}

		if flag.Description == "" {
			var optionName string
			if flag.LongName != "" {
				optionName = flag.LongName
			} else {
				optionName = string(flag.ShortName)
			}
			panic(fmt.Sprintf("Please provide a description for the `%s` flag", optionName))
		}

		if flag.ValueType != StringTypeFlag && flag.ValueType != BoolTypeFlag && flag.ValueType != IntTypeFlag {
			panic("Please set the flag ValueType with `taskrunner.StringTypeFlag`, `taskrunner.BoolTypeFlag` or `taskrunner.IntTypeFlag`.")
		}

		// Validate that there are no duplicate LongNames
		if flag.LongName != "" {
			_, ok := r.flagsByTask[t.Name][flag.LongName]
			if ok {
				panic(fmt.Sprintf("Duplicate flag LongName registered: %s", flag.LongName))
			}
			r.flagsByTask[t.Name][flag.LongName] = flag
		}

		// Validate that there are no duplicate ShortNames
		if flag.ShortName != 0 {
			_, ok := r.flagsByTask[t.Name][string(flag.ShortName)]
			if ok {
				panic(fmt.Sprintf("Duplicate flag ShortName registered: %s", string(flag.ShortName)))
			}
			r.flagsByTask[t.Name][string(flag.ShortName)] = flag
		}
	}

	return t
}

// Group creates a pseudo-task that groups other tasks underneath it. It explicitly doesn't expose
// the pseudo-task because groups are not allowed to be dependencies.
func (r *Registry) Group(name string, tasks ...*Task) {
	r.Add(&Task{
		Name:         name,
		Dependencies: tasks,
		IsGroup:      true,
	})
}

// Tasks returns all registered tasks in alphabetical order.
func (r *Registry) Tasks() (list []*Task) {
	for _, t := range r.definitions {
		list = append(list, t)
	}
	sort.Slice(list, func(i, j int) bool {
		return list[i].Name < list[j].Name
	})
	return list
}

// Add registers a task, failing if the name has already been taken.
var Add = DefaultRegistry.Add

// Group creates a pseudo-task that groups other tasks underneath it.
var Group = DefaultRegistry.Group

// Tasks returns all registered tasks in alphabetical order.
var Tasks = DefaultRegistry.Tasks

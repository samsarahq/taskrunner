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
	}
}

// Registry is an object that keeps track of task definitions.
type Registry struct {
	definitions map[string]*Task
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

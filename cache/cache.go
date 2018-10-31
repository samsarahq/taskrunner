package cache

import (
	"context"
	"log"
	"os"
	"path"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/shell"
)

const CachePath = ".cache/taskrunner"

var CacheDir = path.Join(os.Getenv("HOME"), CachePath)

type Cache struct {
	ranOnce     map[*taskrunner.Task]bool
	snapshotter *snapshotter
	cacheFile   string
	allDirty    bool
	dirtyFiles  []string
}

func New() *Cache {
	return &Cache{
		ranOnce:   make(map[*taskrunner.Task]bool),
		cacheFile: path.Join(CacheDir, "example.json"),
	}
}

func (c *Cache) Start(ctx context.Context, opt shell.RunOption) error {
	c.snapshotter = newSnapshotter(
		func(ctx context.Context, command string, opts ...shell.RunOption) error {
			return shell.Run(ctx, command, append(opts, opt)...)
		},
	)

	s, err := c.snapshotter.Read(c.cacheFile)
	// If we can't get a cache file, assume that everything is dirty and needs to be re-run.
	if err != nil {
		c.allDirty = true
		s = &snapshot{}
	}

	// Truncate the snapshot after we read it in order to prevent a stale cache, should taskrunner
	// be terminated unexpectedly.
	_ = os.Truncate(c.cacheFile, 0)

	files, err := c.snapshotter.Diff(ctx, s)
	if err != nil {
		return err
	}
	c.dirtyFiles = files

	return nil
}

// Finish creates and saves the cache state.
func (c *Cache) Finish(ctx context.Context) error {
	if err := os.MkdirAll(CacheDir, os.ModePerm); err != nil {
		return err
	}
	return c.snapshotter.Write(ctx, c.cacheFile)
}

func (c *Cache) isFirstRun(task *taskrunner.Task) bool {
	ran := c.ranOnce[task]
	c.ranOnce[task] = true
	return !ran
}

func (c *Cache) isValid(task *taskrunner.Task) bool {
	if c.allDirty {
		return false
	}
	for _, f := range c.dirtyFiles {
		if taskrunner.IsTaskSource(task, f) {
			return false
		}
	}
	return true
}

func (c *Cache) maybeRun(task *taskrunner.Task) func(context.Context, shell.ShellRun) error {
	return func(ctx context.Context, shellRun shell.ShellRun) error {
		if c.isFirstRun(task) && c.isValid(task) {
			// report that the task wasn't run
			return shellRun(ctx, `echo "no changes (cache)"`)
		}
		return task.Run(ctx, shellRun)
	}
}

// WrapWithPersistentCache prevents the task from being invalidated between runs if the files it
// depends on don't change.
func (c *Cache) WrapWithPersistentCache(task *taskrunner.Task) *taskrunner.Task {
	if len(task.Sources) == 0 {
		log.Fatalf("Task %s cannot be wrapped with a persistent cache as it has no sources", task.Name)
	}
	newTask := *task
	newTask.Run = c.maybeRun(task)
	return &newTask
}

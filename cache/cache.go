package cache

import (
	"context"
	"fmt"
	"log"
	"os"
	"path"
	"strings"
	"sync"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/shell"
)

const CachePath = ".cache/taskrunner"

var CacheDir = path.Join(os.Getenv("HOME"), CachePath)

type Cache struct {
	ranOnce      map[*taskrunner.Task]bool
	snapshotter  *snapshotter
	cacheFile    string
	allDirty     bool
	dirtyFiles   []string
	ranOnceMutex sync.Mutex

	opts []shell.RunOption
}

// Ignore ignores the cache (useful for conditionally bypassing the cache).
func (c *Cache) Ignore() { c.allDirty = true }

func New(opts ...shell.RunOption) *Cache {
	return &Cache{
		ranOnce: make(map[*taskrunner.Task]bool),
		opts:    opts,
	}
}

func (c *Cache) Option(r *taskrunner.Runtime) {
	r.OnStart(func(ctx context.Context, executor *taskrunner.Executor) error {
		c.cacheFile = getCacheFilePath(executor.Config().WorkingDir)
		return c.Start(ctx)
	})
	r.OnStop(func(ctx context.Context, executor *taskrunner.Executor) error {
		return c.Finish(ctx)
	})
}

func getCacheFilePath(dir string) string {
	hashedName := strings.Replace(dir, "/", "%", -1)
	return path.Join(CacheDir, hashedName)
}

func (c *Cache) Start(ctx context.Context) error {
	c.snapshotter = newSnapshotter(
		func(ctx context.Context, command string, opts ...shell.RunOption) error {
			return shell.Run(ctx, command, append(opts, c.opts...)...)
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
	c.ranOnceMutex.Lock()
	defer c.ranOnceMutex.Unlock()
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
			logger := taskrunner.LoggerFromContext(ctx)
			if logger != nil {
				fmt.Fprintln(logger.Stdout, "no changes (cache)")
				return nil
			}
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

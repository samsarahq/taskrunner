package taskrunner

import (
	"context"
	"log"

	zglob "github.com/mattn/go-zglob"
	"github.com/samsarahq/taskrunner/watcher"
)

// IsTaskSource checks if a provided path matches a source glob for the task.
func IsTaskSource(task *Task, path string) (matches bool) {
	for _, source := range task.Sources {
		if ok, err := zglob.Match(source, path); err != nil {
			log.Fatalf("invalid glob (%s):\n%v\n", path, err)
		} else if ok {
			matches = true
		}
	}
	for _, ignore := range task.Ignore {
		if ok, err := zglob.Match(ignore, path); err != nil {
			log.Fatalf("invalid glob (%s):\n%v\n", path, err)
		} else if ok {
			matches = false
		}
	}
	return matches
}

func (e *Executor) runWatch(ctx context.Context) {
	watcher := watcher.NewWatcher(e.config.WorkingDir)
	for _, enhancer := range e.watcherEnhancers {
		watcher = enhancer(watcher)
	}

	go func() {
		for event := range watcher.Events() {
			for task := range e.tasks {
				if IsTaskSource(task, event.RelativeFilename) {
					go e.Invalidate(task, FileChange{
						File: event.RelativeFilename,
					})
				}
			}
		}
	}()

	e.wg.Go(func() error {
		if err := watcher.Run(ctx); err != nil {
			if ctx.Err() != context.Canceled {
				return err
			}
		}
		return nil
	})
}

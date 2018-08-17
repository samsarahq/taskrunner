package taskrunner

import (
	"context"

	zglob "github.com/mattn/go-zglob"
	"github.com/samsarahq/taskrunner/watcher"
)

func (e *Executor) runWatch(ctx context.Context) {
	watcher := watcher.NewWatcher(e.config.projectPath())
	go func() {
		for event := range watcher.Events() {
			for task := range e.tasks {
				matches := false
				for _, source := range task.Sources {
					if ok, err := zglob.Match(source, event.RelativeFilename); err != nil {
						panic(err)
					} else if ok {
						matches = true
					}
				}
				for _, ignore := range task.Ignore {
					if ok, err := zglob.Match(ignore, event.RelativeFilename); err != nil {
						panic(err)
					} else if ok {
						matches = false
					}
				}

				if matches {
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

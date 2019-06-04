package watcher

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/samsarahq/go/oops"
	"golang.org/x/sync/errgroup"
)

type NumericEventFlag uint64

const (
	NumericEventFlagInvalid           = NumericEventFlag(0)
	NumericEventFlagPlatformSpecific  = NumericEventFlag(1)
	NumericEventFlagCreated           = NumericEventFlag(2)
	NumericEventFlagUpdated           = NumericEventFlag(4)
	NumericEventFlagRemoved           = NumericEventFlag(8)
	NumericEventFlagRenamed           = NumericEventFlag(16)
	NumericEventFlagOwnerModified     = NumericEventFlag(32)
	NumericEventFlagAttributeModified = NumericEventFlag(64)
	NumericEventFlagMovedFrom         = NumericEventFlag(128)
	NumericEventFlagMovedTo           = NumericEventFlag(256)
	NumericEventFlagIsFile            = NumericEventFlag(512)
	NumericEventFlagIsDir             = NumericEventFlag(1024)
	NumericEventFlagIsSymLink         = NumericEventFlag(2048)
	NumericEventFlagIsLink            = NumericEventFlag(4096)
)

type WatchEvent struct {
	ReceivedAt       time.Time
	Flags            NumericEventFlag
	RelativeFilename string
}

type Watcher interface {
	Events() <-chan WatchEvent
	Run(context.Context) error
}

type FSEventsWatcher struct {
	directory string
	eventsCh  chan WatchEvent
}

func NewFSEventsWatcher(absoluteDirectory string) *FSEventsWatcher {
	return &FSEventsWatcher{
		directory: absoluteDirectory,
		eventsCh:  make(chan WatchEvent, 1024),
	}
}

func (w *FSEventsWatcher) Events() <-chan WatchEvent {
	return w.eventsCh
}

func (w *FSEventsWatcher) Run(ctx context.Context) error {
	defer close(w.eventsCh)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)

	cmd := exec.CommandContext(ctx, "fswatch", "-n", "-interval", "0.1", w.directory)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return oops.Wrapf(err, "")
	}
	cmd.Stderr = os.Stderr

	g.Go(func() error {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.Split(scanner.Text(), " ")

			relFilename, err := filepath.Rel(w.directory, line[0])
			if err != nil {
				return oops.Wrapf(err, "filepath.Rel: %s, %s", w.directory, line[0])
			}

			eventFlags, err := strconv.ParseUint(line[1], 10, 64)
			if err != nil {
				return oops.Wrapf(err, "strconv.ParseUint: %s", line[1])
			}

			w.eventsCh <- WatchEvent{
				ReceivedAt:       time.Now(),
				Flags:            NumericEventFlag(eventFlags),
				RelativeFilename: relFilename,
			}
		}
		return oops.Wrapf(scanner.Err(), "scanner.Err()")
	})

	g.Go(func() error {
		return oops.Wrapf(cmd.Run(), "")
	})

	return g.Wait()
}

type INotifyWatcher struct {
	directory string
	eventsCh  chan WatchEvent
}

func NewINotifyWatcher(absoluteDirectory string) *INotifyWatcher {
	return &INotifyWatcher{
		directory: absoluteDirectory,
		eventsCh:  make(chan WatchEvent, 1024),
	}
}

func (w *INotifyWatcher) Events() <-chan WatchEvent {
	return w.eventsCh
}

func (w *INotifyWatcher) Run(ctx context.Context) error {
	defer close(w.eventsCh)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	g, ctx := errgroup.WithContext(ctx)

	// XXX: hack: ignore node_modules because inotify has a 8192 limit by default.
	cmd := exec.CommandContext(ctx, "inotifywait", "-q", "-m", "--exclude", "(node_modules|npm-packages-offline-cache)", "-r", "-e", "modify", "-e", "create", "-e", "delete", "-e", "move", w.directory)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return oops.Wrapf(err, "")
	}
	cmd.Stderr = os.Stderr

	g.Go(func() error {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := strings.Split(scanner.Text(), " ")

			relFilename, err := filepath.Rel(w.directory, filepath.Join(line[0], line[2]))
			if err != nil {
				return oops.Wrapf(err, "filepath.Rel: %s, %s, %s", w.directory, line[0], line[2])
			}

			eventFlag := map[string]NumericEventFlag{
				"ATTRIB":    NumericEventFlagAttributeModified,
				"CREATE":    NumericEventFlagCreated,
				"DELETE":    NumericEventFlagRemoved,
				"MOVED_TO":   NumericEventFlagMovedTo,
				"MOVED_FROM": NumericEventFlagMovedFrom,
				"MODIFY":    NumericEventFlagUpdated,
			}[line[1]]

			if eventFlag == NumericEventFlagInvalid {
				return oops.Wrapf(err, "unknown inotifywait event: %s", line[1])
			}

			w.eventsCh <- WatchEvent{
				ReceivedAt:       time.Now(),
				Flags:            eventFlag,
				RelativeFilename: relFilename,
			}
		}
		return oops.Wrapf(scanner.Err(), "scanner.Err()")
	})

	g.Go(func() error {
		return oops.Wrapf(cmd.Run(), "")
	})

	return g.Wait()
}

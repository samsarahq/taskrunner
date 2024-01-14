// +build darwin

package watcher

func NewWatcher(absoluteDirectory string) Watcher {
	return NewFSEventsWatcher(absoluteDirectory)
}

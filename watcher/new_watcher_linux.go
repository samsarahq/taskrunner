// +build linux

package watcher

func NewWatcher(absoluteDirectory string) Watcher {
	return NewINotifyWatcher(absoluteDirectory)
}

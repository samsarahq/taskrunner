package taskrunner

import "fmt"

type InvalidationReason int

const (
	InvalidationReason_Invalid InvalidationReason = iota
	InvalidationReason_FileChange
	InvalidationReason_DependencyChange
	InvalidationReason_KeepAliveStopped
	InvalidationReason_UserRestart
)

type InvalidationEvent interface {
	Reason() InvalidationReason
	Description() string
}

type FileChange struct {
	File string
}

func (f FileChange) Reason() InvalidationReason {
	return InvalidationReason_FileChange
}

func (f FileChange) Description() string {
	return fmt.Sprintf("file %s changed", f.File)
}

type DependencyChange struct {
	Source *Task
}

func (f DependencyChange) Reason() InvalidationReason {
	return InvalidationReason_DependencyChange
}

func (f DependencyChange) Description() string {
	return fmt.Sprintf("dependency %s was also invalidated", f.Source.Name)
}

type KeepAliveStopped struct{}

func (f KeepAliveStopped) Reason() InvalidationReason {
	return InvalidationReason_KeepAliveStopped
}

func (f KeepAliveStopped) Description() string {
	return fmt.Sprintf("task exited, but is marked as keep alive")
}

type UserRestart struct{}

func (f UserRestart) Reason() InvalidationReason {
	return InvalidationReason_UserRestart
}

func (f UserRestart) Description() string {
	return fmt.Sprintf("user requested task restart")
}

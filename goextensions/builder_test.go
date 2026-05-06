package goextensions_test

import (
	"context"
	"testing"

	"github.com/samsarahq/taskrunner"
	"github.com/samsarahq/taskrunner/goextensions"
	"github.com/samsarahq/taskrunner/shell"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWrapWithGoBuild_ShouldInvalidate(t *testing.T) {
	builder := goextensions.NewGoBuilder()
	wrapper := builder.WrapWithGoBuild("github.com/samsarahq/taskrunner/goextensions/foo")

	task := wrapper(&taskrunner.Task{
		Run: func(_ context.Context, _ shell.ShellRun) error {
			return nil
		},
	})
	assert.NotNil(t, task)

	// Before the task has run, dependencies are unknown — invalidate on any Go file change.
	assert.Empty(t, task.Sources)
	assert.True(t, task.ShouldInvalidate(taskrunner.FileChange{
		File: "github.com/samsarahq/taskrunner/goextensions/unrelated/baz.go",
	}))

	err := task.Run(context.Background(), shell.Run)
	require.NoError(t, err)

	// Sources is intentionally left empty; non-dep filtering is done via ShouldInvalidate.
	assert.Empty(t, task.Sources)
	assert.False(t, task.ShouldInvalidate(taskrunner.FileChange{
		File: "github.com/samsarahq/taskrunner/goextensions/unrelated/baz.go",
	}))
	assert.True(t, task.ShouldInvalidate(taskrunner.FileChange{
		File: "github.com/samsarahq/taskrunner/goextensions/foo/foo.go",
	}))
	assert.True(t, task.ShouldInvalidate(taskrunner.FileChange{
		File: "github.com/samsarahq/taskrunner/goextensions/foo/bar/bar.go",
	}))
}

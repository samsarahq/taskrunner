package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

var errGitFailed = errors.New("git rev-parse HEAD failed: exit status 128; stderr: fatal: not a git repository")

func failingSnapshotter() *snapshotter {
	return &snapshotter{
		CommitFunc: func(ctx context.Context) (string, error) {
			return "", errGitFailed
		},
		UncommittedFilesFunc: func(ctx context.Context) ([]string, []string, error) {
			return nil, nil, nil
		},
		HashFunc: func(s string) (string, error) { return s, nil },
	}
}

func TestCache_StartGitFailureIsNonFatal(t *testing.T) {
	c := New()
	c.snapshotter = failingSnapshotter()

	err := c.Start(context.Background())
	assert.NoError(t, err)
	assert.True(t, c.allDirty, "expected allDirty=true when git fails")
}

func TestCache_FinishGitFailureIsNonFatal(t *testing.T) {
	c := New()
	c.snapshotter = failingSnapshotter()
	c.cacheFile = t.TempDir() + "/cache"

	err := c.Finish(context.Background())
	assert.NoError(t, err)
}

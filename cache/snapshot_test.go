package cache

import (
	"context"
	"testing"

	snapper "github.com/samsarahq/go/snapshotter"
	"github.com/stretchr/testify/assert"
)

func TestSnapshotter_snapshot(t *testing.T) {
	snap := snapper.New(t)
	defer snap.Verify()

	s := snapshotter{
		CommitFunc: func(context.Context) (string, error) {
			return "commit-sha", nil
		},
		UncommittedFilesFunc: func(context.Context) (newFiles []string, modifiedFiles []string, err error) {
			return []string{
					"yay",
					"no/bar.go",
					"no/baz.go",
				}, []string{
					"foo",
					"test/bar.go",
					"test/baz.go",
				}, nil
		},
		HashFunc: func(str string) (string, error) {
			return "hashed" + str, nil
		},
	}

	snapshot, err := s.snapshot(context.TODO(), true)
	assert.NoError(t, err)
	snap.Snapshot("snapshot with modified files", snapshot)

	snapshot, err = s.snapshot(context.TODO(), false)
	assert.NoError(t, err)
	snap.Snapshot("snapshot without modified files", snapshot)
}

func arrToUncommitted(from []string) (to []uncommittedFile) {
	for _, f := range from {
		to = append(to, uncommittedFile{Path: f, MD5: f})
	}
	return to
}

func TestSnapshotter_diff(t *testing.T) {
	testcases := []struct {
		Name                     string
		DiffFiles                []string
		CurrentUncommittedFiles  []string
		PreviousUncommittedFiles []string

		Expected []string
	}{
		{
			Name:                     "clean git state",
			DiffFiles:                nil,
			CurrentUncommittedFiles:  nil,
			PreviousUncommittedFiles: nil,
			Expected:                 nil,
		},
		{
			Name:                     "dirty diff",
			DiffFiles:                []string{"foo"},
			CurrentUncommittedFiles:  nil,
			PreviousUncommittedFiles: nil,
			Expected:                 []string{"foo"},
		},
		{
			Name:                     "no difference in uncommitted files",
			DiffFiles:                nil,
			CurrentUncommittedFiles:  []string{"foo"},
			PreviousUncommittedFiles: []string{"foo"},
			Expected:                 nil,
		},
		{
			Name:                     "difference in uncommitted files",
			DiffFiles:                nil,
			CurrentUncommittedFiles:  []string{"diff"},
			PreviousUncommittedFiles: []string{"diff"},
			Expected:                 []string{"diff"},
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.Name, func(t *testing.T) {
			s := snapshotter{
				CommitFunc:           func(context.Context) (string, error) { return "commit", nil },
				DiffFunc:             func(context.Context, string) ([]string, error) { return testcase.DiffFiles, nil },
				UncommittedFilesFunc: func(context.Context) ([]string, []string, error) { return testcase.CurrentUncommittedFiles, nil, nil },
				HashFunc: func(in string) (string, error) {
					if in == "diff" {
						return "diff+erent", nil
					}
					return in, nil
				},
			}
			previous := &snapshot{UncommittedFiles: arrToUncommitted(testcase.PreviousUncommittedFiles)}
			previous.loadMap()
			diff, err := s.Diff(context.TODO(), previous)
			assert.NoError(t, err)
			assert.Equal(t, testcase.Expected, diff)
		})
	}
}

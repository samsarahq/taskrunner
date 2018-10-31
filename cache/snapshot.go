package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"

	"github.com/samsarahq/taskrunner/shell"
)

// snapshotter tracks the state of a git repository via snapshots.
type snapshotter struct {
	// CommitFunc gets the current commit SHA.
	CommitFunc func(context.Context) (sha string, err error)
	// DiffFunc gets modified files against a previous commit SHA.
	DiffFunc func(context.Context, string) (diffFiles []string, err error)
	// UncommittedFilesFunc gets a list of all uncommitted files (new and modified).
	UncommittedFilesFunc func(context.Context) (newFiles []string, modifiedFiles []string, err error)
	// HashFunc gets an MD5 hash of the file or directory.
	HashFunc func(string) (hash string, err error)
}

// snapshot is the state of a git repository in time. It records the current commit as well as MD5
// sums of uncommited files or directories (hashed by name + timestamp).
// When comparing against a previous snapshot, we can therefore run a git diff against the old sha
// and compare uncommitted files manually.
type snapshot struct {
	// Commit SHA at HEAD.
	CommitSha string `json:"commitSha"`
	// Uncommitted files at the time of snapshotting.
	UncommittedFiles []uncommittedFile `json:"uncommittedFiles"`
	// A map representation of UncomittedFiles for quick lookup: map[filename]md5hash.
	uncommittedFilesMap map[string]string
}

func newSnapshotter(shellRun shell.ShellRun) *snapshotter {
	client := gitClient{shellRun: shellRun}

	return &snapshotter{
		DiffFunc:             client.diff,
		CommitFunc:           client.currentCommit,
		UncommittedFilesFunc: client.uncomittedFiles,
		HashFunc:             hashSum,
	}
}

// diff compares against another snapshot with the current state.
func (c *snapshotter) Diff(ctx context.Context, previous *snapshot) ([]string, error) {
	current, err := c.snapshot(ctx, false)
	if err != nil {
		return nil, err
	}

	gitDiffFiles, err := c.DiffFunc(ctx, previous.CommitSha)
	if err != nil {
		return nil, err
	}

	// Track files that have changed since the last snapshot.
	var modifiedFiles []string

	// Only mark committed files as modified (uncommitted files are handled below).
	for _, f := range gitDiffFiles {
		if previous.hashFor(f) == "" {
			modifiedFiles = append(modifiedFiles, f)
		}
	}

	// Because we diff against the last commit, any files that were not committed the
	// last time a snapshot was recorded needs to compare hashes instead.
	for _, file := range previous.UncommittedFiles {
		// If the file isn't currently uncommitted, rehash.
		md5 := current.hashFor(file.Path)
		if md5 == "" {
			md5, err = c.HashFunc(file.Path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Warning: unable to hash file %s (error: %v)\n", file.Path, err)
			}
		}
		if md5 == "" || md5 != file.MD5 {
			modifiedFiles = append(gitDiffFiles, file.Path)
		}
	}

	for _, file := range current.UncommittedFiles {
		// Any uncommitted file that either has a different MD5 hash or wasn't recorded in the
		// previous snapshot counts as different.
		if sha := previous.hashFor(file.Path); sha == "" || sha != file.MD5 {
			modifiedFiles = append(gitDiffFiles, file.Path)
		}
	}

	return modifiedFiles, err
}

// snapshot takes a snapshot of the current state.
// withChanged dictates whether we should include modified files in the snapshot (vs just new files).
func (c *snapshotter) snapshot(ctx context.Context, withChanged bool) (*snapshot, error) {
	var err error
	s := snapshot{}

	s.CommitSha, err = c.CommitFunc(ctx)
	if err != nil {
		return nil, err
	}

	newFiles, modifiedFiles, err := c.UncommittedFilesFunc(ctx)
	if err != nil {
		return nil, err
	}

	// We only want modified files for recording snapshots, not for diffing against an older
	// snapshot (since we can rely on git diff to handle those).
	if withChanged {
		newFiles = append(newFiles, modifiedFiles...)
	}

	for _, file := range newFiles {
		hash, err := c.HashFunc(file)
		if err != nil {
			return nil, err
		}
		s.UncommittedFiles = append(s.UncommittedFiles, uncommittedFile{
			Path: file,
			MD5:  hash,
		})
	}

	s.loadMap()

	return &s, nil
}

// write records the current state to a file.
func (c *snapshotter) Write(ctx context.Context, cacheFilePath string) error {
	// Create a snapshot including modified files.
	snapshot, err := c.snapshot(ctx, true)
	if err != nil {
		return err
	}

	b, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(cacheFilePath, b, 0644)
}

// read gets a previous state from a file.
func (c *snapshotter) Read(path string) (*snapshot, error) {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var snapshot snapshot
	if err := json.Unmarshal(b, &snapshot); err != nil {
		return nil, err
	}
	snapshot.loadMap()

	return &snapshot, nil
}

func (s *snapshot) hashFor(path string) string {
	return s.uncommittedFilesMap[path]
}

func (s *snapshot) loadMap() {
	s.uncommittedFilesMap = map[string]string{}
	for _, f := range s.UncommittedFiles {
		s.uncommittedFilesMap[f.Path] = f.MD5
	}
}

type uncommittedFile struct {
	Path string `json:"path"`
	MD5  string `json:"md5"`
}

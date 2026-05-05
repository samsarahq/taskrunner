package cache

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/taskrunner/shell"
)

type gitClient struct {
	shellRun shell.ShellRun
}

func stripStdout(buf bytes.Buffer) string {
	return strings.Trim(buf.String(), "\n")
}

func splitStdout(buf bytes.Buffer) []string {
	return strings.Split(stripStdout(buf), "\n")
}

func (g gitClient) currentCommit(ctx context.Context) (commitHash string, err error) {
	var buffer, stderr bytes.Buffer
	if err := g.shellRun(ctx, "git rev-parse HEAD", shell.Stdout(&buffer), shell.Stderr(&stderr)); err != nil {
		return "", oops.Wrapf(err, "git rev-parse HEAD; stderr: %s", stderr.String())
	}

	return stripStdout(buffer), nil
}

func (g gitClient) diff(ctx context.Context, commitHash string) (modifiedFiles []string, error error) {
	var buffer, stderr bytes.Buffer
	if err := g.shellRun(ctx, fmt.Sprintf("git diff --name-only %s", commitHash), shell.Stdout(&buffer), shell.Stderr(&stderr)); err != nil {
		return nil, oops.Wrapf(err, "git diff --name-only %s; stderr: %s", commitHash, stderr.String())
	}

	return splitStdout(buffer), nil
}

func (g gitClient) uncomittedFiles(ctx context.Context) (newFiles []string, modifiedFiles []string, err error) {
	var buffer, stderr bytes.Buffer
	if err := g.shellRun(ctx, "git status --porcelain", shell.Stdout(&buffer), shell.Stderr(&stderr)); err != nil {
		return nil, nil, oops.Wrapf(err, "git status --porcelain; stderr: %s", stderr.String())
	}

	for _, statusLine := range splitStdout(buffer) {
		if len(strings.TrimSpace(statusLine)) < 4 {
			continue
		}
		if strings.HasPrefix(statusLine, "??") {
			newFiles = append(newFiles, statusLine[3:])
		} else {
			modifiedFiles = append(modifiedFiles, statusLine[3:])
		}
	}

	return newFiles, modifiedFiles, nil
}

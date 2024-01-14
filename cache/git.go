package cache

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/ChenJesse/taskrunner/shell"
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
	var buffer bytes.Buffer
	if err := g.shellRun(ctx, "git rev-parse HEAD", shell.Stdout(&buffer)); err != nil {
		return "", err
	}

	return stripStdout(buffer), nil
}

func (g gitClient) diff(ctx context.Context, commitHash string) (modifiedFiles []string, error error) {
	var buffer bytes.Buffer
	if err := g.shellRun(ctx, fmt.Sprintf("git diff --name-only %s", commitHash), shell.Stdout(&buffer)); err != nil {
		return nil, err
	}

	return splitStdout(buffer), nil
}

func (g gitClient) uncomittedFiles(ctx context.Context) (newFiles []string, modifiedFiles []string, err error) {
	var buffer bytes.Buffer
	if err := g.shellRun(ctx, "git status --porcelain", shell.Stdout(&buffer)); err != nil {
		return nil, nil, err
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

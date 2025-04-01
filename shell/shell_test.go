package shell

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRunShell(t *testing.T) {
	var buffer bytes.Buffer
	assert.NoError(t, Run(context.Background(), "echo 1", Stdout(&buffer)))
	assert.Equal(t, "1\n", buffer.String())
}

func TestRunShellInterpreterError(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			assert.True(t, ok)
			assert.Contains(t, err.Error(), "failed to set up interpreter")
		}
	}()
	Run(context.Background(), "")
}

func TestRunShellParseError(t *testing.T) {
	err := Run(context.Background(), "`invalid command")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse shell command")
}

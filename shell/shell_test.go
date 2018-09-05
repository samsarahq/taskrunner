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

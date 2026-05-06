package goextensions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsStdLib(t *testing.T) {
	assert.True(t, isStdLib("testing"))
	assert.True(t, isStdLib("strings"))
	assert.False(t, isStdLib("github.com/samsarahq/thunder"))
}

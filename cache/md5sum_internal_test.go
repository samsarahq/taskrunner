package cache

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHashSum(t *testing.T) {
	hash, err := hashSum("testdata/md5test/md5test.txt")
	assert.NoError(t, err)
	assert.Len(t, hash, 32)

	hash, err = hashSum("testdata/md5test")
	assert.NoError(t, err)
	assert.Len(t, hash, 32)
}

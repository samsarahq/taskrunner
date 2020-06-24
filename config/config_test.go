package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestReadConfig(t *testing.T) {
	config, err := ReadConfig("testdata/base.taskrunner.json")
	assert.NoError(t, err)
	assert.True(t, strings.HasSuffix(config.WorkingDir, "testdata/hello"), "expected %s to end with testdata/hello", config.WorkingDir)
	assert.True(t, strings.HasSuffix(config.ConfigPath, "testdata/base.taskrunner.json"), "expected %s to be file path", config.ConfigPath)
	assert.Equal(t, []string{"basic"}, config.DesiredTasks)
}

func TestReadConfig_Extends(t *testing.T) {
	config, err := ReadConfig("testdata/extension.taskrunner.json")
	assert.NoError(t, err)
	assert.True(t, strings.HasSuffix(config.WorkingDir, "testdata/hello"), "expected %s to end with testdata/hello", config.WorkingDir)
	assert.True(t, strings.HasSuffix(config.ConfigPath, "testdata/extension.taskrunner.json"), "expected %s to be file path", config.ConfigPath)
	assert.Contains(t, config.DesiredTasks, "basic")
	assert.Contains(t, config.DesiredTasks, "advanced")
}

func TestReadConfig_ExtendsNested(t *testing.T) {
	config, err := ReadConfig("testdata/nested/nested.taskrunner.json")
	assert.NoError(t, err)
	assert.True(t, strings.HasSuffix(config.WorkingDir, "testdata/hello"), "expected %s to end with testdata/hello", config.WorkingDir)
	assert.True(t, strings.HasSuffix(config.ConfigPath, "testdata/nested/nested.taskrunner.json"), "expected %s to be file path", config.ConfigPath)
	assert.Contains(t, config.DesiredTasks, "something")
	assert.Contains(t, config.DesiredTasks, "basic")
}

func TestReadConfig_ExtendsChained(t *testing.T) {
	config, err := ReadConfig("testdata/chained.taskrunner.json")
	assert.NoError(t, err)
	assert.True(t, strings.HasSuffix(config.WorkingDir, "testdata/hello"), "expected %s to end with testdata/hello", config.WorkingDir)
	assert.True(t, strings.HasSuffix(config.ConfigPath, "testdata/chained.taskrunner.json"), "expected %s to be file path", config.ConfigPath)
	assert.Contains(t, config.DesiredTasks, "basic")
	assert.Contains(t, config.DesiredTasks, "advanced")
	assert.Contains(t, config.DesiredTasks, "quirky")
}

func TestReadConfig_ExtendsPath(t *testing.T) {
	config, err := ReadConfig("testdata/nested/path.taskrunner.json")
	assert.NoError(t, err)
	assert.True(t, strings.HasSuffix(config.WorkingDir, "testdata/nested/somewhere/else"), "expected %s to end with testdata/nested/somewhere/else", config.WorkingDir)
	assert.True(t, strings.HasSuffix(config.ConfigPath, "testdata/nested/path.taskrunner.json"), "expected %s to be file path", config.ConfigPath)
	assert.Contains(t, config.DesiredTasks, "basic")
	assert.Len(t, config.DesiredTasks, 1)
}

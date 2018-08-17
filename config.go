package taskrunner

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/samsarahq/go/oops"
	"github.com/samsarahq/taskrunner/shell"
	"mvdan.cc/sh/interp"
)

type LogMode string

const (
	LogMode_Stdout         LogMode = "stdout"
	LogMode_LogfilesAppend         = "logfiles-append"
	LogMode_LogfilesByDate         = "logfiles-bydate"
)

type Config struct {
	// configPath is the path to this configuration file, relative to the current working directory.
	configPath string

	Watch bool

	Path string `json:"path"`

	DesiredTasks []string `json:"desiredTasks"`

	LogMode LogMode `json:"logMode"`
}

func ReadUserConfig() (*Config, error) {
	workspaceConfig := "./workspace.taskrunner.json"
	userConfig := "./user.taskrunner.json"
	content, err := ioutil.ReadFile(workspaceConfig)
	if err != nil {
		return nil, err
	}

	config := Config{configPath: workspaceConfig}
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	if _, err := os.Stat(userConfig); err == nil {
		config.configPath = userConfig
		content, err := ioutil.ReadFile(userConfig)
		if err != nil {
			return nil, err
		}

		// Overwrite the workspace settings with the user settings.
		if err := json.Unmarshal(content, &config); err != nil {
			return nil, err
		}
	}

	if err := config.validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

func ReadConfig(configPath string) (*Config, error) {
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	config := Config{configPath: configPath}
	if err := json.Unmarshal(content, &config); err != nil {
		return nil, err
	}

	if err := config.validate(); err != nil {
		return nil, err
	}

	return &config, nil
}

func (config *Config) LogProvider() LogProvider {
	switch config.LogMode {
	case LogMode_Stdout:
		return StdoutLogProvider
	case LogMode_LogfilesAppend:
		return LogfilesAppendProvider
	case LogMode_LogfilesByDate:
		return LogfilesByDateProvider
	}

	panic(fmt.Errorf("unknown log mode: %s", config.LogMode))
}

func (config *Config) ConfigFilePath() string {
	path, err := filepath.Abs(config.configPath)
	if err != nil {
		panic(err)
	}

	return path
}

func (config *Config) ShellDirectory(dir string) shell.RunOption {
	return func(r *interp.Runner) {
		r.Dir = filepath.Join(config.projectPath(), dir)
	}
}

func (config *Config) projectPath() string {
	configPath, err := filepath.Abs(config.configPath)
	if err != nil {
		panic(err)
	}

	return filepath.Join(filepath.Dir(configPath), config.Path)
}

func (config *Config) validate() error {
	if config.Path == "" {
		return oops.Errorf("must specify path: %s", config.configPath)
	}

	if config.LogMode == "" {
		config.LogMode = "stdout"
	}

	if config.LogMode != LogMode_Stdout && config.LogMode != LogMode_LogfilesAppend && config.LogMode != LogMode_LogfilesByDate {
		return oops.Errorf("must specify valid logmode: %s", config.configPath)
	}

	return nil
}

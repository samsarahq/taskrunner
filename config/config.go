package config

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"

	"github.com/samsarahq/go/oops"
)

// configFile is the json representation for the .taskrunner.json
// file format.
type configFile struct {
	configPath string

	Path         *string  `json:"path"`
	DesiredTasks []string `json:"desiredTasks"`
}

// resolveWorkingDir resolves the pathOption into an absolute path. It assumes
// pathOption (the "path" field in the config file) is relative to filePath (the config file).
func resolveWorkingDir(filePath, pathOption string) (string, error) {
	configPath, err := filepath.Abs(filePath)
	if err != nil {
		return "", oops.Wrapf(err, "unable to find config file %s", filePath)
	}

	configPath, err = filepath.EvalSymlinks(configPath)
	if err != nil {
		return "", oops.Wrapf(err, "failed to evaluate symlinks for config file: %s", filePath)
	}
	return filepath.Join(filepath.Dir(configPath), pathOption), nil
}

// ToConfig converts a configFile representation to a usable Config.
func (c configFile) ToConfig() (*Config, error) {
	config := Config{
		DesiredTasks: c.DesiredTasks,
		ConfigPath:   c.configPath,
	}

	if c.Path == nil {
		return nil, oops.Errorf("config must specify path: %s", c.configPath)
	}

	resolvedPath, err := resolveWorkingDir(c.configPath, *c.Path)
	if err != nil {
		return nil, oops.Wrapf(err, "failed to resolve path for %s", *c.Path)
	}
	config.WorkingDir = resolvedPath

	return &config, nil
}

// Config defines user parameters for taskrunner. It is usually loaded
// via ReadUserConfig or ReadConfig.
type Config struct {
	// WorkingDir is the resolved working directory from the "path" attribute.
	WorkingDir string

	// CaonfigPath is the path to the file from where this config was loaded.
	ConfigPath string

	// DesiredTasks is the set of tasks to run specified by this configuration.
	DesiredTasks []string
}

// ReadConfig reads the configuration at configPath and returns a Config.
func ReadConfig(configPath string) (*Config, error) {
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	resolvedConfigPath, err := filepath.Abs(configPath)
	if err != nil {
		return nil, oops.Wrapf(err, "unable to find config file %s", configPath)
	}

	configFile := configFile{configPath: resolvedConfigPath}
	if err := json.Unmarshal(content, &configFile); err != nil {
		return nil, err
	}

	config, err := configFile.ToConfig()
	if err != nil {
		return nil, oops.Wrapf(err, "failed to build config from %s", resolvedConfigPath)
	}

	return config, nil
}

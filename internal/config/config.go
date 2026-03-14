package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// FoobarConfig represents the root configuration structure
type FoobarConfig struct {
	DockerCompose string                   `json:"docker_compose,omitempty"`
	Concurrently  *ConcurrentlyConfig      `json:"concurrently,omitempty"`
	Processes     map[string]ProcessConfig `json:"processes,omitempty"`
}

// ConcurrentlyConfig holds configuration for parsing a package.json script
type ConcurrentlyConfig struct {
	File   string `json:"file"`
	Script string `json:"script"`
}

// ProcessConfig holds configuration for an individual managed process
type ProcessConfig struct {
	Command string `json:"command"`
	Dir     string `json:"dir,omitempty"` // Optional working directory
}

// Load reads and parses a configuration file from the given path
func Load(path string) (*FoobarConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg FoobarConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	if cfg.Processes == nil {
		cfg.Processes = make(map[string]ProcessConfig)
	}

	// Validate config
	if cfg.DockerCompose == "" && cfg.Concurrently == nil && len(cfg.Processes) == 0 {
		return nil, fmt.Errorf("configuration is empty: you must specify at least one of 'docker_compose', 'concurrently', or 'processes'")
	}

	if cfg.Concurrently != nil {
		if cfg.Concurrently.File == "" {
			return nil, fmt.Errorf("concurrently configuration missing required field 'file' (e.g., 'package.json')")
		}
		if cfg.Concurrently.Script == "" {
			return nil, fmt.Errorf("concurrently configuration missing required field 'script' (e.g., 'dev')")
		}
	}

	for name, pcfg := range cfg.Processes {
		if pcfg.Command == "" {
			return nil, fmt.Errorf("process '%s' must specify a 'command' to run", name)
		}
	}

	return &cfg, nil
}

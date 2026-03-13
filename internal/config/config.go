package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// FoobarConfig represents the root configuration structure
type FoobarConfig struct {
	Processes map[string]ProcessConfig `json:"processes"`
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

	// Validate config
	for name, pcfg := range cfg.Processes {
		if pcfg.Command == "" {
			return nil, fmt.Errorf("process '%s' must specify a command", name)
		}
	}

	return &cfg, nil
}

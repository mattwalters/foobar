package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Version   string    `yaml:"version"`
	Settings  Settings  `yaml:"settings"`
	Processes []Process `yaml:"processes"`
}

type Settings struct {
	LogRetentionDays int `yaml:"log_retention_days"`
}

type Process struct {
	Name       string            `yaml:"name"`
	Command    string            `yaml:"command"`
	Cwd        string            `yaml:"cwd,omitempty"`
	Env        map[string]string `yaml:"env,omitempty"`
	Tags       []string          `yaml:"tags,omitempty"`
	Type       string            `yaml:"type,omitempty"`        // "docker-compose" or empty (default)
	ConfigFile string            `yaml:"config_file,omitempty"` // for docker-compose type
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

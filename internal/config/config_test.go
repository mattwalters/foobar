package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "foobar-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	cfgPath := filepath.Join(tmpdir, "foobar.config.json")
	validJSON := `{
		"processes": {
			"web": {
				"command": "npm run start"
			},
			"worker": {
				"command": "python worker.py",
				"dir": "./src"
			}
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(validJSON), 0644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(cfg.Processes) != 2 {
		t.Errorf("expected 2 processes, got %d", len(cfg.Processes))
	}

	if cfg.Processes["web"].Command != "npm run start" {
		t.Errorf("expected web command 'npm run start', got %s", cfg.Processes["web"].Command)
	}
	if cfg.Processes["worker"].Dir != "./src" {
		t.Errorf("expected worker dir './src', got %s", cfg.Processes["worker"].Dir)
	}
}

func TestLoadInvalid(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "foobar-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	cfgPath := filepath.Join(tmpdir, "foobar.config.json")
	invalidJSON := `{
		"processes": {
			"web": {
			}
		}
	}`

	if err := os.WriteFile(cfgPath, []byte(invalidJSON), 0644); err != nil {
		t.Fatal(err)
	}

	_, err = Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing command, got nil")
	}
}

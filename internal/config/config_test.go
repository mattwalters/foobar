package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	// Create a temporary directory for config files
	tmpDir, err := os.MkdirTemp("", "foobar-config-test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	tests := []struct {
		name    string
		content string
		want    *Config
		wantErr bool
	}{
		{
			name: "valid config",
			content: `
version: "1"
settings:
  log_retention_days: 7
processes:
  - name: "api"
    command: "go run main.go"
    cwd: "./api"
    env:
      PORT: "8080"
    tags: ["backend"]
`,
			want: &Config{
				Version: "1",
				Settings: Settings{
					LogRetentionDays: 7,
				},
				Processes: []Process{
					{
						Name:    "api",
						Command: "go run main.go",
						Cwd:     "./api",
						Env:     map[string]string{"PORT": "8080"},
						Tags:    []string{"backend"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid yaml",
			content: `
version: "1"
settings:
  log_retention_days: "seven" # invalid type
`,
			want:    nil,
			wantErr: true,
		},
		{
			name:    "missing file",
			content: "",
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configPath string
			if tt.name == "missing file" {
				configPath = filepath.Join(tmpDir, "nonexistent.yaml")
			} else {
				configPath = filepath.Join(tmpDir, tt.name+".yaml")
				if err := os.WriteFile(configPath, []byte(tt.content), 0644); err != nil {
					t.Fatalf("failed to write config file: %v", err)
				}
			}

			got, err := Load(configPath)
			if (err != nil) != tt.wantErr {
				t.Errorf("Load() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if got.Version != tt.want.Version {
					t.Errorf("Load().Version = %v, want %v", got.Version, tt.want.Version)
				}
				if got.Settings.LogRetentionDays != tt.want.Settings.LogRetentionDays {
					t.Errorf("Load().Settings.LogRetentionDays = %v, want %v", got.Settings.LogRetentionDays, tt.want.Settings.LogRetentionDays)
				}
				if len(got.Processes) != len(tt.want.Processes) {
					t.Errorf("Load().Processes length = %v, want %v", len(got.Processes), len(tt.want.Processes))
				} else {
					// Check first process
					pGot := got.Processes[0]
					pWant := tt.want.Processes[0]
					if pGot.Name != pWant.Name {
						t.Errorf("Process[0].Name = %v, want %v", pGot.Name, pWant.Name)
					}
					if pGot.Env["PORT"] != pWant.Env["PORT"] {
						t.Errorf("Process[0].Env[PORT] = %v, want %v", pGot.Env["PORT"], pWant.Env["PORT"])
					}
				}
			}
		})
	}
}

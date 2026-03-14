package config

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// SetupDockerCompose boots the compose stack and returns a map of foobar processes
// configured to tail the logs of each service.
func SetupDockerCompose(composeFile string) (map[string]ProcessConfig, error) {
	// 1. Boot the stack
	upCmd := exec.Command("docker", "compose", "-f", composeFile, "up", "-d")
	if err := upCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to run docker compose up -d: %w", err)
	}

	// 2. Discover services
	configCmd := exec.Command("docker", "compose", "-f", composeFile, "config", "--services")
	var out bytes.Buffer
	configCmd.Stdout = &out
	if err := configCmd.Run(); err != nil {
		return nil, fmt.Errorf("failed to get docker compose services: %w", err)
	}

	procs := make(map[string]ProcessConfig)
	services := strings.Split(strings.TrimSpace(out.String()), "\n")
	for _, svc := range services {
		svc = strings.TrimSpace(svc)
		if svc == "" {
			continue
		}
		// The command to tail logs for this specific service
		cmdStr := fmt.Sprintf("docker compose -f %s logs -f %s", composeFile, svc)
		
		procs[svc] = ProcessConfig{Command: cmdStr}
	}

	if len(procs) == 0 {
		return nil, fmt.Errorf("no services found in %s", composeFile)
	}

	return procs, nil
}

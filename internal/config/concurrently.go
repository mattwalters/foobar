package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"unicode"
)

type PackageJSON struct {
	Scripts map[string]string `json:"scripts"`
}

// ExtractConcurrentlyProcesses parses a package.json and extracts the underlying commands run by concurrently.
func ExtractConcurrentlyProcesses(pkgPath, scriptName string) (map[string]ProcessConfig, error) {
	data, err := os.ReadFile(pkgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read package.json: %w", err)
	}

	var pkg PackageJSON
	if err := json.Unmarshal(data, &pkg); err != nil {
		return nil, fmt.Errorf("failed to parse package.json: %w", err)
	}

	script, ok := pkg.Scripts[scriptName]
	if !ok {
		return nil, fmt.Errorf("script '%s' not found in package.json", scriptName)
	}

	if !strings.HasPrefix(strings.TrimSpace(script), "concurrently ") {
		return nil, fmt.Errorf("script '%s' does not invoke concurrently: %s", scriptName, script)
	}

	return parseConcurrentlyArgs(script)
}

func parseConcurrentlyArgs(cmdLine string) (map[string]ProcessConfig, error) {
	args := splitTokens(cmdLine)
	
	procs := make(map[string]ProcessConfig)
	var names []string
	
	// Skip the first arg "concurrently"
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "-n" || arg == "--names" {
			if i+1 < len(args) {
				names = strings.Split(args[i+1], ",")
				i++
			}
			continue
		}
		// Skip other known flags that take arguments
		if arg == "-c" || arg == "--prefix-colors" || arg == "-p" || arg == "--prefix" {
			i++
			continue
		}
		// Skip boolean flags
		if strings.HasPrefix(arg, "-") {
			continue
		}
		
		// It's a command
		name := fmt.Sprintf("cmd%d", len(procs))
		if len(procs) < len(names) && names[len(procs)] != "" {
			name = strings.TrimSpace(names[len(procs)])
		}
		procs[name] = ProcessConfig{Command: arg}
	}
	
	if len(procs) == 0 {
		return nil, fmt.Errorf("no commands found in concurrently invocation")
	}
	
	return procs, nil
}

func splitTokens(s string) []string {
	var tokens []string
	var current strings.Builder
	inQuote := false
	var quoteChar rune
	escapeNext := false
	
	for _, r := range s {
		if escapeNext {
			current.WriteRune(r)
			escapeNext = false
			continue
		}
		if r == '\\' {
			escapeNext = true
			continue
		}

		if inQuote {
			if r == quoteChar {
				inQuote = false
			} else {
				current.WriteRune(r)
			}
		} else {
			if r == '"' || r == '\'' {
				inQuote = true
				quoteChar = r
			} else if unicode.IsSpace(r) {
				if current.Len() > 0 {
					tokens = append(tokens, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(r)
			}
		}
	}
	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}
	return tokens
}

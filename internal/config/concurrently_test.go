package config

import (
	"reflect"
	"testing"
)

func TestParseConcurrentlyArgs(t *testing.T) {
	tests := []struct {
		name    string
		cmd     string
		want    map[string]ProcessConfig
		wantErr bool
	}{
		{
			name: "basic",
			cmd:  `concurrently "npm run start" "npm run build"`,
			want: map[string]ProcessConfig{
				"cmd0": {Command: "npm run start"},
				"cmd1": {Command: "npm run build"},
			},
		},
		{
			name: "with names",
			cmd:  `concurrently -n "api,web" -c "bgBlue,bgGreen" "npm run api" "npm run web"`,
			want: map[string]ProcessConfig{
				"api": {Command: "npm run api"},
				"web": {Command: "npm run web"},
			},
		},
		{
			name: "single quotes",
			cmd:  `concurrently 'npm run start' 'npm run build'`,
			want: map[string]ProcessConfig{
				"cmd0": {Command: "npm run start"},
				"cmd1": {Command: "npm run build"},
			},
		},
		{
			name: "escaped quotes",
			cmd:  `concurrently "npm run echo \"hello\""`,
			want: map[string]ProcessConfig{
				"cmd0": {Command: `npm run echo "hello"`},
			},
		},
		{
			name: "wildcards not supported cleanly yet but should parse",
			cmd:  `concurrently "npm:watch-*"`,
			want: map[string]ProcessConfig{
				"cmd0": {Command: "npm:watch-*"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseConcurrentlyArgs(tt.cmd)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseConcurrentlyArgs() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseConcurrentlyArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

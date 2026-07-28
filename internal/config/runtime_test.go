package config

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestRuntimeConfigRoundTripAndPrivatePermissions(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "nested", "config.json")
	want := []string{"moonshotai/kimi-k3", "z-ai/glm-5.2"}
	if err := SaveRuntimeConfig(file, want); err != nil {
		t.Fatalf("SaveRuntimeConfig() error = %v", err)
	}
	got, found, err := LoadRuntimeConfig(file)
	if err != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", err)
	}
	if !found || !reflect.DeepEqual(got, want) {
		t.Fatalf("LoadRuntimeConfig() = %q, %t; want %q, true", got, found, want)
	}
	info, err := os.Stat(file)
	if err != nil {
		t.Fatalf("stat runtime config: %v", err)
	}
	if gotMode := info.Mode().Perm(); gotMode != 0o600 {
		t.Fatalf("runtime config mode = %o, want 600", gotMode)
	}
	if matches, err := filepath.Glob(filepath.Join(filepath.Dir(file), ".config-*.tmp")); err != nil || len(matches) != 0 {
		t.Fatalf("runtime config temp files = %v, glob error = %v", matches, err)
	}
}

func TestRuntimeConfigMissingDisabledAndInvalid(t *testing.T) {
	t.Parallel()

	for _, file := range []string{"", filepath.Join(t.TempDir(), "missing.json")} {
		models, found, err := LoadRuntimeConfig(file)
		if err != nil || found || models != nil {
			t.Fatalf("LoadRuntimeConfig(%q) = %q, %t, %v; want nil, false, nil", file, models, found, err)
		}
	}

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "malformed", body: `{`, want: "decode runtime config"},
		{name: "unknown field", body: `{"version":1,"backup_models":[],"secret":"x"}`, want: "unknown field"},
		{name: "wrong version", body: `{"version":2,"backup_models":[]}`, want: "unsupported runtime config version"},
		{name: "local model", body: `{"version":1,"backup_models":["local/qwen"]}`, want: "must be a cloud model"},
		{name: "trailing value", body: `{"version":1,"backup_models":[]} {}`, want: "multiple JSON values"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			file := filepath.Join(t.TempDir(), "config.json")
			if err := os.WriteFile(file, []byte(tt.body), 0o600); err != nil {
				t.Fatalf("write fixture: %v", err)
			}
			_, _, err := LoadRuntimeConfig(file)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("LoadRuntimeConfig() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestParseBackupRouterRuntimeConfigPrecedence(t *testing.T) {
	t.Parallel()

	file := filepath.Join(t.TempDir(), "config.json")
	if err := SaveRuntimeConfig(file, []string{"saved/model", "saved/other"}); err != nil {
		t.Fatalf("SaveRuntimeConfig() error = %v", err)
	}
	baseEnv := map[string]string{
		envTRAPIKey:   "tr-key",
		envPreset:     "backuprouter",
		envConfigFile: file,
	}
	saved, err := Parse(nil, envLookup(baseEnv), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse(saved) error = %v", err)
	}
	if got, want := strings.Join(saved.BackupModels, ","), "saved/model,saved/other"; got != want {
		t.Fatalf("saved models = %q, want %q", got, want)
	}

	envOverride := map[string]string{}
	for key, value := range baseEnv {
		envOverride[key] = value
	}
	envOverride[envBackupModels] = "env/model"
	fromEnv, err := Parse(nil, envLookup(envOverride), &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Parse(env override) error = %v", err)
	}
	if got := strings.Join(fromEnv.BackupModels, ","); got != "env/model" {
		t.Fatalf("env override models = %q", got)
	}

	fromFlag, err := Parse(
		[]string{"-backup-model", "flag/model"},
		envLookup(envOverride),
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("Parse(flag override) error = %v", err)
	}
	if got := strings.Join(fromFlag.BackupModels, ","); got != "flag/model" {
		t.Fatalf("flag override models = %q", got)
	}
}

package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	runtimeConfigVersion  = 1
	maxRuntimeConfigBytes = 64 << 10
)

type persistedRuntimeConfig struct {
	Version      int      `json:"version"`
	BackupModels []string `json:"backup_models"`
}

// LoadRuntimeConfig reads UI-managed settings. A missing or disabled file is
// not an error and leaves startup defaults in control.
func LoadRuntimeConfig(file string) ([]string, bool, error) {
	if file == "" {
		return nil, false, nil
	}
	handle, err := os.Open(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("open runtime config: %w", err)
	}
	defer handle.Close()

	decoder := json.NewDecoder(io.LimitReader(handle, maxRuntimeConfigBytes+1))
	decoder.DisallowUnknownFields()
	var stored persistedRuntimeConfig
	if err := decoder.Decode(&stored); err != nil {
		return nil, false, fmt.Errorf("decode runtime config: %w", err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return nil, false, err
	}
	if stored.Version != runtimeConfigVersion {
		return nil, false, fmt.Errorf("unsupported runtime config version %d", stored.Version)
	}
	models, err := NormalizeBackupModels(stored.BackupModels)
	if err != nil {
		return nil, false, fmt.Errorf("runtime config backup models: %w", err)
	}
	return models, true, nil
}

// SaveRuntimeConfig atomically persists UI-managed settings with private file
// permissions. No provider or TrustedRouter credentials are written here.
func SaveRuntimeConfig(file string, models []string) error {
	if file == "" {
		return nil
	}
	normalized, err := NormalizeBackupModels(models)
	if err != nil {
		return err
	}
	dir := filepath.Dir(file)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create runtime config directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".config-*.tmp")
	if err != nil {
		return fmt.Errorf("create runtime config temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}
	if err := tmp.Chmod(0o600); err != nil {
		cleanup()
		return fmt.Errorf("protect runtime config temp file: %w", err)
	}
	if err := json.NewEncoder(tmp).Encode(persistedRuntimeConfig{
		Version:      runtimeConfigVersion,
		BackupModels: normalized,
	}); err != nil {
		cleanup()
		return fmt.Errorf("encode runtime config: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("sync runtime config: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close runtime config: %w", err)
	}
	if err := os.Rename(tmpName, file); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("replace runtime config: %w", err)
	}
	if err := os.Chmod(file, 0o600); err != nil {
		return fmt.Errorf("protect runtime config: %w", err)
	}
	return nil
}

func ensureJSONEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); errors.Is(err, io.EOF) {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode runtime config trailing data: %w", err)
	}
	return errors.New("runtime config contains multiple JSON values")
}

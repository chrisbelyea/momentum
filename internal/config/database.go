// Package config contains process configuration defaults for Momentum.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	databaseEnvVar = "DB_PATH"
	appDirectory   = "Momentum"
	databaseName   = "momentum.db"
)

// DatabasePath returns the SQLite database path used by the server.
//
// An explicit DB_PATH always wins and is returned unchanged. When DB_PATH is
// not set, the database is placed in the current user's OS-specific writable
// configuration directory (for example, ~/.config/Momentum on Linux or
// %AppData%\\Momentum on Windows). The application directory is created with
// user-only permissions before the path is returned.
func DatabasePath() (string, error) {
	if path := os.Getenv(databaseEnvVar); path != "" {
		if err := ensureExplicitDatabaseParent(path); err != nil {
			return "", err
		}
		return path, nil
	}

	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("determine user configuration directory: %w", err)
	}
	appDir := filepath.Join(configDir, appDirectory)
	if err := os.MkdirAll(appDir, 0700); err != nil {
		return "", fmt.Errorf("create application configuration directory %q: %w", appDir, err)
	}
	return filepath.Join(appDir, databaseName), nil
}

// ensureExplicitDatabaseParent makes a custom filesystem database path usable
// on first run. SQLite special targets and URI-style paths are intentionally
// left untouched because they are not ordinary filesystem paths.
func ensureExplicitDatabaseParent(path string) error {
	if path == ":memory:" || strings.HasPrefix(path, "file:") {
		return nil
	}
	parent := filepath.Dir(path)
	if parent == "." {
		return nil
	}
	if err := os.MkdirAll(parent, 0700); err != nil {
		return fmt.Errorf("create database parent directory %q: %w", parent, err)
	}
	return nil
}

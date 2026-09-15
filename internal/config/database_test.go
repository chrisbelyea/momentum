package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestDatabasePathPreservesExplicitDBPath(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "custom", "tasks.sqlite")
	t.Setenv(databaseEnvVar, explicit)

	path, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() returned error: %v", err)
	}
	if path != explicit {
		t.Fatalf("DatabasePath() = %q, want explicit path %q", path, explicit)
	}
	info, err := os.Stat(filepath.Dir(explicit))
	if err != nil {
		t.Fatalf("explicit database parent was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("explicit database parent %q is not a directory", filepath.Dir(explicit))
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0077 != 0 {
		t.Fatalf("explicit database parent permissions = %o, want user-only", info.Mode().Perm())
	}
}

func TestDatabasePathPreservesSQLiteSpecialTargets(t *testing.T) {
	for _, target := range []string{":memory:", "file:shared?mode=memory&cache=shared"} {
		t.Run(target, func(t *testing.T) {
			t.Setenv(databaseEnvVar, target)
			path, err := DatabasePath()
			if err != nil {
				t.Fatalf("DatabasePath() returned error: %v", err)
			}
			if path != target {
				t.Fatalf("DatabasePath() = %q, want %q", path, target)
			}
		})
	}
}

func TestDatabasePathUsesWritableOSConfigDirectory(t *testing.T) {
	t.Setenv(databaseEnvVar, "")
	configRoot := t.TempDir()

	// UserConfigDir consults these platform-specific variables. Keeping the
	// test within the current OS makes it validate the same behavior as the
	// release binary on Linux and Windows.
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", configRoot)
	} else {
		t.Setenv("XDG_CONFIG_HOME", configRoot)
		t.Setenv("HOME", filepath.Join(configRoot, "home"))
	}

	path, err := DatabasePath()
	if err != nil {
		t.Fatalf("DatabasePath() returned error: %v", err)
	}
	want := filepath.Join(configRoot, appDirectory, databaseName)
	if path != want {
		t.Fatalf("DatabasePath() = %q, want %q", path, want)
	}
	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("default database directory was not created: %v", err)
	}
	if !info.IsDir() {
		t.Fatalf("default database parent %q is not a directory", filepath.Dir(path))
	}
}

package paths

import (
	"os"
	"path/filepath"
	"testing"
)

func TestXDGPaths(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	cacheRoot := t.TempDir()
	stateRoot := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("XDG_STATE_HOME", stateRoot)

	if got, want := ConfigDir(), filepath.Join(configRoot, appName); got != want {
		t.Fatalf("ConfigDir() = %q, want %q", got, want)
	}
	if got, want := DataDir(), filepath.Join(dataRoot, appName); got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
	if got, want := SessionsDir(), filepath.Join(dataRoot, appName, "sessions"); got != want {
		t.Fatalf("SessionsDir() = %q, want %q", got, want)
	}
	if got, want := CacheDir(), filepath.Join(cacheRoot, appName); got != want {
		t.Fatalf("CacheDir() = %q, want %q", got, want)
	}
	if got, want := StateDir(), filepath.Join(stateRoot, appName); got != want {
		t.Fatalf("StateDir() = %q, want %q", got, want)
	}
}

func TestDerivedPaths(t *testing.T) {
	dataRoot := t.TempDir()
	configRoot := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("XDG_CONFIG_HOME", configRoot)

	if got, want := MemoryDir("/tmp/my project"), filepath.Join(dataRoot, appName, "projects", "tmp-my project", "memory"); got != want {
		t.Fatalf("MemoryDir() = %q, want %q", got, want)
	}
	if got, want := SkillsDir(), filepath.Join(configRoot, appName, "skills"); got != want {
		t.Fatalf("SkillsDir() = %q, want %q", got, want)
	}
	if got, want := SessionDir("session/one"), filepath.Join(dataRoot, appName, "sessions", "session-one"); got != want {
		t.Fatalf("SessionDir() = %q, want %q", got, want)
	}
	if got, want := SessionTasksDir("session/one"), filepath.Join(dataRoot, appName, "sessions", "session-one", "tasks"); got != want {
		t.Fatalf("SessionTasksDir() = %q, want %q", got, want)
	}
	if got, want := TaskOutputPath("session/one", "task-1"), filepath.Join(dataRoot, appName, "sessions", "session-one", "tasks", "task-1.jsonl"); got != want {
		t.Fatalf("TaskOutputPath() = %q, want %q", got, want)
	}
	if got := ProjectSkillsDir(); got != ".nandocodego/skills" {
		t.Fatalf("ProjectSkillsDir() = %q", got)
	}
}

func TestDataDirLocalShareFallback(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", "")
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.MkdirAll(filepath.Join(home, ".local", "share"), 0o700); err != nil {
		t.Fatal(err)
	}
	if got, want := DataDir(), filepath.Join(home, ".local", "share", appName); got != want {
		t.Fatalf("DataDir() = %q, want %q", got, want)
	}
}

func TestNandocodegoOverrides(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	cacheRoot := t.TempDir()
	stateRoot := t.TempDir()

	t.Setenv("NANDOCODEGO_CONFIG_HOME", configRoot)
	t.Setenv("NANDOCODEGO_DATA_HOME", dataRoot)
	t.Setenv("NANDOCODEGO_CACHE_HOME", cacheRoot)
	t.Setenv("NANDOCODEGO_STATE_HOME", stateRoot)
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if got := ConfigDir(); got != configRoot {
		t.Fatalf("ConfigDir() = %q, want %q", got, configRoot)
	}
	if got := DataDir(); got != dataRoot {
		t.Fatalf("DataDir() = %q, want %q", got, dataRoot)
	}
	if got := CacheDir(); got != cacheRoot {
		t.Fatalf("CacheDir() = %q, want %q", got, cacheRoot)
	}
	if got := StateDir(); got != stateRoot {
		t.Fatalf("StateDir() = %q, want %q", got, stateRoot)
	}
}

func TestSanitizePathForDir(t *testing.T) {
	tests := map[string]string{
		"":                    "root",
		"/":                   "root",
		"/tmp/my project":     "tmp-my project",
		`C:\Users\Fernando`:   "C--Users-Fernando",
		"session/one:two":     "session-one-two",
		"already-safe-string": "already-safe-string",
	}

	for input, want := range tests {
		if got := SanitizePathForDir(input); got != want {
			t.Fatalf("SanitizePathForDir(%q) = %q, want %q", input, got, want)
		}
	}
}

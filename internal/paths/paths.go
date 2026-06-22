// Package paths provides XDG-aware path resolution for config, data, and memory directories.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

const (
	appName = "nandocodego"
)

// ConfigDir returns the configuration directory path.
// Honors NANDOCODEGO_CONFIG_HOME, XDG_CONFIG_HOME, then falls back to ~/.nandocodego/.
func ConfigDir() string {
	if override := os.Getenv("NANDOCODEGO_CONFIG_HOME"); override != "" {
		return override
	}
	if xdgConfig := os.Getenv("XDG_CONFIG_HOME"); xdgConfig != "" {
		return filepath.Join(xdgConfig, appName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to relative path if home dir is unavailable
		return filepath.Join(".", "."+appName)
	}

	return filepath.Join(home, "."+appName)
}

// DataDir returns the data directory path.
// Honors NANDOCODEGO_DATA_HOME, XDG_DATA_HOME, then falls back to ~/.local/share/nandocodego/ or ~/.nandocodego/data/.
func DataDir() string {
	if override := os.Getenv("NANDOCODEGO_DATA_HOME"); override != "" {
		return override
	}
	if xdgData := os.Getenv("XDG_DATA_HOME"); xdgData != "" {
		return filepath.Join(xdgData, appName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		// Fallback to relative path if home dir is unavailable
		return filepath.Join(".", "."+appName, "data")
	}

	// Check if ~/.local/share exists (common on Linux)
	localShare := filepath.Join(home, ".local", "share")
	if stat, err := os.Stat(localShare); err == nil && stat.IsDir() {
		return filepath.Join(localShare, appName)
	}

	// Fallback to ~/.nandocodego/data/
	return filepath.Join(home, "."+appName, "data")
}

// CacheDir returns the cache directory path.
// Honors NANDOCODEGO_CACHE_HOME, XDG_CACHE_HOME, then falls back to ~/.cache/nandocodego/ or ~/.nandocodego/cache/.
func CacheDir() string {
	if override := os.Getenv("NANDOCODEGO_CACHE_HOME"); override != "" {
		return override
	}
	if xdgCache := os.Getenv("XDG_CACHE_HOME"); xdgCache != "" {
		return filepath.Join(xdgCache, appName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "."+appName, "cache")
	}

	return filepath.Join(home, ".cache", appName)
}

// StateDir returns the state directory path.
// Honors NANDOCODEGO_STATE_HOME, XDG_STATE_HOME, then falls back to ~/.local/state/nandocodego/ or ~/.nandocodego/state/.
func StateDir() string {
	if override := os.Getenv("NANDOCODEGO_STATE_HOME"); override != "" {
		return override
	}
	if xdgState := os.Getenv("XDG_STATE_HOME"); xdgState != "" {
		return filepath.Join(xdgState, appName)
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "."+appName, "state")
	}

	return filepath.Join(home, ".local", "state", appName)
}

// MemoryDir returns the memory directory path for a given git root.
// The gitRoot is sanitized to create a safe directory name.
func MemoryDir(gitRoot string) string {
	// Sanitize the git root path to create a safe directory name
	sanitized := SanitizePathForDir(gitRoot)
	return filepath.Join(DataDir(), "projects", sanitized, "memory")
}

// SessionsDir returns the sessions directory path.
func SessionsDir() string {
	return filepath.Join(DataDir(), "sessions")
}

// SessionDir returns the directory path for a specific session ID.
func SessionDir(sessionID string) string {
	return filepath.Join(SessionsDir(), SanitizePathForDir(sessionID))
}

// SessionTasksDir returns the task directory for a session.
func SessionTasksDir(sessionID string) string {
	return filepath.Join(SessionDir(sessionID), "tasks")
}

// TaskOutputPath returns the JSONL output file path for a task.
func TaskOutputPath(sessionID, taskID string) string {
	return filepath.Join(SessionTasksDir(sessionID), SanitizePathForDir(taskID)+".jsonl")
}

// SkillsDir returns the user-level skills directory path.
func SkillsDir() string {
	return filepath.Join(ConfigDir(), "skills")
}

// ProjectSkillsDir returns the project-level skills directory path.
func ProjectSkillsDir() string {
	return ".nandocodego/skills"
}

// SanitizePathForDir converts a file path or ID into a safe directory name.
// It replaces path separators and other problematic characters with hyphens.
func SanitizePathForDir(path string) string {
	// Clean the path first
	path = filepath.Clean(path)

	// Replace path separators with hyphens
	sanitized := strings.NewReplacer(
		string(os.PathSeparator), "-",
		"/", "-",
		"\\", "-",
		":", "-",
	).Replace(path)

	// Trim leading/trailing hyphens
	sanitized = strings.Trim(sanitized, "-")
	if sanitized == "" || sanitized == "." {
		return "root"
	}

	return sanitized
}

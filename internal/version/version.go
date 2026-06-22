// Package version provides build-time version information.
package version

import (
	"fmt"
	"runtime"
)

// Version information. These are set via ldflags during build.
var (
	// Version is the semantic version string (e.g., "0.1.0", "0.0.0-dev").
	Version = "0.0.0-dev"

	// Commit is the git commit hash at build time.
	Commit = "unknown"

	// CommitSHA is the git commit hash at build time.
	//
	// Deprecated: use Commit.
	CommitSHA = "unknown"

	// BuildTime is the RFC3339 timestamp of when the binary was built.
	BuildTime = "unknown"
)

// String returns the short human-readable version string.
func String() string {
	return fmt.Sprintf("nandocodego %s (%s)", Version, commit())
}

// Info returns a formatted version string with build metadata.
func Info() string {
	return String()
}

// FullInfo returns detailed version information including Go version and platform.
func FullInfo() string {
	return fmt.Sprintf(
		"nandocodego %s\nCommit: %s\nBuilt: %s\nGo: %s\nOS/Arch: %s/%s",
		Version,
		commit(),
		BuildTime,
		runtime.Version(),
		runtime.GOOS,
		runtime.GOARCH,
	)
}

func commit() string {
	if Commit != "" && Commit != "unknown" {
		return Commit
	}
	return CommitSHA
}

// Package cli provides the command-line interface for nandocodego.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/mcp"
	"github.com/FernasFragas/Nandocode/internal/observability"
	"github.com/FernasFragas/Nandocode/internal/paths"
	"github.com/FernasFragas/Nandocode/internal/version"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check system configuration and environment",
		Long: `The doctor command performs diagnostic checks on the nandocodego installation
and environment. It reports:

  • Version and build information
  • Go runtime version
  • Operating system and architecture
  • Configuration and data directory paths
  • Directory existence and permissions

Use this command to verify your installation or diagnose issues.`,
		RunE: runDoctor,
	}
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "nandocodego Doctor")
	fmt.Fprintln(out, "==================")
	fmt.Fprintln(out)

	// Version information
	fmt.Fprintln(out, "Version Information:")
	fmt.Fprintf(out, "  Version:    %s\n", version.Version)
	fmt.Fprintf(out, "  Commit:     %s\n", version.Commit)
	fmt.Fprintf(out, "  Build Time: %s\n", version.BuildTime)
	fmt.Fprintln(out)

	// Runtime information
	fmt.Fprintln(out, "Runtime Information:")
	fmt.Fprintf(out, "  Go Version: %s\n", runtime.Version())
	fmt.Fprintf(out, "  OS:         %s\n", runtime.GOOS)
	fmt.Fprintf(out, "  Arch:       %s\n", runtime.GOARCH)
	fmt.Fprintf(out, "  CPUs:       %d\n", runtime.NumCPU())
	fmt.Fprintln(out)

	// Directory paths
	configDir := paths.ConfigDir()
	dataDir := paths.DataDir()
	cacheDir := paths.CacheDir()
	stateDir := paths.StateDir()

	fmt.Fprintln(out, "Directory Paths:")
	fmt.Fprintf(out, "  Config Dir: %s\n", configDir)
	fmt.Fprintf(out, "  Data Dir:   %s\n", dataDir)
	fmt.Fprintf(out, "  Cache Dir:  %s\n", cacheDir)
	fmt.Fprintf(out, "  State Dir:  %s\n", stateDir)
	fmt.Fprintln(out)

	// Check directory status
	fmt.Fprintln(out, "Directory Status:")
	checkDirectory(out, "Config", configDir)
	checkDirectory(out, "Data", dataDir)
	checkDirectory(out, "Cache", cacheDir)
	checkDirectory(out, "State", stateDir)
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Ollama Status:")
	fmt.Fprintln(out, "  Ollama: not checked in phase 1")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Telemetry Status:")
	fmt.Fprintf(out, "  Telemetry: %s\n", observability.TelemetryFromEnv().DoctorStatus())
	fmt.Fprintln(out)

	fmt.Fprintln(out, "MCP Status:")
	checkMCPStatus(out, configDir)
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Security Baseline:")
	baselineErr := checkSecurityBaseline(out)
	fmt.Fprintln(out)

	// Environment variables
	fmt.Fprintln(out, "Environment Variables:")
	checkEnvVar(out, "XDG_CONFIG_HOME")
	checkEnvVar(out, "XDG_DATA_HOME")
	checkEnvVar(out, "XDG_CACHE_HOME")
	checkEnvVar(out, "XDG_STATE_HOME")
	checkEnvVar(out, "NANDOCODEGO_CONFIG_HOME")
	checkEnvVar(out, "NANDOCODEGO_DATA_HOME")
	checkEnvVar(out, "NANDOCODEGO_CACHE_HOME")
	checkEnvVar(out, "NANDOCODEGO_STATE_HOME")
	checkEnvVar(out, "NANDOCODEGO_DEBUG")
	checkEnvVar(out, "NANDOCODEGO_TELEMETRY")
	checkEnvVar(out, "NANDOCODEGO_OTEL_ENDPOINT")
	fmt.Fprintln(out)

	if baselineErr != nil {
		fmt.Fprintln(out, "Doctor check failed")
		return baselineErr
	}

	fmt.Fprintln(out, "Doctor check complete")
	return nil
}

func checkMCPStatus(out io.Writer, configDir string) {
	wd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(out, "  MCP: error checking cwd: %v\n", err)
		return
	}
	userCfg := filepath.Join(configDir, "config.toml")
	projectCfg := filepath.Join(wd, ".nandocodego", "config.toml")
	cfg, warnings := mcp.LoadConfig(userCfg, projectCfg)
	fmt.Fprintf(out, "  Servers: %d\n", len(cfg.Servers))
	if len(cfg.Servers) == 0 {
		fmt.Fprintln(out, "  Warnings: none")
		return
	}
	fmt.Fprintln(out, "  Server Details:")
	for _, server := range cfg.Servers {
		fmt.Fprintf(out, "    - %s (%s): enabled=%t trusted=%t\n", server.Name, server.Transport, server.Enabled, server.Trusted)
	}
	if len(warnings) == 0 {
		fmt.Fprintln(out, "  Warnings: none")
	} else {
		fmt.Fprintf(out, "  Warnings: %d\n", len(warnings))
	}
	for _, w := range warnings {
		fmt.Fprintf(out, "    - %s\n", w)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	mgr, startWarnings := mcp.Start(ctx, cfg)
	defer mgr.Close()
	if len(startWarnings) > 0 {
		fmt.Fprintf(out, "  Connection Warnings: %d\n", len(startWarnings))
		for _, w := range startWarnings {
			fmt.Fprintf(out, "    - %s\n", w)
		}
	}
	fmt.Fprintln(out, "  Connectivity:")
	for _, status := range mgr.ServerStatuses() {
		switch {
		case !status.Enabled:
			fmt.Fprintf(out, "    - %s: disabled\n", status.Name)
		case !status.Trusted:
			fmt.Fprintf(out, "    - %s: skipped (untrusted)\n", status.Name)
		case status.Connected:
			fmt.Fprintf(out, "    - %s: connected (%d tools)\n", status.Name, status.ToolCount)
		default:
			errMsg := strings.TrimSpace(status.Error)
			if errMsg == "" {
				errMsg = "connection failed"
			}
			fmt.Fprintf(out, "    - %s: failed (%s)\n", status.Name, errMsg)
		}
	}
}

func checkDirectory(out io.Writer, name, path string) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintf(out, "  %s: missing (will be created on first use)\n", name)
		} else {
			fmt.Fprintf(out, "  %s: error checking: %v\n", name, err)
		}
		return
	}

	if !stat.IsDir() {
		fmt.Fprintf(out, "  %s: exists but is not a directory\n", name)
		return
	}

	// Check if writable
	testFile := fmt.Sprintf("%s/.nandocodego-test-%d", path, os.Getpid())
	if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
		fmt.Fprintf(out, "  %s: exists but not writable: %v\n", name, err)
	} else {
		os.Remove(testFile)
		fmt.Fprintf(out, "  %s: exists and writable\n", name)
	}
}

func checkEnvVar(out io.Writer, name string) {
	value := os.Getenv(name)
	if value == "" {
		fmt.Fprintf(out, "  %s: (not set)\n", name)
	} else {
		fmt.Fprintf(out, "  %s: %s\n", name, value)
	}
}

func checkSecurityBaseline(out io.Writer) error {
	root, err := findRepoRoot()
	if err != nil {
		fmt.Fprintf(out, "  repository root: %v\n", err)
		return fmt.Errorf("security baseline incomplete: %w", err)
	}

	requiredFiles := []string{
		"SECURITY.md",
		filepath.Join("tools", "allowed-deps.txt"),
		filepath.Join("tools", "check-allowed-deps.sh"),
		filepath.Join("tools", "check-network-policy.sh"),
	}

	var missing []string
	for _, file := range requiredFiles {
		fullPath := filepath.Join(root, file)
		if _, err := os.Stat(fullPath); err != nil {
			if os.IsNotExist(err) {
				fmt.Fprintf(out, "  %s: missing\n", file)
				missing = append(missing, file)
				continue
			}
			fmt.Fprintf(out, "  %s: error checking: %v\n", file, err)
			missing = append(missing, file)
			continue
		}
		fmt.Fprintf(out, "  %s: present\n", file)
	}

	if len(missing) > 0 {
		return fmt.Errorf("security baseline incomplete: %w", errors.New("required files missing"))
	}
	return nil
}

func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found from current directory")
		}
		dir = parent
	}
}

package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDoctorCommandOutput(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	cacheRoot := t.TempDir()
	stateRoot := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("XDG_STATE_HOME", stateRoot)

	for _, dir := range []string{
		filepath.Join(configRoot, "nandocodego"),
		filepath.Join(dataRoot, "nandocodego"),
		filepath.Join(cacheRoot, "nandocodego"),
		filepath.Join(stateRoot, "nandocodego"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"nandocodego Doctor",
		"Runtime Information:",
		"Directory Paths:",
		"Cache Dir:",
		"State Dir:",
		"Ollama: not checked in phase 1",
		"Telemetry Status:",
		"Telemetry: disabled",
		"MCP Status:",
		"Servers:",
		"Security Baseline:",
		"SECURITY.md: present",
		"Doctor check complete",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("doctor output missing %q:\n%s", want, out.String())
		}
	}
}

func TestDoctorFailsWhenSecurityBaselineMissing(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(cwd); err != nil {
			t.Fatal(err)
		}
	})

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected doctor to fail with missing security baseline")
	}
	if !strings.Contains(out.String(), "SECURITY.md: missing") {
		t.Fatalf("doctor output = %q", out.String())
	}
}

func TestDoctorReportsMCPServerDetailsAndConnectivity(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	cacheRoot := t.TempDir()
	stateRoot := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("XDG_STATE_HOME", stateRoot)

	configDir := filepath.Join(configRoot, "nandocodego")
	for _, dir := range []string{
		configDir,
		filepath.Join(dataRoot, "nandocodego"),
		filepath.Join(cacheRoot, "nandocodego"),
		filepath.Join(stateRoot, "nandocodego"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	cfg := `
[mcp.servers.disabled_local]
enabled = false
transport = "stdio"
command = "noop"

[mcp.servers.user_untrusted]
enabled = true
trusted = false
transport = "stdio"
command = "noop"
`
	if err := os.WriteFile(filepath.Join(configDir, "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatal(err)
	}

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"MCP Status:",
		"Server Details:",
		"disabled_local (stdio): enabled=false trusted=true",
		"user_untrusted (stdio): enabled=true trusted=false",
		"Connectivity:",
		"disabled_local: disabled",
		"user_untrusted: skipped (untrusted)",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}

func TestDoctorTelemetryStatusFromEnv(t *testing.T) {
	configRoot := t.TempDir()
	dataRoot := t.TempDir()
	cacheRoot := t.TempDir()
	stateRoot := t.TempDir()

	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("XDG_DATA_HOME", dataRoot)
	t.Setenv("XDG_CACHE_HOME", cacheRoot)
	t.Setenv("XDG_STATE_HOME", stateRoot)
	t.Setenv("NANDOCODEGO_TELEMETRY", "1")
	t.Setenv("NANDOCODEGO_OTEL_ENDPOINT", "otel-endpoint")

	for _, dir := range []string{
		filepath.Join(configRoot, "nandocodego"),
		filepath.Join(dataRoot, "nandocodego"),
		filepath.Join(cacheRoot, "nandocodego"),
		filepath.Join(stateRoot, "nandocodego"),
	} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	cmd := NewRootCmd()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"doctor"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Telemetry: otel -> otel-endpoint") {
		t.Fatalf("doctor output missing telemetry endpoint: %s", out.String())
	}
}

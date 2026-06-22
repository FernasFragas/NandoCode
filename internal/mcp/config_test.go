package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTOMLServers(t *testing.T) {
	t.Parallel()
	raw := `
[mcp.servers.filesystem]
enabled = true
transport = "stdio"
command = "npx"
args = ["-y", "@modelcontextprotocol/server-filesystem", "."]
env = { MCP_MODE = "test", DEBUG = "0" }

[mcp.servers.disabled]
enabled = false
command = "nope"
`
	cfg, warnings := parseTOML(raw, "test", true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(cfg.Servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Name != "disabled" || cfg.Servers[1].Name != "filesystem" {
		t.Fatalf("unexpected server ordering: %#v", cfg.Servers)
	}
	if cfg.Servers[0].Enabled {
		t.Fatalf("expected disabled server to stay in parsed config")
	}
	if cfg.Servers[1].Name != "filesystem" {
		t.Fatalf("unexpected server name: %q", cfg.Servers[1].Name)
	}
	if cfg.Servers[1].Command != "npx" {
		t.Fatalf("unexpected command: %q", cfg.Servers[1].Command)
	}
	if cfg.Servers[1].Env["MCP_MODE"] != "test" || cfg.Servers[1].Env["DEBUG"] != "0" {
		t.Fatalf("unexpected env map: %#v", cfg.Servers[1].Env)
	}
}

func TestLoadConfigPrecedence(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	userPath := filepath.Join(td, "user.toml")
	projectPath := filepath.Join(td, "project.toml")

	project := `
[mcp.servers.fs]
enabled = true
command = "project-cmd"
`
	user := `
[mcp.servers.fs]
enabled = true
command = "user-cmd"
`
	if err := os.WriteFile(projectPath, []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(userPath, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, warnings := LoadConfig(userPath, projectPath)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Command != "user-cmd" {
		t.Fatalf("expected user override, got %q", cfg.Servers[0].Command)
	}
}

func TestParseTOMLHTTPServer(t *testing.T) {
	t.Parallel()
	raw := `
[mcp.servers.remote]
transport = "http"
url = "https://127.0.0.1:11434/mcp"
auth = "oauth"
`
	cfg, warnings := parseTOML(raw, "test", true)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Transport != TransportHTTP {
		t.Fatalf("expected http transport")
	}
	if cfg.Servers[0].URL != "https://127.0.0.1:11434/mcp" {
		t.Fatalf("unexpected url: %q", cfg.Servers[0].URL)
	}
	if cfg.Servers[0].Auth != "oauth" {
		t.Fatalf("unexpected auth: %q", cfg.Servers[0].Auth)
	}
}

func TestParseTOMLHTTPMissingURLWarning(t *testing.T) {
	t.Parallel()
	raw := `
[mcp.servers.remote]
transport = "http"
`
	cfg, warnings := parseTOML(raw, "test", true)
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for missing url")
	}
}

func TestLoadConfigProjectDefaultsToUntrusted(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	projectPath := filepath.Join(td, "project.toml")
	project := `
[mcp.servers.project_one]
enabled = true
command = "project-cmd"
`
	if err := os.WriteFile(projectPath, []byte(project), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warnings := LoadConfig("", projectPath)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Trusted {
		t.Fatalf("expected project server to default to untrusted")
	}
}

func TestLoadConfigUserDefaultsToTrusted(t *testing.T) {
	t.Parallel()
	td := t.TempDir()
	userPath := filepath.Join(td, "user.toml")
	user := `
[mcp.servers.user_one]
enabled = true
command = "user-cmd"
`
	if err := os.WriteFile(userPath, []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, warnings := LoadConfig(userPath, "")
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if !cfg.Servers[0].Trusted {
		t.Fatalf("expected user server to default to trusted")
	}
}

func TestParseTOMLWarnsOnInvalidTransport(t *testing.T) {
	t.Parallel()
	raw := `
[mcp.servers.bad]
transport = "websocket"
command = "cmd"
`
	cfg, warnings := parseTOML(raw, "test", true)
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(cfg.Servers))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for invalid transport")
	}
}

func TestParseTOMLWarnsOnDuplicateServerSection(t *testing.T) {
	t.Parallel()
	raw := `
[mcp.servers.fs]
command = "first"

[mcp.servers.fs]
command = "second"
`
	cfg, warnings := parseTOML(raw, "test", true)
	if len(cfg.Servers) != 1 {
		t.Fatalf("expected 1 merged server entry, got %d", len(cfg.Servers))
	}
	if cfg.Servers[0].Command != "second" {
		t.Fatalf("expected last assignment to win, got %q", cfg.Servers[0].Command)
	}
	if len(warnings) == 0 {
		t.Fatalf("expected duplicate section warning")
	}
}

package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/FernasFragas/Nandocode/internal/tools"
)

type Manager struct {
	clients map[string]*Client
	tools   []tools.Tool
	status  []ServerStatus
}

type ServerStatus struct {
	Name      string
	Transport Transport
	Enabled   bool
	Trusted   bool
	Connected bool
	ToolCount int
	Error     string
}

func Start(ctx context.Context, cfg Config) (*Manager, []string) {
	m := &Manager{clients: map[string]*Client{}}
	var warnings []string
	for _, server := range cfg.Servers {
		status := ServerStatus{
			Name:      server.Name,
			Transport: server.Transport,
			Enabled:   server.Enabled,
			Trusted:   server.Trusted,
		}
		if !server.Enabled {
			status.Error = "disabled"
			m.status = append(m.status, status)
			continue
		}
		if !server.Trusted {
			msg := fmt.Sprintf("mcp server %q skipped: untrusted project config entry", server.Name)
			warnings = append(warnings, msg)
			status.Error = "untrusted config entry"
			m.status = append(m.status, status)
			continue
		}

		client, err := NewClient(server)
		if err != nil {
			msg := fmt.Sprintf("mcp server %q failed: %v", server.Name, err)
			warnings = append(warnings, msg)
			status.Error = err.Error()
			m.status = append(m.status, status)
			continue
		}
		if err := client.Connect(ctx); err != nil {
			msg := fmt.Sprintf("mcp server %q connect failed: %v", server.Name, err)
			warnings = append(warnings, msg)
			_ = client.Close()
			status.Error = err.Error()
			m.status = append(m.status, status)
			continue
		}
		m.clients[server.Name] = client
		descriptors := client.Tools()
		for _, td := range descriptors {
			if strings.TrimSpace(td.Name) == "" {
				continue
			}
			m.tools = append(m.tools, NewMCPTool(server.Name, td, client))
		}
		status.Connected = true
		status.ToolCount = len(descriptors)
		m.status = append(m.status, status)
	}
	return m, warnings
}

func (m *Manager) RegisterInto(reg *tools.Registry) []string {
	var warnings []string
	if m == nil || reg == nil {
		return warnings
	}
	for _, t := range m.tools {
		if err := reg.Register(t); err != nil {
			warnings = append(warnings, fmt.Sprintf("register %q failed: %v", t.Name(), err))
		}
	}
	return warnings
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	for _, c := range m.clients {
		_ = c.Close()
	}
}

func (m *Manager) ServerStatuses() []ServerStatus {
	if m == nil {
		return nil
	}
	out := make([]ServerStatus, len(m.status))
	copy(out, m.status)
	return out
}

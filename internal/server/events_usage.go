package server

import "github.com/FernasFragas/Nandocode/internal/agent"

func (s *Session) emitUsageUpdate(usage agent.Usage) {
	s.Emit("usage_update", map[string]any{"usage": usage})
}

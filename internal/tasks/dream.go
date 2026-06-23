package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/types"
)

type DreamTaskState struct {
	RunningTask
	Result      string
	CompletedAt time.Time
}

func (s DreamTaskState) isTaskState() {}

func (s DreamTaskState) ToSummary() types.TaskSummary {
	out := s.RunningTask.ToSummary()
	if !s.CompletedAt.IsZero() {
		out.Status = types.StatusCompleted
		out.FinishedAt = s.CompletedAt
	}
	return out
}

func (s *Supervisor) SpawnDream(ctx context.Context, client llm.Client, model string, systemPrompt string) (string, error) {
	if client == nil {
		return "", fmt.Errorf("llm client is required")
	}
	if s.ActiveWorkerCount() > 0 {
		return "", nil
	}
	s.mu.RLock()
	if s.dreamTaskID != "" {
		if st, ok := s.tasks[s.dreamTaskID]; ok {
			status := st.ToSummary().Status
			if status == types.StatusPending || status == types.StatusRunning {
				s.mu.RUnlock()
				return "", nil
			}
		}
	}
	s.mu.RUnlock()

	id, err := s.Start(ctx, types.KindDream, "speculative dream", func(runCtx context.Context, out *OutputWriter) (int, error) {
		req := &llm.ChatRequest{
			Model:    strings.TrimSpace(model),
			Messages: []llm.Message{{Role: llm.RoleSystem, Content: systemPrompt}, {Role: llm.RoleUser, Content: "Think about what the user may ask next and prepare a brief analysis."}},
			Stream:   true,
		}
		events, err := client.Chat(runCtx, req)
		if err != nil {
			return 1, err
		}
		var content strings.Builder
		for evt := range events {
			if evt.Message.Content != "" {
				content.WriteString(evt.Message.Content)
			}
		}
		s.mu.Lock()
		s.dreamResult = strings.TrimSpace(content.String())
		s.dreamCompletedAt = time.Now().UTC()
		s.mu.Unlock()
		return 0, nil
	})
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	s.dreamTaskID = id
	s.mu.Unlock()
	return id, nil
}

func (s *Supervisor) ConsumeDreamResult(maxAge time.Duration) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(s.dreamResult) == "" {
		return ""
	}
	if maxAge > 0 && !s.dreamCompletedAt.IsZero() && time.Since(s.dreamCompletedAt) > maxAge {
		s.dreamResult = ""
		s.dreamCompletedAt = time.Time{}
		return ""
	}
	out := s.dreamResult
	s.dreamResult = ""
	s.dreamCompletedAt = time.Time{}
	return out
}

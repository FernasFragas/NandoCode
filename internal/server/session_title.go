package server

import (
	"context"
	"strings"
	"time"
	"unicode"

	"github.com/FernasFragas/Nandocode/internal/credentials"
	"github.com/FernasFragas/Nandocode/internal/llm"
)

const titleMaxRunes = 60

func shortTitle(s string) string {
	s = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && unicode.IsSpace(r) {
			return ' '
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return -1
		}
		return r
	}, s)
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > titleMaxRunes {
		s = string(r[:titleMaxRunes])
	}
	return s
}

// generateSessionTitle is called after a terminal, once the run has finished.
func (s *Session) generateSessionTitle() {
	s.mu.Lock()
	if !s.titlePending || s.titleDone || s.titleStarted || s.running || s.state == SessionStateClosing {
		s.mu.Unlock()
		return
	}
	titleCtx, cancelTitle := context.WithCancel(s.ctx)
	s.titleCancel = cancelTitle
	s.titleFinished = make(chan struct{})
	s.titleStarted = true
	prompt := s.titlePrompt
	model := s.appState.ActiveModel
	provider := s.appState.LLMProvider
	client := s.llmClient
	models := s.models
	s.mu.Unlock()
	defer func() {
		cancelTitle()
		s.mu.Lock()
		s.titleCancel = nil
		s.titleStarted = false
		close(s.titleFinished)
		s.mu.Unlock()
	}()

	title, source := shortTitle(prompt), "fallback"
	allowed := client != nil
	if provider == string(llm.ProviderOllamaCloudAPI) {
		allowed = false
		if models != nil && models.Creds != nil {
			ctx, cancel := context.WithTimeout(titleCtx, 2*time.Second)
			key, _, err := models.Creds.Resolve(ctx, credentials.ResolveOptions{})
			cancel()
			allowed = err == nil && key != "" && client != nil
		}
	}
	if allowed {
		ctx, cancel := context.WithTimeout(titleCtx, 5*time.Second)
		stream, err := client.Chat(ctx, &llm.ChatRequest{
			Model: model, Stream: true, Think: false,
			Messages: []llm.Message{
				{Role: llm.RoleSystem, Content: "Write a short plain-text title for this coding task. Maximum six words. Return only the title."},
				{Role: llm.RoleUser, Content: prompt},
			},
			Options: map[string]any{"temperature": 0},
		})
		if err == nil && stream != nil {
			var b strings.Builder
			complete := false
		loop:
			for {
				select {
				case <-ctx.Done():
					break loop
				case evt, ok := <-stream:
					if !ok {
						break loop
					}
					b.WriteString(evt.Message.Content)
					if evt.Done {
						complete = true
					}
				}
			}
			if complete && ctx.Err() == nil {
				if candidate := shortTitle(b.String()); candidate != "" {
					title, source = candidate, "model"
				}
			}
		}
		cancel()
	}
	s.mu.Lock()
	if titleCtx.Err() != nil || !s.titlePending || s.titleDone || s.running || s.state == SessionStateClosing {
		s.mu.Unlock()
		return
	}
	s.title = title
	s.titleDone = true
	s.titlePending = false
	s.mu.Unlock()
	s.Emit("session_titled", map[string]any{"title": title, "source": source})
}

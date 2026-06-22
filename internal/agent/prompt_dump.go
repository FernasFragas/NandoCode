package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/FernasFragas/nandocodego/internal/bootstrap"
	"github.com/FernasFragas/nandocodego/internal/ids"
	"github.com/FernasFragas/nandocodego/internal/llm"
)

type PromptDump struct {
	CreatedAt        time.Time         `json:"created_at"`
	Model            string            `json:"model"`
	DumpMode         string            `json:"dump_mode"`
	Intent           string            `json:"intent,omitempty"`
	AttachmentPolicy string            `json:"attachment_policy,omitempty"`
	HistoryPolicy    string            `json:"history_policy,omitempty"`
	MemoryPolicy     string            `json:"memory_policy,omitempty"`
	RetryPolicy      string            `json:"retry_policy,omitempty"`
	IncludedFileBodies int             `json:"included_file_bodies,omitempty"`
	DirectoryTreeAttached bool         `json:"directory_tree_attached,omitempty"`
	Options          map[string]any    `json:"options"`
	MessageCount     int               `json:"message_count"`
	ToolCount        int               `json:"tool_count"`
	ToolNames        []string          `json:"tool_names"`
	Messages         []PromptMessage   `json:"messages"`
	EstimatedTokens  int               `json:"estimated_tokens"`
	PromptPackReport *PromptPackReport `json:"prompt_pack_report,omitempty"`
	EvidencePack     *EvidencePackReport `json:"evidence_pack,omitempty"`
}

type PromptMessage struct {
	Index           int    `json:"index"`
	Role            string `json:"role"`
	Bytes           int    `json:"bytes"`
	EstimatedTokens int    `json:"estimated_tokens"`
	Content         string `json:"content,omitempty"`
	ContentPreview  string `json:"content_preview,omitempty"`
}

var (
	promptDumpMu     sync.RWMutex
	latestPromptDump PromptDump
)

func LatestPromptDump() (PromptDump, bool) {
	promptDumpMu.RLock()
	defer promptDumpMu.RUnlock()
	if latestPromptDump.CreatedAt.IsZero() {
		return PromptDump{}, false
	}
	cp := latestPromptDump
	return cp, true
}

func SaveLatestPromptDump() (string, error) {
	promptDumpMu.RLock()
	dump := latestPromptDump
	promptDumpMu.RUnlock()
	if dump.CreatedAt.IsZero() {
		return "", fmt.Errorf("no prompt dump available")
	}
	path := promptDumpPath()
	if path == "" {
		return "", fmt.Errorf("state directory unavailable")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	b, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func recordPromptDump(req *llm.ChatRequest, pack *PromptPackReport, promptIntent, attachmentPolicy, historyPolicy string, evidencePack *EvidencePackReport, mode string, keep, previewChars int) {
	if req == nil {
		return
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		mode = "off"
	}
	if envMode := strings.ToLower(strings.TrimSpace(os.Getenv("NANDOCODEGO_PROMPT_DUMP"))); envMode != "" {
		mode = envMode
	}
	if previewChars <= 0 {
		previewChars = 600
	}
	if keep <= 0 {
		keep = 10
	}

	dump := PromptDump{
		CreatedAt:       time.Now().UTC(),
		Model:           req.Model,
		DumpMode:        mode,
		Intent:          promptIntent,
		AttachmentPolicy: attachmentPolicy,
		HistoryPolicy:   historyPolicy,
		Options:         copyOptions(req.Options),
		MessageCount:    len(req.Messages),
		ToolCount:       len(req.Tools),
		ToolNames:       toolNames(req.Tools),
		EstimatedTokens: estimatePromptTokens(req.Messages),
	}
	if pack != nil {
		cp := *pack
		dump.PromptPackReport = &cp
		dump.MemoryPolicy = cp.MemoryPolicy
		dump.RetryPolicy = cp.RetryPolicy
		dump.IncludedFileBodies = cp.IncludedFileBodies
		dump.DirectoryTreeAttached = cp.DirectoryTreeAttached
		if dump.HistoryPolicy == "" {
			dump.HistoryPolicy = cp.HistoryPolicy
		}
		if dump.Intent == "" {
			dump.Intent = cp.Intent
		}
		if dump.AttachmentPolicy == "" {
			dump.AttachmentPolicy = cp.AttachmentPolicy
		}
	}
	if evidencePack != nil {
		cp := *evidencePack
		dump.EvidencePack = &cp
	}
	for i, msg := range req.Messages {
		pm := PromptMessage{
			Index:           i,
			Role:            string(msg.Role),
			Bytes:           len(msg.Content),
			EstimatedTokens: estimatePromptTokens([]llm.Message{msg}),
		}
		switch mode {
		case "full":
			pm.Content = msg.Content
		case "metadata":
			pm.ContentPreview = truncatePreview(msg.Content, previewChars)
		}
		dump.Messages = append(dump.Messages, pm)
	}

	promptDumpMu.Lock()
	latestPromptDump = dump
	promptDumpMu.Unlock()

	if mode == "off" {
		return
	}
	_ = persistPromptDump(dump, mode, keep)
}

func persistPromptDump(dump PromptDump, mode string, keep int) error {
	base := promptDumpDir()
	if base == "" {
		return nil
	}
	if err := os.MkdirAll(base, 0o755); err != nil {
		return err
	}
	stamp := dump.CreatedAt.Format("20060102-150405")
	name := fmt.Sprintf("%s-%s.json", stamp, ids.New("pr"))
	path := filepath.Join(base, name)
	b, err := json.MarshalIndent(dump, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return err
	}
	latestPath := filepath.Join(base, "latest.json")
	if err := os.WriteFile(latestPath, b, 0o644); err != nil {
		return err
	}
	if keep > 0 {
		trimPromptDumps(base, keep)
	}
	return nil
}

func trimPromptDumps(base string, keep int) {
	entries, err := os.ReadDir(base)
	if err != nil {
		return
	}
	files := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || e.Name() == "latest.json" || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		files = append(files, e.Name())
	}
	if len(files) <= keep {
		return
	}
	sortStrings(files)
	for i := 0; i < len(files)-keep; i++ {
		_ = os.Remove(filepath.Join(base, files[i]))
	}
}

func promptDumpDir() string {
	snap := bootstrap.Global().Snapshot()
	if strings.TrimSpace(snap.StateDir) == "" {
		return ""
	}
	return filepath.Join(snap.StateDir, "prompt-dumps")
}

func promptDumpPath() string { return filepath.Join(promptDumpDir(), "latest.json") }

func truncatePreview(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

func copyOptions(in map[string]any) map[string]any {
	if len(in) == 0 {
		return map[string]any{}
	}
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func toolNames(defs []llm.ToolDef) []string {
	if len(defs) == 0 {
		return nil
	}
	names := make([]string, 0, len(defs))
	for _, d := range defs {
		names = append(names, d.Function.Name)
	}
	return names
}

func sortStrings(in []string) {
	for i := 0; i < len(in); i++ {
		for j := i + 1; j < len(in); j++ {
			if in[j] < in[i] {
				in[i], in[j] = in[j], in[i]
			}
		}
	}
}

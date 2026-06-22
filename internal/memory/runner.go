package memory

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/llm"
)

// Runner decorates agent runner with memory recall and extraction.
type Runner struct {
	next interface {
		Run(context.Context, agent.Input) <-chan agent.Event
	}
	client llm.Client
	cfg    Config
	nowFn  func() time.Time
}

func NewRunner(next interface {
	Run(context.Context, agent.Input) <-chan agent.Event
}, client llm.Client, cfg Config) *Runner {
	if cfg.Model == "" {
		cfg.Model = llm.DefaultModel
	}
	return &Runner{
		next:   next,
		client: client,
		cfg:    cfg,
		nowFn:  time.Now,
	}
}

func (r *Runner) Run(ctx context.Context, in agent.Input) <-chan agent.Event {
	out := make(chan agent.Event, 16)
	go func() {
		defer close(out)
		if r.next == nil {
			return
		}
		if !r.cfg.Enabled {
			for evt := range r.next.Run(ctx, in) {
				out <- evt
			}
			return
		}

		scopeRoot, err := ScopeRoot(in.ToolContext.WorkingDir, "")
		if err == nil {
			memDir := DirForScope(scopeRoot)
			stageStart := time.Now()
			in.SystemPrompt = r.buildAugmentedPrompt(ctx, in, memDir)
			select {
			case out <- agent.StageTiming{Stage: "memory_recall", Duration: time.Since(stageStart)}:
			case <-ctx.Done():
				return
			}
		}

		events := r.next.Run(ctx, in)
		for evt := range events {
			out <- evt
			term, ok := evt.(agent.Terminal)
			if !ok {
				continue
			}
			if term.Reason != agent.TerminalCompleted || len(term.Conversation) == 0 {
				continue
			}
			if r.cfg.NoExtract {
				continue
			}
			scopeRoot, err := ScopeRoot(in.ToolContext.WorkingDir, "")
			if err != nil {
				continue
			}
			memDir := DirForScope(scopeRoot)
			conversation := append([]llm.Message(nil), term.Conversation...)
			go r.extractPending(context.WithoutCancel(ctx), memDir, in.Model, conversation)
		}
	}()
	return out
}

func (r *Runner) buildAugmentedPrompt(ctx context.Context, in agent.Input, memDir string) string {
	if shouldSkipMemoryForListing(in) {
		return in.SystemPrompt
	}
	cfg := r.cfg
	if strings.TrimSpace(in.Model) != "" {
		cfg.Model = in.Model
	}
	st := newStore(memDir)
	if err := st.ensure(ctx); err != nil {
		return in.SystemPrompt
	}
	scanRes, err := Scan(ctx, memDir)
	if err != nil {
		return in.SystemPrompt
	}
	query := Query{
		LatestUser: latestUserMessage(in.Messages),
		Messages:   in.Messages,
	}
	if looksLikeListingQuery(query.LatestUser) {
		return in.SystemPrompt
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.RecallMode))
	if mode == "" {
		mode = "fast"
	}
	recallRes := RecallResult{}
	switch mode {
	case "off":
		// keep empty recall selection
	case "llm":
		recallCtx, cancel := context.WithTimeout(ctx, cfg.RecallTimeout)
		defer cancel()
		res, err := Recall(recallCtx, r.client, cfg, query, scanRes.Entries, nil)
		if err == nil {
			recallRes = res
		}
	default:
		// fast mode: use lexical relevance without an LLM side-query.
		limit := cfg.MaxSelected
		if limit <= 0 || limit > len(scanRes.Entries) {
			limit = len(scanRes.Entries)
		}
		type scored struct {
			entry Entry
			score int
		}
		scoredEntries := make([]scored, 0, len(scanRes.Entries))
		for i := range scanRes.Entries {
			scoredEntries = append(scoredEntries, scored{
				entry: scanRes.Entries[i],
				score: scoreMemoryEntry(query.LatestUser, scanRes.Entries[i]),
			})
		}
		sort.SliceStable(scoredEntries, func(i, j int) bool {
			if scoredEntries[i].score == scoredEntries[j].score {
				return scoredEntries[i].entry.Filename < scoredEntries[j].entry.Filename
			}
			return scoredEntries[i].score > scoredEntries[j].score
		})
		selected := make([]Entry, 0, limit)
		for i := 0; i < len(scoredEntries) && len(selected) < limit; i++ {
			if scoredEntries[i].score < 2 {
				continue
			}
			selected = append(selected, scoredEntries[i].entry)
		}
		recallRes.Selected = selected
	}
	if len(recallRes.Selected) == 0 {
		return in.SystemPrompt
	}
	index, err := st.loadIndex(cfg)
	if err != nil {
		return in.SystemPrompt
	}
	loaded, err := st.readSelected(recallRes.Selected, r.nowFn())
	if err != nil {
		loaded = nil
	}
	section := BuildSection(SectionInput{
		MemoryDir: memDir,
		Index:     index,
		Recalled:  loaded,
	})
	if strings.TrimSpace(in.SystemPrompt) == "" {
		return section
	}
	return in.SystemPrompt + "\n\n" + section
}

func shouldSkipMemoryForListing(in agent.Input) bool {
	if strings.EqualFold(strings.TrimSpace(in.PromptIntent), "directory_listing") {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(in.AttachmentPolicy), "listing_tree_only") {
		return true
	}
	latest := latestUserMessage(in.Messages)
	return strings.Contains(latest, "Directory tree data:") && looksLikeListingQuery(latest)
}

func looksLikeListingQuery(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "directory tree data:") {
		return true
	}
	return strings.Contains(lower, "list") && (strings.Contains(lower, "file") || strings.Contains(lower, "folder") || strings.Contains(lower, "directory"))
}

func scoreMemoryEntry(query string, entry Entry) int {
	terms := tokenSet(query)
	if len(terms) == 0 {
		return 0
	}
	target := strings.ToLower(entry.Filename + " " + entry.Name + " " + entry.Description + " " + string(entry.Type))
	score := 0
	for term := range terms {
		switch term {
		case "list", "file", "files", "folder", "folders", "directory", "tree", "project":
			continue
		}
		if strings.Contains(target, term) {
			score += 2
		}
	}
	return score
}

func tokenSet(s string) map[string]struct{} {
	cleaned := strings.ToLower(s)
	replacer := strings.NewReplacer(",", " ", ".", " ", ":", " ", ";", " ", "/", " ", "\\", " ", "(", " ", ")", " ", "[", " ", "]", " ", "{", " ", "}", " ", "\"", " ", "'", " ")
	cleaned = replacer.Replace(cleaned)
	parts := strings.Fields(cleaned)
	set := make(map[string]struct{}, len(parts))
	for _, p := range parts {
		if len(p) < 3 {
			continue
		}
		set[p] = struct{}{}
	}
	return set
}

func (r *Runner) extractPending(ctx context.Context, memDir, model string, conversation []llm.Message) int {
	cfg := r.cfg
	if strings.TrimSpace(model) != "" {
		cfg.Model = model
	}
	st := newStore(memDir)
	if err := st.ensure(ctx); err != nil {
		return 0
	}
	scanRes, err := Scan(ctx, memDir)
	if err != nil {
		return 0
	}
	extractCtx, cancel := context.WithTimeout(ctx, cfg.ExtractTimeout)
	defer cancel()
	drafts, err := ExtractDrafts(extractCtx, r.client, cfg, conversation, scanRes.Entries)
	if err != nil {
		return 0
	}
	written := 0
	for _, d := range drafts {
		// Prefix timestamp to reduce collisions while keeping model-proposed suffix.
		prefixed := Draft{
			Filename: time.Now().UTC().Format("20060102T150405Z") + "-" + filepath.Base(d.Filename),
			Content:  d.Content,
		}
		if _, err := st.writePending(ctx, prefixed); err == nil {
			written++
		}
	}
	return written
}

func latestUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}

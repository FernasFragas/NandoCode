package retrievalroute

import (
	"strings"
	"time"
	"unicode"
)

type Action string

const (
	ActionSkipAllRetrieval    Action = "skip_all_retrieval"
	ActionExplicitContextOnly Action = "use_explicit_context_only"
	ActionLocalSearchOnly     Action = "use_local_search_only"
	ActionSemanticLight       Action = "use_semantic_light"
	ActionSemanticFull        Action = "use_semantic_full"
)

type Reason string

const (
	ReasonSkipLocalCommand       Reason = "skip_local_command"
	ReasonSkipExplicitContext    Reason = "skip_explicit_context"
	ReasonSkipListingIntent      Reason = "skip_listing_intent"
	ReasonSkipMemoryRecall       Reason = "skip_memory_recall"
	ReasonSkipGeneralPrompt      Reason = "skip_general_prompt"
	ReasonRunRelatedContext      Reason = "run_related_context"
	ReasonRunWorkspaceDiscovery  Reason = "run_workspace_discovery"
	ReasonSkipIndexMissing       Reason = "skip_index_missing"
	ReasonSkipIndexStale         Reason = "skip_index_stale"
	ReasonSkipDimensionsMismatch Reason = "skip_dimensions_mismatch"
	ReasonSkipDeadline           Reason = "skip_deadline"
)

type Input struct {
	RawPrompt            string
	NormalizedPrompt     string
	ShouldQuery          bool
	AttachmentPolicy     string
	CurrentTurnPaths     []string
	CurrentTurnDirs      []string
	AttachedFileCount    int
	AttachedContextBytes int
	IndexKnown           bool
	HasIndex             bool
	IndexCompatible      bool
	SemanticEnabled      bool
	SemanticMode         string
	ForceDeep            bool
	PromptIntent         string
}

type ToolMode string

const (
	ToolModeDefault ToolMode = "default"
	ToolModeNone    ToolMode = "none"
)

type Limits struct {
	MaxRecords      int
	MaxFiles        int
	MaxContextBytes int
	Deadline        time.Duration
}

type Config struct {
	Mode  string
	Light Limits
	Full  Limits
	Deep  Limits
}

type Decision struct {
	Action               Action
	Reason               Reason
	AllowEmbedding       bool
	ToolMode             ToolMode
	MaxRecords           int
	MaxFiles             int
	MaxContextBytes      int
	Deadline             time.Duration
	UseCurrentPathWeight bool
	RequestProfile       string
	Profile              string
	StatusLabel          string
}

// Decide picks the retrieval action and, separately, the tool mode. The
// retrieval pipeline never sets ToolModeNone; decideToolMode owns that.
func Decide(input Input, cfg Config) Decision {
	d := decideRetrieval(input, cfg)
	d.ToolMode = decideToolMode(input, normalizePrompt(input), d)
	return d
}

func decideRetrieval(input Input, cfg Config) Decision {
	cfg = normalizeConfig(cfg)
	prompt := normalizePrompt(input)
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "auto"
	}

	if !input.ShouldQuery || strings.HasPrefix(prompt, "/") {
		return skipDecision(ActionSkipAllRetrieval, ReasonSkipLocalCommand, "Preparing model request...", "local_command", ToolModeDefault)
	}
	if isListingIntent(input) {
		return skipDecision(ActionLocalSearchOnly, ReasonSkipListingIntent, "Searching workspace...", "listing_intent", ToolModeDefault)
	}
	if isMemoryRecallIntent(prompt) {
		return skipDecision(ActionSkipAllRetrieval, ReasonSkipMemoryRecall, "Preparing model request...", "memory_recall", ToolModeDefault)
	}

	hasExplicitContext := input.AttachedFileCount > 0 || len(input.CurrentTurnPaths) > 0 || len(input.CurrentTurnDirs) > 0
	if input.PromptIntent == "file_status" {
		return skipDecision(ActionExplicitContextOnly, ReasonSkipExplicitContext, "Reading mentioned file...", "file_status", ToolModeDefault)
	}
	if idxDecision, ok := indexUnavailableDecision(input, mode, hasExplicitContext, prompt); ok {
		return idxDecision
	}

	if mode == "explicit" {
		if hasExplicitContext && isRelatedCodePrompt(prompt) && input.SemanticEnabled {
			return semanticDecision(ActionSemanticLight, ReasonRunRelatedContext, cfg.Light, true, "light", "explicit_related")
		}
		return skipDecision(ActionExplicitContextOnly, ReasonSkipExplicitContext, "Reading mentioned file...", "explicit_context", ToolModeDefault)
	}
	if mode == "off" || !input.SemanticEnabled {
		return skipDecision(ActionExplicitContextOnly, ReasonSkipExplicitContext, "Preparing model request...", "semantic_off", ToolModeDefault)
	}

	if input.ForceDeep {
		return semanticDecision(ActionSemanticFull, ReasonRunWorkspaceDiscovery, cfg.Deep, false, "deep", "workspace_deep")
	}

	if hasExplicitContext {
		if isRelatedCodePrompt(prompt) {
			return semanticDecision(ActionSemanticLight, ReasonRunRelatedContext, cfg.Light, true, "light", "related_context")
		}
		return skipDecision(ActionExplicitContextOnly, ReasonSkipExplicitContext, "Reading mentioned file...", "explicit_context", ToolModeDefault)
	}

	if !isWorkspaceDiscoveryPrompt(prompt) {
		return skipDecision(ActionSkipAllRetrieval, ReasonSkipGeneralPrompt, "Preparing model request...", "general_prompt", ToolModeDefault)
	}

	return semanticDecision(ActionSemanticFull, ReasonRunWorkspaceDiscovery, cfg.Full, false, "full", "workspace_discovery")
}

func indexUnavailableDecision(input Input, mode string, hasExplicitContext bool, prompt string) (Decision, bool) {
	if !input.SemanticEnabled || !input.IndexKnown || (input.HasIndex && input.IndexCompatible) {
		return Decision{}, false
	}
	wouldUseSemantic := input.ForceDeep ||
		mode == "explicit" ||
		isWorkspaceDiscoveryPrompt(prompt) ||
		(hasExplicitContext && isRelatedCodePrompt(prompt))
	if !wouldUseSemantic {
		return Decision{}, false
	}
	reason := ReasonSkipIndexMissing
	if input.HasIndex && !input.IndexCompatible {
		reason = ReasonSkipDimensionsMismatch
	}
	action := ActionSkipAllRetrieval
	label := "Semantic index unavailable..."
	profile := "semantic_index_unavailable"
	if hasExplicitContext {
		action = ActionExplicitContextOnly
		label = "Reading mentioned file..."
		profile = "explicit_context"
	}
	return skipDecision(action, reason, label, profile, ToolModeDefault), true
}

func normalizeConfig(cfg Config) Config {
	if cfg.Light.MaxRecords <= 0 {
		cfg.Light.MaxRecords = 4
	}
	if cfg.Light.MaxFiles <= 0 {
		cfg.Light.MaxFiles = 1
	}
	if cfg.Light.MaxContextBytes <= 0 {
		cfg.Light.MaxContextBytes = 8 * 1024
	}
	if cfg.Light.Deadline <= 0 {
		cfg.Light.Deadline = 1200 * time.Millisecond
	}
	if cfg.Full.MaxRecords <= 0 {
		cfg.Full.MaxRecords = 12
	}
	if cfg.Full.MaxFiles <= 0 {
		cfg.Full.MaxFiles = 4
	}
	if cfg.Full.MaxContextBytes <= 0 {
		cfg.Full.MaxContextBytes = 64 * 1024
	}
	if cfg.Full.Deadline <= 0 {
		cfg.Full.Deadline = 3 * time.Second
	}
	if cfg.Deep.MaxRecords <= 0 {
		cfg.Deep.MaxRecords = 40
	}
	if cfg.Deep.MaxFiles <= 0 {
		cfg.Deep.MaxFiles = 12
	}
	if cfg.Deep.MaxContextBytes <= 0 {
		cfg.Deep.MaxContextBytes = 256 * 1024
	}
	if cfg.Deep.Deadline <= 0 {
		cfg.Deep.Deadline = 3 * time.Second
	}
	if strings.TrimSpace(cfg.Mode) == "" {
		cfg.Mode = "auto"
	}
	return cfg
}

func normalizePrompt(input Input) string {
	if strings.TrimSpace(input.NormalizedPrompt) != "" {
		return strings.ToLower(strings.TrimSpace(input.NormalizedPrompt))
	}
	return strings.ToLower(strings.TrimSpace(input.RawPrompt))
}

func isListingIntent(input Input) bool {
	if input.AttachmentPolicy == "listing_tree_only" {
		return true
	}
	switch input.PromptIntent {
	case "directory_listing", "directory_listing_with_content":
		return true
	default:
		return false
	}
}

func isMemoryRecallIntent(prompt string) bool {
	return strings.Contains(prompt, "from memory") ||
		strings.Contains(prompt, "recall") ||
		strings.Contains(prompt, "previous context")
}

func isRelatedCodePrompt(prompt string) bool {
	return containsAny(prompt, []string{
		"related", "caller", "callers", "utilities", "utility", "references", "referenced",
		"where is", "where it's", "where its", "used by", "imports", "imported by",
		"dependency", "dependencies", "adjacent", "nearby", "touching", "touches",
	})
}

func isWorkspaceDiscoveryPrompt(prompt string) bool {
	return hasCodeWord(prompt)
}

// codeWords are whole words that mark a prompt as being about the workspace.
// Matching is by word (not substring) so "capital" does not match "api" and
// "author" does not match "auth".
var codeWords = wordSet(
	"fix", "debug", "bug", "broken", "error", "failing", "failure", "panic",
	"implement", "refactor", "code", "codebase", "repo", "repository", "workspace",
	"project", "file", "function", "method", "class", "package", "module", "test",
	"auth", "authentication", "handler", "service", "api", "cli", "tui",
	"endpoint", "server", "session", "model", "route", "verify", "agent",
	"event", "loop", "struct", "interface", "variable", "compile", "build",
	"config", "configuration", "database", "schema", "query", "parser",
	"middleware", "request", "response", "stack", "trace", "commit", "branch",
	"diff", "lint", "dependency", "dependencies", "import", "library",
)

// toolWords are action verbs and shell/tool nouns. They keep tools enabled but
// do not by themselves trigger semantic retrieval. When in doubt, keep tools.
var toolWords = wordSet(
	"read", "write", "edit", "update", "modify", "change", "changed", "changes",
	"run", "execute", "delete", "remove", "rename", "move", "copy", "create",
	"add", "show", "list", "open", "find", "search", "grep", "look", "check",
	"summarize", "summarise", "inspect", "review", "install", "start", "stop",
	"git", "npm", "ls", "cat", "make", "go", "docker", "bash", "shell", "sh",
	"status", "log", "logs", "logging", "file", "folder", "directory", "dir",
	"readme", "todo", "path", "output", "terminal", "command", "script",
)

func wordSet(words ...string) map[string]struct{} {
	m := make(map[string]struct{}, len(words))
	for _, w := range words {
		m[w] = struct{}{}
	}
	return m
}

func hasCodeWord(prompt string) bool { return hasWordIn(prompt, codeWords) }

func hasToolWord(prompt string) bool { return hasWordIn(prompt, toolWords) }

func hasWordIn(prompt string, set map[string]struct{}) bool {
	words := strings.FieldsFunc(strings.ToLower(prompt), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, w := range words {
		if _, ok := set[w]; ok {
			return true
		}
		if strings.HasSuffix(w, "s") {
			if _, ok := set[strings.TrimSuffix(w, "s")]; ok {
				return true
			}
		}
	}
	return false
}

// hasIdentifierToken reports whether the raw prompt contains something that
// looks like a code identifier or path: snake_case, camelCase, a path
// separator, a file extension, or a call such as foo().
func hasIdentifierToken(raw string) bool {
	for _, tok := range strings.Fields(raw) {
		tok = strings.TrimFunc(tok, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
		})
		if tok == "" {
			continue
		}
		if strings.ContainsAny(tok, "_/\\") {
			return true
		}
		if i := strings.LastIndex(tok, "."); i > 0 && len(tok)-i-1 >= 2 {
			return true
		}
		runes := []rune(tok)
		for i := 1; i < len(runes); i++ {
			if unicode.IsLower(runes[i-1]) && unicode.IsUpper(runes[i]) {
				return true
			}
		}
	}
	return strings.Contains(raw, "()")
}

// decideToolMode disables tools only for general prompts (semantic retrieval on)
// the workspace. It is independent of whether semantic retrieval is enabled.
// Anything with explicit context, a local command, listing, memory recall,
// file status, code vocabulary or an identifier keeps its tools.
func decideToolMode(input Input, prompt string, d Decision) ToolMode {
	if !input.ShouldQuery || strings.HasPrefix(prompt, "/") || isListingIntent(input) || isMemoryRecallIntent(prompt) {
		return ToolModeDefault
	}
	if input.PromptIntent == "file_status" ||
		input.AttachedFileCount > 0 || len(input.CurrentTurnPaths) > 0 || len(input.CurrentTurnDirs) > 0 {
		return ToolModeDefault
	}
	// Only the general-prompt branch (semantic retrieval on, nothing
	// workspace-related) may disable tools.
	if d.Reason != ReasonSkipGeneralPrompt {
		return ToolModeDefault
	}
	if hasCodeWord(prompt) || hasToolWord(prompt) || isRelatedCodePrompt(prompt) || hasIdentifierToken(input.RawPrompt+" "+input.NormalizedPrompt) {
		return ToolModeDefault
	}
	return ToolModeNone
}

func containsAny(s string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(s, n) {
			return true
		}
	}
	return false
}

func skipDecision(action Action, reason Reason, status, requestProfile string, toolMode ToolMode) Decision {
	if strings.TrimSpace(requestProfile) == "" {
		requestProfile = "default"
	}
	if strings.TrimSpace(string(toolMode)) == "" {
		toolMode = ToolModeDefault
	}
	return Decision{
		Action:         action,
		Reason:         reason,
		AllowEmbedding: false,
		ToolMode:       toolMode,
		RequestProfile: requestProfile,
		StatusLabel:    status,
	}
}

func semanticDecision(action Action, reason Reason, limits Limits, useCurrentPathWeight bool, profile, requestProfile string) Decision {
	if strings.TrimSpace(requestProfile) == "" {
		requestProfile = "default"
	}
	return Decision{
		Action:               action,
		Reason:               reason,
		AllowEmbedding:       true,
		ToolMode:             ToolModeDefault,
		MaxRecords:           limits.MaxRecords,
		MaxFiles:             limits.MaxFiles,
		MaxContextBytes:      limits.MaxContextBytes,
		Deadline:             limits.Deadline,
		UseCurrentPathWeight: useCurrentPathWeight,
		RequestProfile:       requestProfile,
		Profile:              profile,
		StatusLabel:          "Embedding query...",
	}
}

// Package turnprep is the one place that turns a user prompt into an
// agent.Input: mention packing, retrieval routing, optional semantic
// retrieval, and the grounding system prompt. The TUI, the server, --print and
// the eval harness all call Prepare, so a routing or packing fix lands once.
//
// Prepare is surface neutral. It never prints or emits events; surfaces observe
// progress through Hooks and format the returned Prepared themselves.
package turnprep

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/FernasFragas/Nandocode/internal/agent"
	"github.com/FernasFragas/Nandocode/internal/analysis"
	"github.com/FernasFragas/Nandocode/internal/contextpack"
	"github.com/FernasFragas/Nandocode/internal/llm"
	"github.com/FernasFragas/Nandocode/internal/mentions"
	"github.com/FernasFragas/Nandocode/internal/retrievalroute"
	"github.com/FernasFragas/Nandocode/internal/semantic"
	"github.com/FernasFragas/Nandocode/internal/tools"
)

// Routing selects how the retrieval route is decided.
type Routing int

const (
	// RoutingAuto routes with the surface's semantic service and config
	// (TUI and server). Semantic retrieval runs when the route allows it.
	RoutingAuto Routing = iota
	// RoutingStaticAuto routes as if semantic search were enabled in auto mode
	// with no index, and never retrieves (--print).
	RoutingStaticAuto
	// RoutingNone skips routing and retrieval; the route fields stay empty
	// (eval).
	RoutingNone
)

// Semantic carries the optional semantic retrieval service and its config.
type Semantic struct {
	Service semantic.Service
	Config  semantic.Config
}

// Hooks let a surface observe preparation as it happens. All are optional.
type Hooks struct {
	OnRoute         func(retrievalroute.Decision)
	OnSemanticStart func()
	OnSemanticStage func(semantic.RetrieveStageEvent)
}

// Request describes one turn.
type Request struct {
	// Ctx bounds semantic retrieval and the index status probe. Nil means
	// context.Background().
	Ctx context.Context
	// Prompt is the text the user typed. It drives routing, semantic queries
	// and Input.OriginalUserText.
	Prompt string
	// Input is the text to pack. Empty means Prompt. The TUI sets it when a
	// checkpoint resume rewrites the prompt.
	Input string
	// PreExpanded skips mention packing: Input is already the final prompt.
	PreExpanded bool

	ToolContext     tools.Context
	AgentConfig     agent.Config
	Model           string
	LLMProvider     string
	ContextMode     string
	MaxOutputTokens int
	History         []llm.Message

	Semantic  Semantic
	Routing   Routing
	ForceDeep bool

	// SystemPrompt is the surface's own system prompt (eval, coordinator).
	SystemPrompt string
	// Grounding appends GroundingPrompt when the run has tools.
	Grounding bool
	// TrimToLatest sets Input.Messages to the current user message when the
	// history policy is latest-only (the TUI does this itself; the agent would
	// trim anyway).
	TrimToLatest bool

	Hooks Hooks
}

// SemanticKind classifies what semantic retrieval did.
type SemanticKind int

const (
	SemanticSkipped  SemanticKind = iota // not attempted
	SemanticUsed                         // context appended to the prompt
	SemanticFallback                     // attempted, fell back; Reason says why
	SemanticError                        // attempted, hard error
)

// SemanticOutcome reports semantic retrieval for the surface to display.
type SemanticOutcome struct {
	Attempted bool
	Kind      SemanticKind
	Result    semantic.RetrieveResult
	Err       error
	// Reason is the fallback reason for SemanticFallback.
	Reason   string
	Duration time.Duration
}

// Evidence is the user-visible state of what grounds this turn.
type Evidence struct {
	AttachedFiles  int
	AttachedDirs   int
	SemanticUsed   bool
	ToolsAvailable bool
	Grounded       bool
}

// Prepared is the result of Prepare.
type Prepared struct {
	// Input is ready to run once the surface adds permission wiring,
	// ToolContext overrides and channels.
	Input agent.Input
	// Messages is History plus the new user message, never trimmed.
	Messages []llm.Message
	// UserMessage is the new user message (packed prompt plus semantic context).
	UserMessage llm.Message
	// Expanded is UserMessage.Content.
	Expanded        string
	Files           []mentions.ResolvedFile
	Dirs            []mentions.ResolvedDirectory
	ExpansionReport mentions.ExpansionReport
	Route           retrievalroute.Decision
	Semantic        SemanticOutcome
	Evidence        Evidence
	// PackDuration is the mention packing time. It is set on error too.
	PackDuration time.Duration
}

// IsEvidenceTooLarge reports whether err is a contextpack.ErrEvidenceTooLarge.
func IsEvidenceTooLarge(err error) bool {
	var tooLarge contextpack.ErrEvidenceTooLarge
	return errors.As(err, &tooLarge)
}

// Prepare builds the agent.Input for one turn.
func Prepare(req Request) (Prepared, error) {
	ctx := req.Ctx
	if ctx == nil {
		ctx = context.Background()
	}
	input := req.Input
	if input == "" {
		input = req.Prompt
	}

	var out Prepared
	expanded := input
	var evidencePack *agent.EvidencePackReport
	if !req.PreExpanded {
		start := time.Now()
		packed, _, err := contextpack.BuildCurrentTurnPrompt(input, req.ToolContext, req.AgentConfig, agent.Input{
			Model:           req.Model,
			ContextMode:     req.ContextMode,
			MaxOutputTokens: req.MaxOutputTokens,
		}, req.History)
		out.PackDuration = time.Since(start)
		if err != nil {
			return out, err
		}
		expanded = packed.Prompt
		out.Files = packed.Files
		out.Dirs = packed.Dirs
		out.ExpansionReport = packed.ExpansionReport
		rep := packed.PackReport
		evidencePack = &rep
	}

	currentTurn := make([]string, 0, len(out.Files))
	for _, f := range out.Files {
		if strings.TrimSpace(f.Path) != "" {
			currentTurn = append(currentTurn, f.Path)
		}
	}
	currentDirs := make([]string, 0, len(out.Dirs))
	for _, d := range out.Dirs {
		if strings.TrimSpace(d.Path) != "" {
			currentDirs = append(currentDirs, d.Path)
		}
	}

	intent := out.ExpansionReport.Intent
	switch req.Routing {
	case RoutingStaticAuto:
		out.Route = retrievalroute.Decide(retrievalroute.Input{
			RawPrompt:            req.Prompt,
			ShouldQuery:          true,
			AttachmentPolicy:     string(intent.AttachmentPolicy),
			CurrentTurnPaths:     currentTurn,
			CurrentTurnDirs:      currentDirs,
			AttachedFileCount:    len(out.Files),
			AttachedContextBytes: len(expanded),
			SemanticEnabled:      true,
			SemanticMode:         "auto",
			PromptIntent:         string(intent.Kind),
		}, retrievalroute.Config{Mode: "auto"})
		callRoute(req.Hooks, out.Route)
	case RoutingNone:
		// No route: the surface runs with default tool mode and no retrieval.
	default:
		svc, cfg := req.Semantic.Service, req.Semantic.Config
		routeCfg := SemanticRouteConfig(cfg, svc != nil)
		known, hasIndex, compatible := semanticIndexStatus(ctx, svc, cfg, req.ToolContext.WorkingDir)
		out.Route = retrievalroute.Decide(retrievalroute.Input{
			RawPrompt:            req.Prompt,
			ShouldQuery:          true,
			AttachmentPolicy:     string(intent.AttachmentPolicy),
			CurrentTurnPaths:     currentTurn,
			CurrentTurnDirs:      currentDirs,
			AttachedFileCount:    len(out.Files),
			AttachedContextBytes: len(expanded),
			IndexKnown:           known,
			HasIndex:             hasIndex,
			IndexCompatible:      compatible,
			SemanticEnabled:      svc != nil && cfg.Enabled,
			SemanticMode:         routeCfg.Mode,
			ForceDeep:            req.ForceDeep,
			PromptIntent:         string(intent.Kind),
		}, routeCfg)
		callRoute(req.Hooks, out.Route)
		if out.Route.AllowEmbedding && svc != nil {
			expanded, out.Semantic = retrieve(ctx, req, out.Route, expanded, currentTurn)
		}
	}

	userMessage := llm.Message{Role: llm.RoleUser, Content: expanded}
	messages := append(append([]llm.Message(nil), req.History...), userMessage)
	historyPolicy := agent.HistoryPolicyDefault
	if intent.AttachmentPolicy == mentions.AttachListingTreeOnly || intent.Kind == mentions.IntentFileStatus {
		historyPolicy = agent.HistoryPolicyLatestOnly
	}
	runMessages := messages
	if req.TrimToLatest && historyPolicy == agent.HistoryPolicyLatestOnly {
		runMessages = []llm.Message{userMessage}
	}

	toolsAvailable := string(out.Route.ToolMode) != agent.ToolModeNone
	systemPrompt := req.SystemPrompt
	grounded := req.Grounding && toolsAvailable
	if grounded {
		systemPrompt = composeSystemPrompt(systemPrompt)
	}

	out.Input = agent.Input{
		Model:            req.Model,
		LLMProvider:      req.LLMProvider,
		SystemPrompt:     systemPrompt,
		Messages:         runMessages,
		ToolContext:      req.ToolContext,
		ContextMode:      req.ContextMode,
		PromptIntent:     string(intent.Kind),
		AttachmentPolicy: string(intent.AttachmentPolicy),
		OriginalUserText: req.Prompt,
		HistoryPolicy:    historyPolicy,
		ToolMode:         string(out.Route.ToolMode),
		RouteAction:      string(out.Route.Action),
		RouteReason:      string(out.Route.Reason),
		RouteProfile:     out.Route.RequestProfile,
		EvidencePack:     evidencePack,
		MaxOutputTokens:  req.MaxOutputTokens,
	}
	out.Messages = messages
	out.UserMessage = userMessage
	out.Expanded = expanded
	out.Evidence = Evidence{
		AttachedFiles:  len(out.Files),
		AttachedDirs:   len(out.Dirs),
		SemanticUsed:   out.Semantic.Kind == SemanticUsed,
		ToolsAvailable: toolsAvailable,
		Grounded:       grounded,
	}
	return out, nil
}

func callRoute(h Hooks, d retrievalroute.Decision) {
	if h.OnRoute != nil {
		h.OnRoute(d)
	}
}

func retrieve(ctx context.Context, req Request, route retrievalroute.Decision, expanded string, currentTurn []string) (string, SemanticOutcome) {
	if req.Hooks.OnSemanticStart != nil {
		req.Hooks.OnSemanticStart()
	}
	start := time.Now()
	res, err := req.Semantic.Service.Retrieve(ctx, semantic.RetrieveRequest{
		Root:                 req.ToolContext.WorkingDir,
		Query:                req.Prompt,
		Config:               req.Semantic.Config,
		ExplicitPaths:        analysis.ExtractMentionedPaths(req.Prompt),
		CurrentTurnPaths:     currentTurn,
		Deadline:             route.Deadline,
		RouteAction:          string(route.Action),
		RouteReason:          string(route.Reason),
		RouteProfile:         route.Profile,
		UseCurrentPathWeight: route.UseCurrentPathWeight,
		MaxRecords:           route.MaxRecords,
		MaxFiles:             route.MaxFiles,
		MaxContextBytes:      route.MaxContextBytes,
		Observer: func(evt semantic.RetrieveStageEvent) {
			if req.Hooks.OnSemanticStage != nil {
				req.Hooks.OnSemanticStage(evt)
			}
		},
	})
	outcome := SemanticOutcome{Attempted: true, Result: res, Err: err, Duration: time.Since(start)}
	switch {
	case err == nil && res.Used:
		outcome.Kind = SemanticUsed
		expanded = strings.TrimRight(expanded, "\n") + "\n\n" + strings.TrimSpace(res.RenderedContext)
	case err != nil && semantic.IsFallbackError(err):
		outcome.Kind = SemanticFallback
		outcome.Reason = err.Error()
	case err != nil:
		outcome.Kind = SemanticError
	case res.FallbackReason != "":
		outcome.Kind = SemanticFallback
		outcome.Reason = res.FallbackReason
	}
	return expanded, outcome
}

// SemanticRouteConfig maps the semantic config to a route config. Semantic
// search is off when the config is disabled or there is no service.
func SemanticRouteConfig(cfg semantic.Config, hasService bool) retrievalroute.Config {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "auto"
	}
	if !cfg.Enabled || !hasService {
		mode = "off"
	}
	return retrievalroute.Config{
		Mode: mode,
		Light: retrievalroute.Limits{
			MaxRecords:      cfg.LightTopKRecords,
			MaxFiles:        cfg.LightTopKFiles,
			MaxContextBytes: cfg.LightMaxContextBytes,
			Deadline:        time.Duration(cfg.LightDeadlineMS) * time.Millisecond,
		},
		Full: retrievalroute.Limits{
			MaxRecords:      cfg.FullTopKRecords,
			MaxFiles:        cfg.FullTopKFiles,
			MaxContextBytes: cfg.FullMaxContextBytes,
			Deadline:        time.Duration(cfg.FullDeadlineMS) * time.Millisecond,
		},
		Deep: retrievalroute.Limits{
			MaxRecords:      cfg.DeepTopKRecords,
			MaxFiles:        cfg.DeepTopKFiles,
			MaxContextBytes: cfg.DeepMaxContextBytes,
			Deadline:        time.Duration(cfg.DeepDeadlineMS) * time.Millisecond,
		},
	}
}

// semanticIndexStatus probes the index for routing. It never blocks long: a
// slow or failing probe reports "unknown".
func semanticIndexStatus(ctx context.Context, svc semantic.Service, cfg semantic.Config, root string) (known, exists, compatible bool) {
	if svc == nil || !cfg.Enabled {
		return false, false, false
	}
	ctx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	status, err := svc.Status(ctx, root)
	if err != nil {
		return false, false, false
	}
	return true, status.Exists, status.Compatible
}

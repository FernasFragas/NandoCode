package tui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/FernasFragas/nandocodego/internal/agent"
	"github.com/FernasFragas/nandocodego/internal/analysis"
	"github.com/FernasFragas/nandocodego/internal/commands"
	"github.com/FernasFragas/nandocodego/internal/contextpack"
	"github.com/FernasFragas/nandocodego/internal/credentials"
	"github.com/FernasFragas/nandocodego/internal/hooks"
	"github.com/FernasFragas/nandocodego/internal/llm"
	"github.com/FernasFragas/nandocodego/internal/llm/modelruntime"
	"github.com/FernasFragas/nandocodego/internal/mentions"
	"github.com/FernasFragas/nandocodego/internal/observability"
	"github.com/FernasFragas/nandocodego/internal/permissions"
	"github.com/FernasFragas/nandocodego/internal/retrievalroute"
	"github.com/FernasFragas/nandocodego/internal/semantic"
	"github.com/FernasFragas/nandocodego/internal/skills"
	"github.com/FernasFragas/nandocodego/internal/state"
	"github.com/FernasFragas/nandocodego/internal/tui/fileindex"
	"github.com/FernasFragas/nandocodego/internal/tui/picker"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Model is the root Bubble Tea model for the REPL.
type Model struct {
	store                 *state.Store[state.App]
	agent                 AgentRunner
	program               ProgramSender
	viewport              viewport.Model
	input                 textarea.Model
	renderer              *MarkdownRenderer
	styles                Styles
	vim                   *VimState
	broker                *PermissionBroker
	width                 int
	height                int
	cancelRun             context.CancelFunc
	compactCh             chan struct{}
	transcript            []TranscriptItem
	toolIndex             map[string]int
	err                   error
	activeRunCtx          context.Context
	skillLoader           *skills.Loader
	cmdRegistry           *commands.Registry
	cmdCtx                commands.HandlerContext
	meter                 *observability.Meter
	startupNotes          []string
	fileIndex             *fileindex.Index
	frecency              *fileindex.Frecency
	picker                picker.State
	providers             map[picker.Trigger]picker.Provider
	thinkingActive        bool
	runStartedAt          time.Time
	firstStreamAt         time.Time
	firstRenderSet        bool
	slowStageNotified     map[string]bool
	compactingActive      bool
	retryActiveUntil      time.Time
	heightCache           map[int]int
	lastStreamRenderAt    time.Time
	everHadInput          bool
	lastGChordAt          time.Time
	searchQuery           string
	searchMatches         []int
	searchPos             int
	bindingStack          *BindingStack
	chordInterceptor      *ChordInterceptor
	inputPreprocessor     *InputPreprocessor
	backgroundedRun       bool
	pendingBTW            string
	activeRunKind         string
	resolvedActiveModel   string
	cloudCredentialPrompt *cloudCredentialPromptState
	killDream             func()
	consumeDream          func() string
	spawnDream            func()
	semanticService       semantic.Service
	semanticConfig        semantic.Config
	semanticDeepNext      bool
	indexProgress         indexProgressState
	transcriptRenderKey   transcriptCacheKey
	transcriptRender      string
	transcriptRenderValid bool
	viewportRenderKey     transcriptCacheKey
	viewportRenderValid   bool
	markdownRenderWidth   int
}

type cloudCredentialPromptState struct {
	Requested           string
	Resolved            llm.ResolvedModel
	ForPromptSubmission bool
	Input               string
	DisplayInput        string
	PreExpanded         bool
	KeyInput            textinput.Model
	FocusIndex          int
	Error               string
}

type transcriptCacheKey struct {
	MutationSig uint64
	Start       int
	End         int
	ActiveRun   bool
	Width       int
	StyleSig    uint64
	EverHad     bool
}

type promptResult struct {
	Key  string
	Save bool
}

type modelSwitchRequest struct {
	Requested           string
	ForPromptSubmission bool
	Input               string
	DisplayInput        string
	PreExpanded         bool
	PromptResult        *promptResult
}

// New creates a new REPL model.
func New(
	appStore *state.Store[state.App],
	runner AgentRunner,
	program ProgramSender,
	loader *skills.Loader,
	llmClient llm.Client,
	hookSnapshot *hooks.Snapshot,
	hookReloadSet func(hooks.Snapshot),
	memoryDir string,
	hookUserPath string,
	hookProjectPath string,
	startupNotices []string,
) (*Model, error) {
	// Create markdown renderer with a default width
	renderer, err := NewMarkdownRenderer(80)
	if err != nil {
		return nil, err
	}

	// Initialize textarea
	inputBox := textarea.New()
	inputBox.Placeholder = "Type a prompt or /help for commands..."
	inputBox.ShowLineNumbers = false
	inputBox.Blur()

	// Initialize viewport
	vp := viewport.New(80, 20)

	// Initialize Vim state
	vimState := NewVimState()

	// Permission broker sends messages to itself (will be set properly in Init)
	broker := NewPermissionBroker(func(msg tea.Msg) {
		// Placeholder; will be replaced in Init
	})

	reg := commands.New()
	commands.RegisterDefaults(reg)

	workingDir := appStore.Get().ToolSettings.WorkingDir
	idx := fileindex.New(workingDir)
	frecency := fileindex.NewFrecency()
	return &Model{
		store:       appStore,
		agent:       runner,
		program:     program,
		viewport:    vp,
		input:       inputBox,
		renderer:    renderer,
		styles:      DefaultStyles(),
		vim:         vimState,
		broker:      broker,
		transcript:  []TranscriptItem{},
		toolIndex:   make(map[string]int),
		skillLoader: loader,
		cmdRegistry: reg,
		cmdCtx: commands.HandlerContext{
			Store:         appStore,
			LLMClient:     llmClient,
			MemoryDir:     memoryDir,
			SkillLoader:   loader,
			HookUserPath:  hookUserPath,
			HookProjPath:  hookProjectPath,
			HookSnapshot:  hookSnapshot,
			HookReloadSet: hookReloadSet,
		},
		startupNotes:   append([]string(nil), startupNotices...),
		fileIndex:      idx,
		frecency:       frecency,
		semanticConfig: semantic.DefaultConfig(),
		providers: map[picker.Trigger]picker.Provider{
			picker.TriggerFile:    picker.NewFileProvider(idx, frecency),
			picker.TriggerCommand: picker.NewCommandProvider(reg),
		},
		slowStageNotified:   map[string]bool{},
		heightCache:         map[int]int{},
		bindingStack:        NewBindingStack(),
		chordInterceptor:    NewChordInterceptor(time.Second),
		inputPreprocessor:   NewInputPreprocessor(),
		activeRunKind:       "main",
		markdownRenderWidth: 80,
	}, nil
}

// SetProgramSender updates the program sender after model construction.
func (m *Model) SetProgramSender(sender ProgramSender) {
	m.program = sender
}

func (m *Model) SetMeter(meter *observability.Meter) {
	m.meter = meter
	m.cmdCtx.Meter = meter
}

func (m *Model) SetModelRuntime(runtime *modelruntime.Service) {
	m.cmdCtx.ModelRuntime = runtime
}

func (m *Model) SetDreamHooks(kill func(), consume func() string, spawn func()) {
	m.killDream = kill
	m.consumeDream = consume
	m.spawnDream = spawn
}

func (m *Model) SetSemanticService(service semantic.Service, cfg semantic.Config) {
	m.semanticService = service
	m.semanticConfig = cfg
}

func (m *Model) startModelSwitchCmd(req modelSwitchRequest) tea.Cmd {
	return func() tea.Msg {
		runtime := m.cmdCtx.ModelRuntime
		if runtime == nil {
			return modelSwitchFailedMsg{
				Requested:           req.Requested,
				Err:                 errors.New("model runtime unavailable"),
				ForPromptSubmission: req.ForPromptSubmission,
				Input:               req.Input,
				DisplayInput:        req.DisplayInput,
				PreExpanded:         req.PreExpanded,
			}
		}
		resolved, err := runtime.Resolve(context.Background(), req.Requested)
		if err != nil {
			return modelSwitchFailedMsg{
				Requested:           req.Requested,
				Err:                 err,
				ForPromptSubmission: req.ForPromptSubmission,
				Input:               req.Input,
				DisplayInput:        req.DisplayInput,
				PreExpanded:         req.PreExpanded,
			}
		}

		switchOpts := modelruntime.SwitchOptions{
			RequestedModel: req.Requested,
			AllowPrompt:    false,
		}
		if req.PromptResult != nil {
			switchOpts.AllowPrompt = true
			p := *req.PromptResult
			switchOpts.Prompt = func(context.Context, credentials.Prompt) (credentials.PromptResult, error) {
				return credentials.PromptResult{
					Key:      p.Key,
					Save:     p.Save,
					Canceled: false,
				}, nil
			}
		}
		result, err := runtime.Switch(context.Background(), switchOpts)
		if err != nil {
			if errors.Is(err, modelruntime.ErrCredentialRequired) && req.PromptResult == nil {
				return modelSwitchNeedsCredentialMsg{
					Requested:           req.Requested,
					Resolved:            resolved,
					ForPromptSubmission: req.ForPromptSubmission,
					Input:               req.Input,
					DisplayInput:        req.DisplayInput,
					PreExpanded:         req.PreExpanded,
				}
			}
			return modelSwitchFailedMsg{
				Requested:           req.Requested,
				Err:                 err,
				ForPromptSubmission: req.ForPromptSubmission,
				Input:               req.Input,
				DisplayInput:        req.DisplayInput,
				PreExpanded:         req.PreExpanded,
			}
		}
		return modelSwitchCompletedMsg{
			Result:              result,
			ForPromptSubmission: req.ForPromptSubmission,
			Input:               req.Input,
			DisplayInput:        req.DisplayInput,
			PreExpanded:         req.PreExpanded,
		}
	}
}

func newCloudCredentialPrompt(msg modelSwitchNeedsCredentialMsg) cloudCredentialPromptState {
	ti := textinput.New()
	ti.Placeholder = "Enter Ollama API key"
	ti.Prompt = "API key: "
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '*'
	ti.Focus()
	return cloudCredentialPromptState{
		Requested:           msg.Requested,
		Resolved:            msg.Resolved,
		ForPromptSubmission: msg.ForPromptSubmission,
		Input:               msg.Input,
		DisplayInput:        msg.DisplayInput,
		PreExpanded:         msg.PreExpanded,
		KeyInput:            ti,
		FocusIndex:          0,
	}
}

func (m *Model) handleCloudCredentialKeyMsg(msg tea.KeyMsg) tea.Cmd {
	if m.cloudCredentialPrompt == nil {
		return nil
	}
	p := m.cloudCredentialPrompt
	if msg.Paste {
		p.FocusIndex = 0
		p.Error = ""
		p.KeyInput.Focus()
		var cmd tea.Cmd
		p.KeyInput, cmd = p.KeyInput.Update(msg)
		m.cloudCredentialPrompt = p
		return cmd
	}
	switch msg.Type {
	case tea.KeyEsc:
		return func() tea.Msg { return cloudCredentialResolvedMsg{Cancel: true} }
	case tea.KeyTab:
		p.FocusIndex = (p.FocusIndex + 1) % 4
		p.Error = ""
	case tea.KeyShiftTab:
		p.FocusIndex = (p.FocusIndex - 1 + 4) % 4
		p.Error = ""
	case tea.KeyLeft:
		if p.FocusIndex > 0 {
			p.FocusIndex--
		}
	case tea.KeyRight:
		if p.FocusIndex < 3 {
			p.FocusIndex++
		}
	case tea.KeyEnter:
		switch p.FocusIndex {
		case 0, 1:
			return func() tea.Msg {
				return cloudCredentialResolvedMsg{
					Key:  p.KeyInput.Value(),
					Save: false,
				}
			}
		case 2:
			return func() tea.Msg {
				return cloudCredentialResolvedMsg{
					Key:  p.KeyInput.Value(),
					Save: true,
				}
			}
		default:
			return func() tea.Msg { return cloudCredentialResolvedMsg{Cancel: true} }
		}
	default:
		var cmd tea.Cmd
		p.KeyInput, cmd = p.KeyInput.Update(msg)
		m.cloudCredentialPrompt = p
		return cmd
	}
	if p.FocusIndex == 0 {
		p.KeyInput.Focus()
	} else {
		p.KeyInput.Blur()
	}
	m.cloudCredentialPrompt = p
	return nil
}

func (m *Model) renderCloudCredentialModal(prompt *cloudCredentialPromptState) string {
	button := func(label string, focused bool) string {
		if focused {
			return m.styles.SemAccent.Render("[" + label + "]")
		}
		return "[" + label + "]"
	}
	modelName := prompt.Resolved.Model
	if modelName == "" {
		modelName = prompt.Requested
	}
	errLine := ""
	if strings.TrimSpace(prompt.Error) != "" {
		errLine = "\n" + m.styles.StatusError.Render(prompt.Error) + "\n"
	}
	modalContent := fmt.Sprintf(`%s

Model %s is available through Ollama Cloud, not your local Ollama server.
Using it may send prompts, file context, tool output, memory snippets, and project metadata to Ollama Cloud.

%s
%s
%s  %s  %s`,
		m.styles.ModalTitle.Render("Ollama Cloud API key required"),
		modelName,
		prompt.KeyInput.View(),
		errLine,
		button("Use once", prompt.FocusIndex == 1),
		button("Save to keychain", prompt.FocusIndex == 2),
		button("Cancel", prompt.FocusIndex == 3),
	)
	return m.styles.Modal.Render(modalContent)
}

// shortID returns up to n runes from id for compact display in the TUI.
func shortID(id string, n int) string {
	if n <= 0 {
		return ""
	}
	r := []rune(id)
	if len(r) <= n {
		return id
	}
	return string(r[:n])
}

const maxToolDisplayBytes = 4096

func truncateForDisplay(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}

	used := 0
	var b strings.Builder
	for _, r := range s {
		size := utf8.RuneLen(r)
		if used+size > max {
			break
		}
		b.WriteRune(r)
		used += size
	}
	remaining := len(s) - used
	return fmt.Sprintf("%s… (truncated, %d more bytes)", b.String(), remaining)
}

// Init initializes the model.
func (m *Model) Init() tea.Cmd {
	// Wire the broker's send function to the program
	m.broker = NewPermissionBroker(func(msg tea.Msg) {
		if m.program != nil {
			m.program.Send(msg)
		}
	})

	// Focus input for typing
	m.input.Focus()
	if len(m.startupNotes) > 0 {
		for _, note := range m.startupNotes {
			note = strings.TrimSpace(note)
			if note == "" {
				continue
			}
			m.transcript = append(m.transcript, CreateSystemItem(note))
		}
		m.refreshViewportContent(true)
	}
	if m.skillLoader != nil {
		m.skillLoader.OnChange(func(name string, src skills.Source) {
			if m.program != nil {
				m.program.Send(skillChangedMsg{Name: name, Source: src.String()})
			}
		})
	}

	// Start a ticker for periodic updates
	return m.refreshFileIndexCmd("startup")
}

// Update handles messages and model updates.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	consumeWidgets := false
	m.syncBindingContexts()

	switch msg := msg.(type) {
	case tea.KeyMsg:
		var cmd tea.Cmd
		cmd, consumeWidgets = m.handleKeyMsg(msg)
		cmds = append(cmds, cmd)

	case tea.WindowSizeMsg:
		cmds = append(cmds, m.handleWindowSize(msg))

	case agentEventMsg:
		m.handleAgentEvent(msg.Event)

	case agentDoneMsg:
		if m.cancelRun != nil {
			m.cancelRun()
			m.cancelRun = nil
		}
		if m.activeRunCtx != nil {
			m.activeRunCtx = nil
		}
		m.compactCh = nil
		m.thinkingActive = false
		if m.activeRunKind == "main" {
			m.backgroundedRun = false
		}
		m.transcript = FinalizeThinkingItem(m.transcript)
		m.closePicker()
		cmds = append(cmds, m.refreshFileIndexCmd("post-run"))
		if strings.TrimSpace(m.pendingBTW) != "" && m.activeRunKind == "main" {
			question := strings.TrimSpace(m.pendingBTW)
			m.pendingBTW = ""
			m.store.Set(func(app state.App) state.App {
				app.ActiveRun = true
				return app
			})
			cmds = append(cmds, m.startBTWPrompt(question))
			break
		}
		appState := m.store.Get()
		if len(appState.QueuedPrompts) > 0 {
			m.store.Set(func(app state.App) state.App {
				app.QueuedPrompts = append([]string(nil), app.QueuedPrompts[1:]...)
				app.ActiveRun = true
				return app
			})
			cmds = append(cmds, m.startQueuedPrompt())
		} else {
			m.store.Set(func(app state.App) state.App {
				app.ActiveRun = false
				return app
			})
			if m.spawnDream != nil {
				m.spawnDream()
			}
		}

	case permissionPromptMsg:
		m.closePicker()
		m.store.Set(func(app state.App) state.App {
			app.PermissionPrompt = &state.PermissionPrompt{
				ID:       msg.Request.ID,
				ToolName: msg.Request.ToolName,
				Target:   msg.Request.Target,
				Reason:   msg.Request.Reason,
			}
			return app
		})

	case permissionCancelledMsg:
		m.store.Set(func(app state.App) state.App {
			if app.PermissionPrompt != nil && app.PermissionPrompt.ID == msg.ID {
				app.PermissionPrompt = nil
			}
			return app
		})

	case permissionResolvedMsg:
		m.broker.Resolve(msg.ID, permissionDecision(msg.Decision))
		m.store.Set(func(app state.App) state.App {
			app.PermissionPrompt = nil
			return app
		})

	case skillChangedMsg:
		m.transcript = append(m.transcript, CreateSystemItem(fmt.Sprintf("Skill reloaded: %s (%s)", msg.Name, msg.Source)))
		m.refreshViewportContent(true)

	case fileIndexRefreshedMsg:
		if msg.Err != nil && msg.Source == "manual" {
			m.transcript = append(m.transcript, CreateSystemItem("[Index refresh failed] "+msg.Err.Error()))
			m.refreshViewportContent(true)
		}
		if msg.Err == nil && msg.Source == "manual" {
			status := fmt.Sprintf("[Index refreshed: %d entries]", msg.Count)
			if msg.Truncated {
				status = fmt.Sprintf("[Index refreshed: %d entries, truncated]", msg.Count)
			}
			m.transcript = append(m.transcript, CreateSystemItem(status))
			m.refreshViewportContent(true)
		}

	case indexOpDoneMsg:
		m.clearIndexProgress()
		if msg.Err != nil {
			m.transcript = append(m.transcript, CreateSystemItem("[Index error] "+msg.Err.Error()))
		} else if strings.TrimSpace(msg.Content) != "" {
			m.transcript = append(m.transcript, CreateSystemItem(msg.Content))
		}
		m.refreshViewportContent(true)

	case indexProgressMsg:
		m.updateIndexProgress(msg.Event)
		m.refreshViewportContent(true)

	case memoryEditDoneMsg:
		if msg.Err != nil {
			m.transcript = append(m.transcript, CreateSystemItem("[Error: editor failed: "+msg.Err.Error()+"]"))
		} else {
			m.transcript = append(m.transcript, CreateSystemItem("[Edited memory file: "+filepath.Base(msg.Path)+"]"))
		}
		m.refreshViewportContent(true)

	case modelSwitchNeedsCredentialMsg:
		m.closePicker()
		prompt := newCloudCredentialPrompt(msg)
		m.cloudCredentialPrompt = &prompt
		m.refreshViewportContent(true)

	case cloudCredentialResolvedMsg:
		if m.cloudCredentialPrompt == nil {
			break
		}
		prompt := m.cloudCredentialPrompt
		if msg.Cancel {
			if prompt.ForPromptSubmission {
				m.input.SetValue(prompt.DisplayInput)
				m.input.Focus()
				m.transcript = append(m.transcript, CreateSystemItem("[Model switch canceled: cloud credential required before sending prompt]"))
				m.refreshViewportContent(true)
			}
			m.cloudCredentialPrompt = nil
			break
		}
		if strings.TrimSpace(msg.Key) == "" {
			prompt.Error = "API key is required"
			prompt.KeyInput.Focus()
			m.cloudCredentialPrompt = prompt
			m.refreshViewportContent(true)
			break
		}
		m.cloudCredentialPrompt = nil
		cmds = append(cmds, m.startModelSwitchCmd(modelSwitchRequest{
			Requested:           prompt.Requested,
			ForPromptSubmission: prompt.ForPromptSubmission,
			Input:               prompt.Input,
			DisplayInput:        prompt.DisplayInput,
			PreExpanded:         prompt.PreExpanded,
			PromptResult: &promptResult{
				Key:  msg.Key,
				Save: msg.Save,
			},
		}))

	case modelSwitchCompletedMsg:
		m.applyModelSwitchResult(msg.Result)
		if msg.ForPromptSubmission {
			cmd := m.submitPrompt(msg.Input, msg.DisplayInput, msg.PreExpanded)
			if cmd != nil {
				cmds = append(cmds, cmd)
			}
			break
		}
		if msg.Result.Resolved.AliasUsed {
			m.transcript = append(m.transcript, CreateSystemItem(
				"[Model "+msg.Result.Resolved.RequestedName+" is local-cloud naming. Using Ollama Cloud API model: "+msg.Result.Resolved.Model+"]",
			))
		}
		m.transcript = append(m.transcript, CreateSystemItem(msg.Result.Message))
		m.refreshViewportContent(true)

	case modelSwitchFailedMsg:
		if msg.ForPromptSubmission {
			if errors.Is(msg.Err, modelruntime.ErrCredentialCanceled) {
				m.input.SetValue(msg.DisplayInput)
				m.input.Focus()
				m.transcript = append(m.transcript, CreateSystemItem("[Model switch canceled: cloud credential required before sending prompt]"))
				m.refreshViewportContent(true)
				break
			}
			m.input.SetValue(msg.DisplayInput)
			m.input.Focus()
		}
		switch {
		case errors.Is(msg.Err, modelruntime.ErrCredentialRequired):
			m.transcript = append(m.transcript, CreateSystemItem("[Error: Ollama Cloud model requires API key]"))
		case errors.Is(msg.Err, modelruntime.ErrUnauthorized):
			m.transcript = append(m.transcript, CreateSystemItem("[Error: invalid Ollama Cloud API key]"))
		default:
			m.transcript = append(m.transcript, CreateSystemItem("[Error: "+msg.Err.Error()+"]"))
		}
		m.refreshViewportContent(true)

	case tickMsg:
		m.refreshViewportContent(false)
		if m.shouldRunTick(time.Now()) {
			cmds = append(cmds, m.nextTickCmd())
		}
	}

	// Update input and viewport
	if !consumeWidgets {
		var inputCmd tea.Cmd
		m.input, inputCmd = m.input.Update(msg)
		cmds = append(cmds, inputCmd)

		var vpCmd tea.Cmd
		m.viewport, vpCmd = m.viewport.Update(msg)
		cmds = append(cmds, vpCmd)
	}

	switch msg.(type) {
	case tea.KeyMsg:
		m.refreshPicker()
	case tea.WindowSizeMsg:
		m.refreshPicker()
	case fileIndexRefreshedMsg:
		m.refreshPicker()
	}
	m.syncBindingContexts()

	return m, tea.Batch(cmds...)
}

// View renders the model.
func (m *Model) View() string {
	appState := m.store.Get()

	// Keep transcript viewport content in sync while avoiding redundant resets.
	m.viewport.Height = m.computeViewportHeight()
	m.ensureViewportContent()
	vpView := m.viewport.View()

	// Render input area
	inputView := m.renderInput()

	// Render status bar
	statusView := m.renderStatusBar(appState)
	activityView := m.renderActiveTaskLine(appState)
	tipView := m.renderTipLine(appState)

	// Render modal if needed
	modalView := ""
	if m.cloudCredentialPrompt != nil {
		modalView = m.renderCloudCredentialModal(m.cloudCredentialPrompt)
	} else if appState.PermissionPrompt != nil {
		modalView = m.renderPermissionModal(appState.PermissionPrompt)
	}

	// Combine views
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		vpView,
		inputView,
		activityView,
		tipView,
		statusView,
	)

	if modalView != "" {
		// Overlay modal on top
		return m.overlayModal(content, modalView)
	}

	return content
}

// handleKeyMsg handles keyboard input.
func (m *Model) handleKeyMsg(msg tea.KeyMsg) (tea.Cmd, bool) {
	if m.inputPreprocessor != nil {
		var consume bool
		msg, consume = m.inputPreprocessor.Process(msg)
		if consume {
			return nil, true
		}
	}

	// Cloud credential modal is open, handle modal keys before generic paste
	// routing so pasted API keys land in the password field.
	if m.cloudCredentialPrompt != nil {
		return m.handleCloudCredentialKeyMsg(msg), true
	}

	// Bracketed paste should bypass normal-mode command handling and route
	// directly into the input buffer.
	if msg.Paste {
		if !m.vim.IsInsert() {
			m.vim.EnterInsert()
			m.input.Focus()
		}
		return nil, false
	}

	// Handle Ctrl-C
	if msg.Type == tea.KeyCtrlC {
		if m.activeRunCtx != nil && m.cancelRun != nil {
			m.cancelRun()
			return nil, true
		}
		// No active run, exit
		return tea.Quit, true
	}

	// Handle Ctrl-D (exit)
	if msg.Type == tea.KeyCtrlD {
		if m.store.Get().PermissionPrompt == nil && m.cloudCredentialPrompt == nil {
			return tea.Quit, true
		}
	}

	// Permission modal is open, handle modal keys
	if m.store.Get().PermissionPrompt != nil {
		return m.handlePermissionKeyMsg(msg), true
	}

	// Handle Vim mode
	if m.vim.IsNormal() {
		if m.chordInterceptor != nil && m.chordInterceptor.Expired(time.Now()) {
			m.chordInterceptor.Reset()
		}
		switch msg.String() {
		case "ctrl+t":
			m.toggleLastThinkingItem()
			return nil, true
		case "g":
			now := time.Now()
			if action, ok := m.chordInterceptor.Match("g", now); ok && action == "goto_top" {
				m.viewport.GotoTop()
				return nil, true
			}
			m.chordInterceptor.Start("g", now)
			return nil, true
		case "G":
			now := time.Now()
			if action, ok := m.chordInterceptor.Match("G", now); ok && action == "goto_bottom" {
				m.viewport.GotoBottom()
				return nil, true
			}
			m.viewport.GotoBottom()
			m.chordInterceptor.Reset()
			return nil, true
		case "n":
			if len(m.searchMatches) > 0 {
				m.searchPos = (m.searchPos + 1) % len(m.searchMatches)
				m.scrollToTranscriptIndex(m.searchMatches[m.searchPos])
				return nil, true
			}
		case "N":
			if len(m.searchMatches) > 0 {
				m.searchPos = (m.searchPos - 1 + len(m.searchMatches)) % len(m.searchMatches)
				m.scrollToTranscriptIndex(m.searchMatches[m.searchPos])
				return nil, true
			}
		case "i":
			m.vim.EnterInsert()
			m.input.Focus()
			return nil, true
		case "a":
			m.vim.EnterInsert()
			m.input.Focus()
			return nil, true
		case "q":
			if m.store.Get().ActiveRun {
				return nil, true // Don't allow quitting during active run
			}
			return tea.Quit, true
		case "esc":
			m.vim.EnterNormal()
			m.input.Blur()
			m.closePicker()
			m.lastGChordAt = time.Time{}
			m.chordInterceptor.Reset()
			return nil, true
		}
		return nil, false
	}

	if m.vim.IsInsert() {
		if m.picker.Visible {
			switch msg.Type {
			case tea.KeyEsc:
				m.closePicker()
				return nil, true
			case tea.KeyUp, tea.KeyCtrlP:
				m.movePickerSelection(-1)
				return nil, true
			case tea.KeyDown, tea.KeyCtrlN:
				m.movePickerSelection(1)
				return nil, true
			case tea.KeyTab:
				if len(m.picker.Items) > 0 {
					m.acceptPickerSelection(false)
				}
				return nil, true
			case tea.KeyShiftTab:
				if len(m.picker.Items) > 0 {
					m.acceptPickerSelection(true)
				}
				return nil, true
			case tea.KeyRight:
				if len(m.picker.Items) > 0 && m.isCursorAtLineEnd() {
					m.acceptPickerSelection(false)
					return nil, true
				}
			case tea.KeyEnter:
				if len(m.picker.Items) > 0 {
					m.acceptPickerSelection(false)
					return nil, true
				}
			}
		}

		if msg.Type == tea.KeyCtrlT {
			m.toggleLastThinkingItem()
			return nil, true
		}

		switch msg.Type {
		case tea.KeyEsc:
			m.vim.EnterNormal()
			m.input.Blur()
			m.closePicker()
			return nil, true

		case tea.KeyEnter:
			input := strings.TrimSpace(m.input.Value())
			displayInput := input
			preExpanded := false
			m.input.Reset()
			m.closePicker()

			if input == "" {
				return nil, true
			}
			m.everHadInput = true

			// Check for slash command
			if strings.HasPrefix(input, "/") {
				command, args := ParseSlashCommand(input)
				if command == "memory" && len(args) > 0 && strings.EqualFold(strings.TrimSpace(args[0]), "edit") {
					return m.handleMemoryEdit(args), true
				}

				if command == "refresh-index" {
					return m.refreshFileIndexCmd("manual"), true
				}

				if command == "compact" {
					appState := m.store.Get()
					if appState.ActiveRun && m.compactCh != nil {
						select {
						case m.compactCh <- struct{}{}:
							m.transcript = append(m.transcript, CreateSystemItem("[Compact requested...]"))
						default:
							m.transcript = append(m.transcript, CreateSystemItem("[Compact already pending]"))
						}
					} else {
						msgs := appState.Messages
						cfg := agent.DefaultCompactionConfig()
						turns := agent.CountTurnsExported(msgs)
						if turns < cfg.MinTurns {
							m.transcript = append(m.transcript, CreateSystemItem(
								fmt.Sprintf("[Compact skipped: not enough turns to compact (need at least %d, have %d)]",
									cfg.MinTurns, turns)))
						} else {
							truncated := agent.EmergencyTruncateExported(msgs, cfg.MinTurns)
							m.store.Set(func(app state.App) state.App {
								app.Messages = truncated
								return app
							})
							m.transcript = append(m.transcript, CreateSystemItem(
								fmt.Sprintf("[Compacted: %d → %d messages (emergency truncate)]",
									len(msgs), len(truncated))))
						}
					}
					m.refreshViewportContent(true)
					return nil, true
				}
				if command == "analyze-project" {
					path := "."
					question := ""
					if len(args) > 0 {
						path = strings.TrimSpace(args[0])
					}
					if len(args) > 1 {
						question = strings.TrimSpace(strings.Join(args[1:], " "))
					}
					if question == "" {
						question = "Analyze this project and provide a detailed implementation and risk summary."
					}
					prompt, report := m.buildAnalyzeProjectInput(path, question)
					input = prompt
					preExpanded = true
					for _, note := range report.StageNotes {
						m.transcript = append(m.transcript, CreateSystemItem("[Analysis stage] "+note))
					}
					m.transcript = append(m.transcript, CreateSystemItem(
						fmt.Sprintf("[Analysis workflow: files=%d chunks=%d cache hit/miss=%d/%d]",
							report.SelectedFiles, report.ChunkCount, report.CacheHits, report.CacheMisses),
					))
					m.transcript = append(m.transcript, CreateSystemItem(
						fmt.Sprintf("[Analysis evidence ledger run=%s]", report.LedgerRunID),
					))
					goto submitPrompt
				}
				if command == "search" {
					m.handleTranscriptSearch(args)
					return nil, true
				}
				if command == "bg" {
					m.handleBackgroundCommand(args)
					return nil, true
				}
				if command == "btw" {
					return m.handleBTWCommand(args), true
				}
				if command == "semantic" {
					return m.handleSemanticCommand(args), true
				}
				if command == "index" {
					return m.handleIndexCommand(args), true
				}
				if command == "model" && m.cmdCtx.ModelRuntime != nil {
					if len(args) == 0 {
						out := m.cmdRegistry.Dispatch(context.Background(), command, args, m.cmdCtx)
						m.transcript = append(m.transcript, TranscriptItem{Kind: TranscriptSystem, Content: out.Content})
						m.refreshViewportContent(true)
						return nil, true
					}
					if m.store.Get().ActiveRun {
						m.transcript = append(m.transcript, CreateSystemItem("[Error: cannot switch models while a run is active]"))
						m.refreshViewportContent(true)
						return nil, true
					}
					requested := strings.TrimSpace(strings.Join(args, " "))
					if requested == "" {
						m.transcript = append(m.transcript, CreateSystemItem("[Error: /model requires a model name. Usage: /model <name>]"))
						m.refreshViewportContent(true)
						return nil, true
					}
					return m.startModelSwitchCmd(modelSwitchRequest{Requested: requested}), true
				}

				out := m.cmdRegistry.Dispatch(context.Background(), command, args, m.cmdCtx)

				// Add system item to transcript
				item := TranscriptItem{Kind: TranscriptSystem, Content: out.Content}
				if out.Kind == commands.OutputAssistant {
					item.Kind = TranscriptAssistant
				}
				m.transcript = append(m.transcript, item)
				m.refreshViewportContent(true)

				if out.Clear {
					m.clearTranscript()
					m.refreshViewportContent(true)
					m.store.Set(func(app state.App) state.App {
						app.Messages = []llm.Message{}
						app.LastRetryNotice = ""
						app.TerminalReason = ""
						app.TerminalDetail = ""
						app.ActiveTools = map[string]state.ToolUse{}
						return app
					})
				}

				if out.Quit {
					m.broker.CancelAll()
					return tea.Quit, true
				}

				return nil, true
			}

		submitPrompt:
			cmd := m.submitPrompt(input, displayInput, preExpanded)
			return cmd, true
		}
	}

	return nil, false
}

// handleWindowSize handles terminal resize events.
func (m *Model) handleWindowSize(msg tea.WindowSizeMsg) tea.Cmd {
	m.width = msg.Width
	m.height = msg.Height

	// Resize viewport
	m.viewport.Width = msg.Width
	m.viewport.Height = msg.Height - 8 // Reserve space for input and status

	// Resize input
	m.input.SetWidth(msg.Width - 2)
	m.input.SetHeight(3)

	// Resize renderer
	if m.renderer != nil {
		newWidth := msg.Width - 4
		m.renderer.Resize(newWidth)
		if newWidth != m.markdownRenderWidth {
			m.markdownRenderWidth = newWidth
			m.invalidateAssistantMarkdownCache()
			m.invalidateTranscriptRenderCache()
		}
	}

	return nil
}

// handleAgentEvent processes an agent event.
func (m *Model) handleAgentEvent(evt agent.Event) {
	refreshNow := true

	defer func() {
		if r := recover(); r != nil {
			m.transcript = append(m.transcript, CreateSystemItem(fmt.Sprintf("[render error: %v]", r)))
			// Keep session alive even if refresh/render panics.
			defer func() { _ = recover() }()
			m.refreshViewportContent(true)
		}
	}()

	switch e := evt.(type) {
	case agent.AssistantTurnStarted:
		m.transcript = append(m.transcript, CreateSystemItem(fmt.Sprintf("[Turn %d]", e.Turn)))
		if m.heightCache != nil {
			last := len(m.transcript) - 1
			m.heightCache[last] = estimateTranscriptItemLines(m.transcript[last])
		}

	case agent.AssistantTextDelta:
		m.markFirstStreamEvent()
		m.transcript = AppendAssistantDelta(m.transcript, e.Content)
		if m.heightCache != nil && len(m.transcript) > 0 {
			last := len(m.transcript) - 1
			m.heightCache[last] = estimateTranscriptItemLines(m.transcript[last])
		}
		refreshNow = m.shouldRefreshStreamingEvent()

	case agent.AssistantThinkingDelta:
		m.markFirstStreamEvent()
		m.thinkingActive = true
		m.transcript = AppendThinkingDelta(m.transcript, e.Thinking)
		if m.heightCache != nil && len(m.transcript) > 0 {
			last := len(m.transcript) - 1
			m.heightCache[last] = estimateTranscriptItemLines(m.transcript[last])
		}
		refreshNow = m.shouldRefreshStreamingEvent()

	case agent.ToolUseStart:
		m.markFirstStreamEvent()
		toolItem := CreateToolItem(e.ID, e.Name)
		m.transcript = append(m.transcript, toolItem)
		m.toolIndex[e.ID] = len(m.transcript) - 1
		if m.heightCache != nil {
			m.heightCache[len(m.transcript)-1] = estimateTranscriptItemLines(toolItem)
		}
		m.store.Set(func(app state.App) state.App {
			app.ActiveTools[e.ID] = state.ToolUse{
				ID:        e.ID,
				Name:      e.Name,
				Summary:   "[started]",
				StartedAt: time.Now(),
				Done:      false,
				Error:     "",
			}
			return app
		})

	case agent.ToolUseProgress:
		// Update transcript tool item
		// Data is typically a stream or message string
		dataStr := truncateForDisplay(fmt.Sprintf("%v", e.Data), maxToolDisplayBytes)
		m.updateToolItem(e.ID, func(item *TranscriptItem) {
			item.Content = fmt.Sprintf("[%s] %s", shortID(e.ID, 8), dataStr)
		})
		m.store.Set(func(app state.App) state.App {
			if tool, ok := app.ActiveTools[e.ID]; ok {
				tool.Summary = dataStr
				app.ActiveTools[e.ID] = tool
			}
			return app
		})
		refreshNow = m.shouldRefreshStreamingEvent()

	case agent.ToolUseResult:
		resultStr := ""
		if e.Result.Data != nil || e.Result.Display != "" {
			resultStr = e.Result.Display
			if resultStr == "" && e.Result.Data != nil {
				resultStr = fmt.Sprintf("%v", e.Result.Data)
			}
		}
		resultStr = truncateForDisplay(resultStr, maxToolDisplayBytes)
		if e.Err != nil {
			errStr := truncateForDisplay(e.Err.Error(), maxToolDisplayBytes)
			m.updateToolItem(e.ID, func(item *TranscriptItem) {
				item.Content = fmt.Sprintf("[ERROR] %s", errStr)
				item.Error = errStr
			})
		} else {
			m.updateToolItem(e.ID, func(item *TranscriptItem) {
				item.Content = fmt.Sprintf("[OK] %s", resultStr)
			})
		}
		m.store.Set(func(app state.App) state.App {
			if tool, ok := app.ActiveTools[e.ID]; ok {
				tool.Done = true
				if e.Err != nil {
					tool.Error = truncateForDisplay(e.Err.Error(), maxToolDisplayBytes)
					tool.Summary = tool.Error
				} else {
					tool.Summary = resultStr
				}
				app.ActiveTools[e.ID] = tool
			}
			return app
		})

	case agent.CompactionStarted:
		m.compactingActive = true
		m.transcript = append(m.transcript, CreateSystemItem("[Compacting context...]"))
		if m.heightCache != nil {
			last := len(m.transcript) - 1
			m.heightCache[last] = estimateTranscriptItemLines(m.transcript[last])
		}

	case agent.CompactionCompleted:
		m.compactingActive = false
		if e.Result.Skipped {
			m.transcript = append(m.transcript, CreateSystemItem(
				fmt.Sprintf("[Compact skipped: not enough turns to compact (%d messages)]", e.Result.Before)))
		} else if e.Result.Error != "" {
			m.transcript = append(m.transcript, CreateSystemItem("[Compaction failed — continuing without compaction]"))
		} else {
			m.transcript = append(m.transcript, CreateSystemItem(
				fmt.Sprintf("[Compacted: %d → %d messages]", e.Result.Before, e.Result.After)))
		}

	case agent.RetryNotice:
		item := CreateSystemItem(fmt.Sprintf("[Retry %d] %s", e.Attempt, e.Cause))
		m.transcript = append(m.transcript, item)
		if m.heightCache != nil {
			m.heightCache[len(m.transcript)-1] = estimateTranscriptItemLines(item)
		}
		m.retryActiveUntil = time.Now().Add(3 * time.Second)
		m.store.Set(func(app state.App) state.App {
			app.LastRetryNotice = fmt.Sprintf("Retry %d: %s", e.Attempt, e.Cause)
			return app
		})

	case agent.HookNotice:
		if e.Message != "" {
			m.transcript = append(m.transcript, CreateSystemItem(e.Message))
			if m.heightCache != nil {
				last := len(m.transcript) - 1
				m.heightCache[last] = estimateTranscriptItemLines(m.transcript[last])
			}
		}

	case agent.LLMIdleWarning:
		provider := strings.TrimSpace(e.Provider)
		msg := "[Still waiting for model stream"
		if provider != "" {
			msg += " (" + provider + ")"
		}
		msg += fmt.Sprintf("; idle %s]", e.Timeout)
		m.transcript = append(m.transcript, CreateSystemItem(msg))
		if m.heightCache != nil {
			last := len(m.transcript) - 1
			m.heightCache[last] = estimateTranscriptItemLines(m.transcript[last])
		}
	case agent.LLMRequestStarted:
		// The server emits this as an event. In the TUI slow-stage summary, only
		// elapsed work should be recorded, so request start itself is intentionally
		// not reported as a duration.
	case agent.LLMStreamOpened:
		if e.Latency > 0 {
			if m.meter != nil {
				m.meter.NotePendingRunStage("llm_request_open", e.Latency)
			}
			m.appendSlowStageNotice("llm_request_open", e.Latency)
		}
	case agent.FirstTokenReceived:
		if e.Latency > 0 {
			if m.meter != nil {
				m.meter.NotePendingRunStage("first_token", e.Latency)
			}
			m.appendSlowStageNotice("first_token", e.Latency)
		}

	case agent.StageTiming:
		m.appendSlowStageNotice(e.Stage, e.Duration)

	case agent.PromptPackReport:
		if e.SkippedMessages > 0 {
			msg := fmt.Sprintf(
				"[Prompt packed: kept %d/%d messages, skipped ~%d tokens, dropped %d mention blocks, budget=%d]",
				e.IncludedMessages,
				e.IncludedMessages+e.SkippedMessages,
				e.EstimatedSkipped,
				e.DroppedMentionBlocks,
				e.InputBudgetTokens,
			)
			if e.ForcedIncludeLast {
				msg += " [forced last message]"
			}
			m.transcript = append(m.transcript, CreateSystemItem(msg))
		}
	case agent.Terminal:
		m.recordFirstVisibleRenderIfNeeded()
		m.thinkingActive = false
		m.transcript = FinalizeThinkingItem(m.transcript)
		m.appendStageSummaryFromTrace()
		if e.Reason != agent.TerminalCompleted {
			detail := strings.TrimSpace(e.Detail)
			message := fmt.Sprintf("[Run ended: %s]", e.Reason)
			if detail != "" {
				message = fmt.Sprintf("[Run ended: %s: %s]", e.Reason, detail)
			}
			m.transcript = append(m.transcript, CreateSystemItem(message))
		}
		m.store.Set(func(app state.App) state.App {
			app.ActiveRun = false
			app.TerminalReason = e.Reason
			app.TerminalDetail = e.Detail
			app.Usage = e.Usage
			if len(e.Conversation) > 0 && m.activeRunKind != "btw" {
				app.Messages = append(app.Messages, e.Conversation...)
			}
			return app
		})
		m.compactingActive = false
		m.retryActiveUntil = time.Time{}
		m.saveRunCheckpoint(e)
		m.activeRunKind = "main"
	}

	if refreshNow {
		m.refreshViewportContent(true)
		m.recordFirstVisibleRenderIfNeeded()
	}
}

func (m *Model) retrieveAnalysisMentions(path, question string, limit int) []string {
	if m == nil || m.fileIndex == nil {
		return nil
	}
	entries := m.fileIndex.Snapshot()
	if len(entries) == 0 {
		return nil
	}
	return analysis.RetrieveTopFiles(entries, question, path, m.frecency, limit)
}

func (m *Model) buildAnalyzeProjectInput(path, question string) (string, analysis.ProjectWorkflowReport) {
	workingDir := m.store.Get().ToolSettings.WorkingDir
	retrieved := m.retrieveAnalysisMentions(path, question, 12)
	prompt, report, err := analysis.BuildProjectAnalysisPrompt(analysis.ProjectWorkflowOptions{
		RootDir:   workingDir,
		ScopePath: path,
		Question:  question,
		Retrieved: retrieved,
		MaxFiles:  12,
	})
	if err != nil {
		fallback := fmt.Sprintf("%s @%s", question, path)
		return fallback, analysis.ProjectWorkflowReport{ScopePath: path}
	}
	return prompt, report
}

func (m *Model) saveRunCheckpoint(term agent.Terminal) {
	lastUser := ""
	lastAssistant := ""
	app := m.store.Get()
	for i := len(app.Messages) - 1; i >= 0; i-- {
		msg := app.Messages[i]
		if lastUser == "" && msg.Role == llm.RoleUser && strings.TrimSpace(msg.Content) != "" {
			lastUser = msg.Content
		}
		if lastAssistant == "" && msg.Role == llm.RoleAssistant && strings.TrimSpace(msg.Content) != "" {
			lastAssistant = msg.Content
		}
		if lastUser != "" && lastAssistant != "" {
			break
		}
	}
	if lastAssistant == "" {
		for i := len(m.transcript) - 1; i >= 0; i-- {
			if m.transcript[i].Kind == TranscriptAssistant && strings.TrimSpace(m.transcript[i].Content) != "" {
				lastAssistant = m.transcript[i].Content
				break
			}
		}
	}
	pending := term.Reason != agent.TerminalCompleted || analysis.LooksLikeIncompleteFinalAnswer(lastAssistant)
	inspected := analysis.ExtractMentionedPaths(lastUser)
	_ = analysis.SaveCheckpoint(analysis.Checkpoint{
		Model:               app.ActiveModel,
		WorkingDir:          app.ToolSettings.WorkingDir,
		LastUserPrompt:      lastUser,
		LastAssistantOutput: lastAssistant,
		TerminalReason:      string(term.Reason),
		TerminalDetail:      term.Detail,
		PendingFinalAnswer:  pending,
		InspectedFiles:      inspected,
		SynthesisStage:      synthesisStage(term, pending),
	})
}

func synthesisStage(term agent.Terminal, pending bool) string {
	if pending {
		return "synthesis_pending"
	}
	if term.Reason == agent.TerminalCompleted {
		return "completed"
	}
	return "interrupted"
}

func (m *Model) applyModelSwitchResult(result modelruntime.SwitchResult) {
	m.resolvedActiveModel = result.Resolved.Model
	baseURL := strings.TrimSpace(result.Resolved.BaseURL)
	if baseURL == "" && m.cmdCtx.ModelRuntime != nil {
		baseURL = m.cmdCtx.ModelRuntime.LocalBaseURL
	}
	m.store.Set(func(app state.App) state.App {
		app.ActiveModel = result.Resolved.Model
		app.LLMProvider = string(result.Resolved.Provider)
		if baseURL != "" {
			app.LLMBaseURL = baseURL
		}
		return app
	})
	if m.cmdCtx.LLMClient != nil {
		if details, err := m.cmdCtx.LLMClient.ShowModel(context.Background(), result.Resolved.Model); err == nil {
			limits := llm.ComputeLimits(details)
			m.store.Set(func(app state.App) state.App {
				app.MaxOutputTokens = limits.MaxOutputTokens
				if limits.NumCtx > 0 {
					app.RuntimeNumCtx = limits.NumCtx
				}
				app.ToolSettings.MaxResultChars = limits.MaxResultChars
				return app
			})
		}
	}
}

func (m *Model) submitPrompt(input, displayInput string, preExpanded bool) tea.Cmd {
	if strings.EqualFold(strings.TrimSpace(input), "continue") {
		if ckpt, err := analysis.LoadPendingCheckpoint(24 * time.Hour); err == nil {
			input = analysis.BuildResumePrompt(ckpt, input)
			m.transcript = append(m.transcript, CreateSystemItem("[Resuming from analysis checkpoint]"))
		} else if err == analysis.ErrCheckpointStale {
			_ = analysis.DeleteCheckpoint()
			m.transcript = append(m.transcript, CreateSystemItem("[Checkpoint was stale and has been cleared]"))
		}
	}

	appState := m.store.Get()
	if m.cmdCtx.ModelRuntime != nil && m.resolvedActiveModel != appState.ActiveModel {
		return m.startModelSwitchCmd(modelSwitchRequest{
			Requested:           appState.ActiveModel,
			ForPromptSubmission: true,
			Input:               input,
			DisplayInput:        displayInput,
			PreExpanded:         preExpanded,
		})
	}

	expandedInput := input
	var expandedFiles []mentions.ResolvedFile
	var expandedDirs []mentions.ResolvedDirectory
	var expansionReport mentions.ExpansionReport
	var evidencePack *agent.EvidencePackReport
	if !preExpanded {
		mentionStart := time.Now()
		assemblyCfg := agent.DefaultConfig()
		assemblyCfg.ContextMode = appState.ContextMode
		if appState.RuntimeNumCtx > 0 {
			assemblyCfg.NumCtx = appState.RuntimeNumCtx
		}
		if appState.MaxOutputTokens > 0 {
			assemblyCfg.MaxOutputTokens = appState.MaxOutputTokens
		}
		packed, _, err := contextpack.BuildCurrentTurnPrompt(input, appState.ToolContext(context.Background()), assemblyCfg, agent.Input{
			Model:           appState.ActiveModel,
			ContextMode:     appState.ContextMode,
			MaxOutputTokens: appState.MaxOutputTokens,
		}, appState.Messages)
		mentionDuration := time.Since(mentionStart)
		if m.meter != nil {
			m.meter.NotePendingRunStage("mention_expand", mentionDuration)
		}
		m.appendSlowStageNotice("mention_expand", mentionDuration)
		if err != nil {
			var tooLarge contextpack.ErrEvidenceTooLarge
			if errors.As(err, &tooLarge) {
				largest := ""
				if len(tooLarge.Largest) > 0 {
					largest = fmt.Sprintf(" Largest omitted: %s.", tooLarge.Largest[0].Path)
				}
				m.transcript = append(m.transcript, CreateSystemItem(fmt.Sprintf("[Context too large: %d files exceed the current context budget after packing.%s %s.]", tooLarge.ReferencedFiles, largest, tooLarge.SplitHint)))
				m.refreshViewportContent(true)
				return nil
			}
			m.transcript = append(m.transcript, CreateSystemItem("[Mention error] "+err.Error()))
			m.refreshViewportContent(true)
			return nil
		}
		expandedInput = packed.Prompt
		expandedFiles = packed.Files
		expandedDirs = packed.Dirs
		expansionReport = packed.ExpansionReport
		rep := packed.PackReport
		evidencePack = &rep
	}
	explicitPaths := analysis.ExtractMentionedPaths(displayInput)
	currentTurnPaths := make([]string, 0, len(expandedFiles))
	for _, f := range expandedFiles {
		if strings.TrimSpace(f.Path) == "" {
			continue
		}
		currentTurnPaths = append(currentTurnPaths, f.Path)
	}
	currentTurnDirs := make([]string, 0, len(expandedDirs))
	for _, d := range expandedDirs {
		if strings.TrimSpace(d.Path) == "" {
			continue
		}
		currentTurnDirs = append(currentTurnDirs, d.Path)
	}
	routeCfg := semanticRouteConfig(m.semanticConfig, m.semanticService != nil)
	routeDecision := retrievalroute.Decide(retrievalroute.Input{
		RawPrompt:            displayInput,
		ShouldQuery:          true,
		AttachmentPolicy:     string(expansionReport.Intent.AttachmentPolicy),
		CurrentTurnPaths:     currentTurnPaths,
		CurrentTurnDirs:      currentTurnDirs,
		AttachedFileCount:    len(expandedFiles),
		AttachedContextBytes: len(expandedInput),
		IndexKnown:           false,
		HasIndex:             false,
		IndexCompatible:      false,
		SemanticEnabled:      m.semanticService != nil && m.semanticConfig.Enabled,
		SemanticMode:         routeCfg.Mode,
		ForceDeep:            m.semanticDeepNext,
		PromptIntent:         string(expansionReport.Intent.Kind),
	}, routeCfg)
	if m.semanticDeepNext {
		m.semanticDeepNext = false
	}

	if routeDecision.AllowEmbedding && m.semanticService != nil {
		semanticStart := time.Now()
		res, err := m.semanticService.Retrieve(context.Background(), semantic.RetrieveRequest{
			Root:                 appState.ToolSettings.WorkingDir,
			Query:                displayInput,
			ExplicitPaths:        explicitPaths,
			CurrentTurnPaths:     currentTurnPaths,
			Deadline:             routeDecision.Deadline,
			RouteAction:          string(routeDecision.Action),
			RouteReason:          string(routeDecision.Reason),
			RouteProfile:         routeDecision.Profile,
			UseCurrentPathWeight: routeDecision.UseCurrentPathWeight,
			MaxRecords:           routeDecision.MaxRecords,
			MaxFiles:             routeDecision.MaxFiles,
			MaxContextBytes:      routeDecision.MaxContextBytes,
			Observer: func(evt semantic.RetrieveStageEvent) {
				stage := strings.TrimSpace(string(evt.Stage))
				if stage == "" {
					return
				}
				stageName := "semantic_" + stage
				if evt.Stage == semantic.RetrieveStageTotal {
					stageName = "semantic_retrieve"
				}
				if m.meter != nil {
					m.meter.NotePendingRunStage(stageName, evt.Duration)
				}
				m.appendSlowStageNotice(stageName, evt.Duration)
			},
		})
		semanticDuration := time.Since(semanticStart)
		if m.meter != nil {
			m.meter.NotePendingRunStage("semantic_retrieve", semanticDuration)
		}
		m.appendSlowStageNotice("semantic_retrieve", semanticDuration)
		switch {
		case err == nil && res.Used:
			expandedInput = strings.TrimRight(expandedInput, "\n") + "\n\n" + strings.TrimSpace(res.RenderedContext)
			m.transcript = append(m.transcript, CreateSystemItem(
				fmt.Sprintf("[Semantic retrieval: records=%d files=%d stale_dropped=%d context=%d bytes]",
					len(res.Records), len(res.Files), res.StaleDropped, res.ContextBytes),
			))
		case err != nil && semantic.IsFallbackError(err):
			m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval fallback] "+err.Error()))
		case err != nil:
			m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval error] "+err.Error()))
		case res.FallbackReason != "":
			m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval fallback] "+res.FallbackReason))
		}
	}

	userMessage := llm.Message{
		Role:    llm.RoleUser,
		Content: expandedInput,
	}
	updatedMessages := append(append([]llm.Message(nil), appState.Messages...), userMessage)
	historyPolicy := agent.HistoryPolicyDefault
	if expansionReport.Intent.AttachmentPolicy == mentions.AttachListingTreeOnly ||
		expansionReport.Intent.Kind == mentions.IntentFileStatus {
		historyPolicy = agent.HistoryPolicyLatestOnly
	}
	runMessages := updatedMessages
	if historyPolicy == agent.HistoryPolicyLatestOnly {
		runMessages = []llm.Message{userMessage}
	}

	m.transcript = append(m.transcript, CreateUserItem(displayInput))
	if summary := directoryExpansionSummary(expandedFiles, expandedDirs); summary != "" {
		m.transcript = append(m.transcript, CreateSystemItem(summary))
	}
	if evidencePack != nil && evidencePack.Packed {
		m.transcript = append(m.transcript, CreateSystemItem(
			fmt.Sprintf("[Context packed: files=%d raw=%d chars excerpted=%d omitted=%d budget=%d tokens]",
				evidencePack.FilesReferenced,
				evidencePack.RawBytesIncluded,
				evidencePack.FilesExcerpted,
				evidencePack.FilesOmitted,
				evidencePack.BudgetTokens,
			),
		))
	}
	if historyPolicy == agent.HistoryPolicyLatestOnly {
		if expansionReport.Intent.AttachmentPolicy == mentions.AttachListingTreeOnly {
			m.transcript = append(m.transcript, CreateSystemItem("[Listing prompt: tree data attached, file bodies=0, history=latest_only]"))
		} else if expansionReport.Intent.Kind == mentions.IntentFileStatus {
			m.transcript = append(m.transcript, CreateSystemItem("[Status prompt: explicit references detected, history=latest_only]"))
		}
	}
	for _, warn := range expansionReport.Warnings {
		if warn == "listing-intent-with-file-bodies" {
			m.transcript = append(m.transcript, CreateSystemItem("[Warning: listing prompt expanded file bodies; use @path?tree or check listing-intent detection]"))
		}
	}
	if m.meter != nil {
		mode := ""
		if len(expandedDirs) > 0 {
			mode = expandedDirs[0].Mode
		}
		discovered := 0
		for _, d := range expandedDirs {
			discovered += d.DiscoveredFiles
		}
		m.meter.NotePendingRunExpansion(mode, len(expandedDirs), discovered, len(expandedFiles), expansionReport.ListingIntent)
	}
	m.refreshViewportContent(true)

	if appState.ActiveRun {
		m.store.Set(func(app state.App) state.App {
			app.QueuedPrompts = append(app.QueuedPrompts, expandedInput)
			app.Messages = updatedMessages
			return app
		})
		return nil
	}

	if m.killDream != nil {
		m.killDream()
	}
	m.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		app.Messages = updatedMessages
		return app
	})
	m.activeRunKind = "main"

	runCtx, cancel := context.WithCancel(context.Background())
	parentAbort := make(chan struct{})
	go func() {
		<-runCtx.Done()
		close(parentAbort)
	}()
	m.activeRunCtx = runCtx
	m.cancelRun = cancel
	m.compactCh = make(chan struct{}, 1)
	m.runStartedAt = time.Now()
	m.firstStreamAt = time.Time{}
	m.firstRenderSet = false
	m.slowStageNotified = map[string]bool{}

	if m.consumeDream != nil {
		if d := strings.TrimSpace(m.consumeDream()); d != "" {
			runMessages = append(runMessages, llm.Message{
				Role:    llm.RoleSystem,
				Content: "[dream] " + d,
			})
		}
	}
	agentInput := agent.Input{
		Model:            appState.ActiveModel,
		LLMProvider:      appState.LLMProvider,
		Messages:         runMessages,
		ToolContext:      appState.ToolContext(runCtx),
		ContextMode:      appState.ContextMode,
		PromptIntent:     string(expansionReport.Intent.Kind),
		AttachmentPolicy: string(expansionReport.Intent.AttachmentPolicy),
		OriginalUserText: displayInput,
		HistoryPolicy:    historyPolicy,
		ToolMode:         string(routeDecision.ToolMode),
		RouteAction:      string(routeDecision.Action),
		RouteReason:      string(routeDecision.Reason),
		RouteProfile:     routeDecision.RequestProfile,
		EvidencePack:     evidencePack,
		PermissionMode:   appState.PermissionMode,
		PermissionRules:  appState.PermissionRules,
		PermissionPrompt: m.broker.PromptFunc(),
		ParentAbort:      parentAbort,
		CompactRequest:   m.compactCh,
		MaxOutputTokens:  appState.MaxOutputTokens,
	}

	runCmd := startAgentCmd(runCtx, m.agent, agentInput, func(msg tea.Msg) {
		if m.program != nil {
			m.program.Send(msg)
		}
	})
	if m.program != nil && m.shouldRunTick(time.Now()) {
		return tea.Batch(runCmd, m.nextTickCmd())
	}
	return runCmd
}

// handlePermissionKeyMsg handles keys when permission modal is open.
func (m *Model) handlePermissionKeyMsg(msg tea.KeyMsg) tea.Cmd {
	appState := m.store.Get()
	if appState.PermissionPrompt == nil {
		return nil
	}

	switch msg.String() {
	case "a":
		// Allow once
		return func() tea.Msg {
			return permissionResolvedMsg{
				ID:       appState.PermissionPrompt.ID,
				Decision: decisionAllow,
			}
		}

	case "d":
		// Deny
		return func() tea.Msg {
			return permissionResolvedMsg{
				ID:       appState.PermissionPrompt.ID,
				Decision: decisionDeny,
			}
		}
	case "esc":
		return func() tea.Msg {
			return permissionResolvedMsg{
				ID:       appState.PermissionPrompt.ID,
				Decision: decisionDeny,
			}
		}

	case "A":
		// Always allow - add rule and approve
		m.store.Set(func(app state.App) state.App {
			toolName := strings.TrimSpace(app.PermissionPrompt.ToolName)
			target := strings.TrimSpace(app.PermissionPrompt.Target)
			pattern := toolName + "(" + target + ")"
			rule := permissions.Rule{
				Pattern: pattern,
				Source:  permissions.SourceSession,
			}

			app.PermissionRules.AlwaysAllow = append(app.PermissionRules.AlwaysAllow, rule)
			return app
		})

		return func() tea.Msg {
			return permissionResolvedMsg{
				ID:       appState.PermissionPrompt.ID,
				Decision: decisionAlwaysAllow,
			}
		}
	}

	return nil
}

func (m *Model) clearTranscript() {
	m.transcript = []TranscriptItem{}
	m.thinkingActive = false
	clear(m.toolIndex)
	clear(m.heightCache)
	m.err = nil
	m.firstStreamAt = time.Time{}
	m.firstRenderSet = false
	m.slowStageNotified = map[string]bool{}
	m.closePicker()
	m.compactingActive = false
	m.retryActiveUntil = time.Time{}
	m.invalidateTranscriptRenderCache()
}

func (m *Model) toggleLastThinkingItem() {
	for i := len(m.transcript) - 1; i >= 0; i-- {
		if m.transcript[i].Kind == TranscriptThinking {
			m.transcript[i].Collapsed = !m.transcript[i].Collapsed
			m.transcript[i].Rendered = ""
			m.refreshViewportContent(false)
			return
		}
	}
}

func (m *Model) findToolItemIndex(id string) int {
	if id == "" {
		return -1
	}
	if idx, ok := m.toolIndex[id]; ok && idx >= 0 && idx < len(m.transcript) {
		item := m.transcript[idx]
		if item.Kind == TranscriptTool && item.ToolID == id {
			return idx
		}
	}
	for i := len(m.transcript) - 1; i >= 0; i-- {
		if m.transcript[i].Kind == TranscriptTool && m.transcript[i].ToolID == id {
			m.toolIndex[id] = i
			return i
		}
	}
	return -1
}

func (m *Model) updateToolItem(id string, mut func(*TranscriptItem)) bool {
	idx := m.findToolItemIndex(id)
	if idx < 0 {
		return false
	}
	mut(&m.transcript[idx])
	if m.heightCache != nil {
		m.heightCache[idx] = estimateTranscriptItemLines(m.transcript[idx])
	}
	m.toolIndex[id] = idx
	return true
}

func (m *Model) closePicker() {
	m.picker = picker.State{}
}

func (m *Model) movePickerSelection(delta int) {
	if len(m.picker.Items) == 0 {
		m.picker.Index = 0
		return
	}
	m.picker.Index += delta
	if m.picker.Index < 0 {
		m.picker.Index = len(m.picker.Items) - 1
	}
	if m.picker.Index >= len(m.picker.Items) {
		m.picker.Index = 0
	}
}

func (m *Model) refreshPicker() {
	if !m.vim.IsInsert() || m.store.Get().PermissionPrompt != nil {
		m.closePicker()
		return
	}
	line, cursor, ok := m.currentInputLine()
	if !ok {
		m.closePicker()
		return
	}
	ctx := picker.Detect(line, cursor)
	if !ctx.Active {
		m.closePicker()
		return
	}
	provider := m.providers[ctx.Kind]
	if provider == nil {
		m.closePicker()
		return
	}
	items := provider.Suggest(ctx.Query, 8)
	m.picker.Visible = true
	m.picker.Trigger = ctx.Kind
	m.picker.Token = ctx
	m.picker.Items = items
	if len(items) == 0 {
		m.picker.Index = 0
		return
	}
	if m.picker.Index < 0 || m.picker.Index >= len(items) {
		m.picker.Index = 0
	}
}

func (m *Model) acceptPickerSelection(acceptDirectory bool) {
	if !m.picker.Visible || len(m.picker.Items) == 0 {
		return
	}
	sel := m.picker.Items[m.picker.Index]
	token := m.picker.Token
	line, _, ok := m.currentInputLine()
	if !ok {
		m.closePicker()
		return
	}
	runes := []rune(line)
	if token.Start < 0 || token.Start > len(runes) || token.End < token.Start || token.End > len(runes) {
		m.closePicker()
		return
	}

	insert := ""
	keepOpen := false
	switch m.picker.Trigger {
	case picker.TriggerFile:
		insert = "@" + sel.Insert
		if sel.IsDir {
			insert += "/"
			if acceptDirectory {
				insert += " "
			} else {
				keepOpen = true
			}
		} else {
			insert += " "
			if m.frecency != nil {
				m.frecency.Touch(sel.Insert)
			}
		}
	case picker.TriggerCommand:
		insert = "/" + sel.Insert + " "
	default:
		return
	}

	before := string(runes[:token.Start])
	after := string(runes[token.End:])
	newLine := before + insert + after
	newCol := utf8.RuneCountInString(before + insert)
	m.replaceCurrentLine(newLine, newCol)

	if keepOpen {
		m.refreshPicker()
		return
	}
	m.closePicker()
}

func (m *Model) currentInputLine() (string, int, bool) {
	value := m.input.Value()
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		return "", 0, false
	}
	lineIdx := m.input.Line()
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}
	line := lines[lineIdx]
	cursor := m.input.LineInfo().CharOffset
	runes := []rune(line)
	if cursor < 0 {
		cursor = 0
	}
	if cursor > len(runes) {
		cursor = len(runes)
	}
	return line, cursor, true
}

func (m *Model) replaceCurrentLine(newLine string, newCursorCol int) {
	value := m.input.Value()
	lines := strings.Split(value, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	lineIdx := m.input.Line()
	if lineIdx < 0 {
		lineIdx = 0
	}
	if lineIdx >= len(lines) {
		lineIdx = len(lines) - 1
	}
	lines[lineIdx] = newLine
	merged := strings.Join(lines, "\n")
	m.input.SetValue(merged)
	targetLine := lineIdx
	for m.input.Line() > targetLine {
		m.input.CursorUp()
	}
	for m.input.Line() < targetLine {
		m.input.CursorDown()
	}
	if newCursorCol < 0 {
		newCursorCol = 0
	}
	lineRunes := []rune(newLine)
	if newCursorCol > len(lineRunes) {
		newCursorCol = len(lineRunes)
	}
	m.input.SetCursor(newCursorCol)
}

func (m *Model) isCursorAtLineEnd() bool {
	line, cursor, ok := m.currentInputLine()
	if !ok {
		return false
	}
	return cursor >= utf8.RuneCountInString(line)
}

func directoryExpansionSummary(_ []mentions.ResolvedFile, dirs []mentions.ResolvedDirectory) string {
	if len(dirs) == 0 {
		return ""
	}
	totalIncluded := 0
	totalDiscovered := 0
	totalIgnored := 0
	totalBytes := 0
	truncated := 0
	reasons := map[string]struct{}{}
	mode := ""
	for _, d := range dirs {
		totalIncluded += d.IncludedFiles
		totalDiscovered += d.DiscoveredFiles
		totalIgnored += d.IgnoredByGit
		totalBytes += d.TotalBytes
		if d.Truncated {
			truncated++
			if strings.TrimSpace(d.Reason) != "" {
				reasons[d.Reason] = struct{}{}
			}
		}
		if mode == "" {
			mode = d.Mode
		}
	}
	reasonText := ""
	if len(reasons) > 0 {
		keys := make([]string, 0, len(reasons))
		for k := range reasons {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		reasonText = strings.Join(keys, ", ")
	}
	if mode == "tree" {
		msg := fmt.Sprintf("expanded %d directories as tree, %d files discovered, %d file bodies included", len(dirs), totalDiscovered, totalIncluded)
		if truncated > 0 {
			msg += fmt.Sprintf(" [truncated: %d", truncated)
			if reasonText != "" {
				msg += fmt.Sprintf(", reason=%s", reasonText)
			}
			msg += "]"
		}
		return msg
	}
	msg := fmt.Sprintf("expanded %d directories, %d files included, %d discovered, %s", len(dirs), totalIncluded, totalDiscovered, formatBytesIEC(int64(totalBytes)))
	if totalIgnored > 0 {
		msg += fmt.Sprintf(" [warning: %d files omitted by gitignore]", totalIgnored)
	}
	if truncated > 0 {
		msg += fmt.Sprintf(" [truncated: %d", truncated)
		if reasonText != "" {
			msg += fmt.Sprintf(", reason=%s", reasonText)
		}
		msg += "]"
	}
	return msg
}

func formatBytesIEC(n int64) string {
	if n < 1024 {
		return fmt.Sprintf("%d B", n)
	}
	if n < 1024*1024 {
		return fmt.Sprintf("%.1f KiB", float64(n)/1024.0)
	}
	return fmt.Sprintf("%.1f MiB", float64(n)/(1024.0*1024.0))
}

func (m *Model) refreshFileIndexCmd(source string) tea.Cmd {
	if m.fileIndex == nil {
		return nil
	}
	idx := m.fileIndex
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		err := idx.Refresh(ctx)
		return fileIndexRefreshedMsg{
			Err:       err,
			Count:     len(idx.Snapshot()),
			Truncated: idx.Truncated(),
			Source:    source,
		}
	}
}

// startQueuedPrompt starts an agent run for a prompt that was already appended to
// app.Messages and the visible transcript when it was queued.
func (m *Model) startQueuedPrompt() tea.Cmd {
	appState := m.store.Get()
	runCtx, cancel := context.WithCancel(context.Background())
	parentAbort := make(chan struct{})
	go func() {
		<-runCtx.Done()
		close(parentAbort)
	}()
	m.activeRunCtx = runCtx
	m.cancelRun = cancel
	m.compactCh = make(chan struct{}, 1)
	m.runStartedAt = time.Now()
	m.firstStreamAt = time.Time{}
	m.firstRenderSet = false
	m.slowStageNotified = map[string]bool{}
	m.activeRunKind = "main"

	agentInput := agent.Input{
		Model:            appState.ActiveModel,
		LLMProvider:      appState.LLMProvider,
		Messages:         append([]llm.Message(nil), appState.Messages...),
		ToolContext:      appState.ToolContext(runCtx),
		ContextMode:      appState.ContextMode,
		HistoryPolicy:    agent.HistoryPolicyDefault,
		PermissionMode:   appState.PermissionMode,
		PermissionRules:  appState.PermissionRules,
		PermissionPrompt: m.broker.PromptFunc(),
		ParentAbort:      parentAbort,
		CompactRequest:   m.compactCh,
		MaxOutputTokens:  appState.MaxOutputTokens,
	}
	if len(agentInput.Messages) > 0 {
		last := agentInput.Messages[len(agentInput.Messages)-1]
		if last.Role == llm.RoleUser && strings.Contains(last.Content, "Directory tree data:") {
			agentInput.HistoryPolicy = agent.HistoryPolicyLatestOnly
			agentInput.PromptIntent = string(mentions.IntentDirectoryListing)
			agentInput.AttachmentPolicy = string(mentions.AttachListingTreeOnly)
			agentInput.Messages = []llm.Message{last}
		}
	}
	return startAgentCmd(runCtx, m.agent, agentInput, func(msg tea.Msg) {
		if m.program != nil {
			m.program.Send(msg)
		}
	})
}

func (m *Model) startBTWPrompt(question string) tea.Cmd {
	appState := m.store.Get()
	runCtx, cancel := context.WithCancel(context.Background())
	parentAbort := make(chan struct{})
	go func() {
		<-runCtx.Done()
		close(parentAbort)
	}()
	m.activeRunCtx = runCtx
	m.cancelRun = cancel
	m.compactCh = make(chan struct{}, 1)
	m.runStartedAt = time.Now()
	m.firstStreamAt = time.Time{}
	m.firstRenderSet = false
	m.slowStageNotified = map[string]bool{}
	m.activeRunKind = "btw"

	m.transcript = append(m.transcript, CreateSystemItem("[BTW] Running isolated side question in read-only mode"))
	m.transcript = append(m.transcript, CreateUserItem("[BTW] "+question))
	m.refreshViewportContent(true)

	agentInput := agent.Input{
		Model:            appState.ActiveModel,
		LLMProvider:      appState.LLMProvider,
		Messages:         []llm.Message{{Role: llm.RoleUser, Content: question}},
		ToolContext:      appState.ToolContext(runCtx),
		ContextMode:      appState.ContextMode,
		HistoryPolicy:    agent.HistoryPolicyLatestOnly,
		ToolsetName:      agent.ToolsetReadOnly,
		PermissionMode:   permissions.ModePlan,
		PermissionRules:  appState.PermissionRules,
		PermissionPrompt: m.broker.PromptFunc(),
		ParentAbort:      parentAbort,
		CompactRequest:   m.compactCh,
		MaxOutputTokens:  appState.MaxOutputTokens,
	}
	return startAgentCmd(runCtx, m.agent, agentInput, func(msg tea.Msg) {
		if m.program != nil {
			m.program.Send(msg)
		}
	})
}

func (m *Model) handleBackgroundCommand(_ []string) {
	if !m.store.Get().ActiveRun {
		m.backgroundedRun = false
		m.transcript = append(m.transcript, CreateSystemItem("[BG] No active run"))
		m.refreshViewportContent(true)
		return
	}
	if !m.backgroundedRun {
		m.backgroundedRun = true
		m.transcript = append(m.transcript, CreateSystemItem("[BG] Active run moved to background. Use /bg to inspect status."))
		m.refreshViewportContent(true)
		return
	}
	runState := m.snapshotRunUIState(m.store.Get(), time.Now())
	m.transcript = append(m.transcript, CreateSystemItem(fmt.Sprintf("[BG] phase=%s queued=%d", runState.Phase, runState.QueuedCount)))
	m.refreshViewportContent(true)
}

func (m *Model) handleBTWCommand(args []string) tea.Cmd {
	question := strings.TrimSpace(strings.Join(args, " "))
	if question == "" {
		m.transcript = append(m.transcript, CreateSystemItem("[Error: Usage: /btw <question>]"))
		m.refreshViewportContent(true)
		return nil
	}
	if m.store.Get().ActiveRun {
		m.pendingBTW = question
		m.transcript = append(m.transcript, CreateSystemItem("[BTW] queued. It will run after the active run finishes."))
		m.refreshViewportContent(true)
		return nil
	}
	m.store.Set(func(app state.App) state.App {
		app.ActiveRun = true
		return app
	})
	return m.startBTWPrompt(question)
}

func (m *Model) handleSemanticCommand(args []string) tea.Cmd {
	if len(args) == 0 {
		mode := strings.ToLower(strings.TrimSpace(m.semanticConfig.Mode))
		if mode == "" {
			mode = "auto"
		}
		if !m.semanticConfig.Enabled {
			mode = "off"
		}
		content := fmt.Sprintf("[Semantic retrieval: mode=%s light=%d/%d/%dB/%dms full=%d/%d/%dB/%dms deep=%d/%d/%dB/%dms]",
			mode,
			m.semanticConfig.LightTopKRecords, m.semanticConfig.LightTopKFiles, m.semanticConfig.LightMaxContextBytes, m.semanticConfig.LightDeadlineMS,
			m.semanticConfig.FullTopKRecords, m.semanticConfig.FullTopKFiles, m.semanticConfig.FullMaxContextBytes, m.semanticConfig.FullDeadlineMS,
			m.semanticConfig.DeepTopKRecords, m.semanticConfig.DeepTopKFiles, m.semanticConfig.DeepMaxContextBytes, m.semanticConfig.DeepDeadlineMS,
		)
		if m.semanticService != nil {
			status, err := m.semanticService.Status(context.Background(), m.store.Get().ToolSettings.WorkingDir)
			if err == nil {
				content = fmt.Sprintf("%s [index exists=%t compatible=%t records=%d files=%d]",
					content, status.Exists, status.Compatible, status.RecordCount, status.FileCount)
			}
		}
		m.transcript = append(m.transcript, CreateSystemItem(content))
		m.refreshViewportContent(true)
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "on":
		m.semanticConfig.Enabled = true
		m.semanticConfig.Mode = "auto"
		m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval enabled: mode=auto]"))
	case "off":
		m.semanticConfig.Enabled = false
		m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval disabled for this session]"))
	case "auto":
		m.semanticConfig.Enabled = true
		m.semanticConfig.Mode = "auto"
		m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval mode set to auto]"))
	case "explicit":
		m.semanticConfig.Enabled = true
		m.semanticConfig.Mode = "explicit"
		m.transcript = append(m.transcript, CreateSystemItem("[Semantic retrieval mode set to explicit]"))
	case "deep":
		m.semanticDeepNext = true
		m.transcript = append(m.transcript, CreateSystemItem("[Semantic deep: next prompt will use broader semantic retrieval]"))
	case "status":
		return m.handleSemanticCommand(nil)
	default:
		m.transcript = append(m.transcript, CreateSystemItem("[Error: Usage: /semantic on|off|auto|explicit|status|deep]"))
	}
	m.refreshViewportContent(true)
	return nil
}

func semanticRouteConfig(cfg semantic.Config, hasService bool) retrievalroute.Config {
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

func (m *Model) handleIndexCommand(args []string) tea.Cmd {
	if m.semanticService == nil {
		m.transcript = append(m.transcript, CreateSystemItem("[Index unavailable: semantic service not configured]"))
		m.refreshViewportContent(true)
		return nil
	}
	if len(args) == 0 {
		m.transcript = append(m.transcript, CreateSystemItem("[Error: Usage: /index build|refresh|status|clear]"))
		m.refreshViewportContent(true)
		return nil
	}
	root := m.store.Get().ToolSettings.WorkingDir
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "status":
		return func() tea.Msg {
			st, err := m.semanticService.Status(context.Background(), root)
			if err != nil {
				return indexOpDoneMsg{Err: err}
			}
			model := st.Model
			if strings.TrimSpace(model) == "" {
				model = "n/a"
			}
			return indexOpDoneMsg{Content: fmt.Sprintf(
				"[Index status: exists=%t compatible=%t model=%s dims=%d records=%d files=%d]",
				st.Exists, st.Compatible, model, st.Dimensions, st.RecordCount, st.FileCount,
			)}
		}
	case "clear":
		return func() tea.Msg {
			if err := m.semanticService.Clear(context.Background(), root); err != nil {
				return indexOpDoneMsg{Err: err}
			}
			return indexOpDoneMsg{Content: "[Index cleared]"}
		}
	case "build":
		if m.indexProgress.Active {
			m.transcript = append(m.transcript, CreateSystemItem("[Index already running]"))
			m.refreshViewportContent(true)
			return nil
		}
		m.startIndexProgress("build")
		m.transcript = append(m.transcript, CreateSystemItem("[Index build started: scanning workspace...]"))
		m.refreshViewportContent(true)
		cfg := m.semanticConfig
		sink := m.newIndexEventSink()
		return func() tea.Msg {
			report, err := m.semanticService.Build(context.Background(), semantic.BuildRequest{
				Root:      root,
				Config:    cfg,
				EventSink: sink,
			})
			if err != nil {
				return indexOpDoneMsg{Err: err}
			}
			return indexOpDoneMsg{Content: fmt.Sprintf(
				"[Index build complete: files_seen=%d files_indexed=%d records=%d skipped=%d batches=%d duration=%s]",
				report.FilesSeen, report.FilesIndexed, report.RecordsIndexed, report.FilesSkipped, report.EmbedBatches, report.Duration.Round(time.Millisecond),
			)}
		}
	case "refresh":
		if m.indexProgress.Active {
			m.transcript = append(m.transcript, CreateSystemItem("[Index already running]"))
			m.refreshViewportContent(true)
			return nil
		}
		m.startIndexProgress("refresh")
		m.transcript = append(m.transcript, CreateSystemItem("[Index refresh started: scanning workspace...]"))
		m.refreshViewportContent(true)
		cfg := m.semanticConfig
		sink := m.newIndexEventSink()
		return func() tea.Msg {
			report, err := m.semanticService.Refresh(context.Background(), semantic.RefreshRequest{
				Root:      root,
				Config:    cfg,
				MaxFiles:  cfg.PromptRefreshMaxFiles,
				Timeout:   cfg.PromptRefreshTimeout,
				EventSink: sink,
			})
			if err != nil {
				return indexOpDoneMsg{Err: err}
			}
			return indexOpDoneMsg{Content: fmt.Sprintf(
				"[Index refresh complete: files_seen=%d files_indexed=%d records=%d skipped=%d batches=%d duration=%s]",
				report.FilesSeen, report.FilesIndexed, report.RecordsIndexed, report.FilesSkipped, report.EmbedBatches, report.Duration.Round(time.Millisecond),
			)}
		}
	default:
		m.transcript = append(m.transcript, CreateSystemItem("[Error: Usage: /index build|refresh|status|clear]"))
		m.refreshViewportContent(true)
		return nil
	}
}

func (m *Model) markFirstStreamEvent() {
	if !m.firstStreamAt.IsZero() {
		return
	}
	m.firstStreamAt = time.Now()
}

func (m *Model) recordFirstVisibleRenderIfNeeded() {
	if m.meter == nil || m.firstRenderSet || m.firstStreamAt.IsZero() {
		return
	}
	now := time.Now()
	m.meter.NotePendingRunStage("first_stream_to_visible_render", now.Sub(m.firstStreamAt))
	if !m.runStartedAt.IsZero() {
		m.meter.NotePendingRunStage("first_visible_render", now.Sub(m.runStartedAt))
	}
	m.appendSlowStageNotice("first_stream_to_visible_render", now.Sub(m.firstStreamAt))
	m.firstRenderSet = true
}

func (m *Model) appendSlowStageNotice(stage string, dur time.Duration) {
	if stage == "" || dur < m.slowStageNoticeThreshold() {
		return
	}
	if m.slowStageNotified == nil {
		m.slowStageNotified = map[string]bool{}
	}
	if m.slowStageNotified[stage] {
		return
	}
	m.slowStageNotified[stage] = true
	m.transcript = append(m.transcript, CreateSystemItem(
		fmt.Sprintf("[slow stage] %s took %s", stage, dur.Round(time.Millisecond)),
	))
}

func (m *Model) appendStageSummaryFromTrace() {
	if m.meter == nil {
		return
	}
	threshold := m.slowStageNoticeThreshold()
	trace := m.meter.Snapshot().LastRunTrace
	if len(trace.StageLatencies) == 0 {
		return
	}
	type pair struct {
		stage string
		dur   time.Duration
	}
	rows := make([]pair, 0, len(trace.StageLatencies))
	for stage, dur := range trace.StageLatencies {
		if stage == "" || dur < threshold {
			continue
		}
		rows = append(rows, pair{stage: stage, dur: dur})
	}
	if len(rows) == 0 {
		return
	}
	slices.SortFunc(rows, func(a, b pair) int {
		if a.dur == b.dur {
			return strings.Compare(a.stage, b.stage)
		}
		if a.dur > b.dur {
			return -1
		}
		return 1
	})
	if len(rows) > 3 {
		rows = rows[:3]
	}
	parts := make([]string, 0, len(rows))
	for _, row := range rows {
		parts = append(parts, fmt.Sprintf("%s %s", row.stage, row.dur.Round(time.Millisecond)))
	}
	m.transcript = append(m.transcript, CreateSystemItem("[stage summary] slowest: "+strings.Join(parts, ", ")))
}

func (m *Model) slowStageNoticeThreshold() time.Duration {
	threshold := m.store.Get().SlowStageNoticeThreshold
	if threshold <= 0 {
		return 750 * time.Millisecond
	}
	return threshold
}

func (m *Model) computeViewportHeight() int {
	height := m.height - 8 - m.pickerExtraHeight()
	if height < 3 {
		height = 3
	}
	return height
}

func (m *Model) pickerExtraHeight() int {
	if !m.vim.IsInsert() || !m.picker.Visible {
		return 0
	}
	rows := len(m.picker.Items)
	if rows == 0 {
		rows = 1
	}
	maxRows := m.pickerMaxRows()
	if rows > maxRows {
		rows = maxRows
	}
	return rows + 3
}

func (m *Model) pickerMaxRows() int {
	maxRows := 8
	if m.height <= 0 {
		return maxRows
	}
	// reserve transcript + input + status + panel framing
	room := m.height - 12
	if room < 1 {
		room = 1
	}
	if room < maxRows {
		return room
	}
	return maxRows
}

func (m *Model) refreshViewportContent(gotoBottom bool) {
	oldOffset := m.viewport.YOffset
	wasAtBottom := m.viewport.AtBottom()
	updated := m.ensureViewportContent()
	if !updated {
		if gotoBottom && wasAtBottom {
			m.viewport.GotoBottom()
		}
		return
	}
	if gotoBottom && wasAtBottom {
		m.viewport.GotoBottom()
		return
	}
	if oldOffset > 0 {
		m.viewport.SetYOffset(oldOffset)
	}
}

// renderTranscript renders the transcript into ANSI text.
func (m *Model) renderTranscript() string {
	_, rendered := m.renderTranscriptCached()
	return rendered
}

func (m *Model) renderTranscriptCached() (transcriptCacheKey, string) {
	key := m.transcriptCacheKey()
	if m.transcriptRenderValid && m.transcriptRenderKey == key {
		return key, m.transcriptRender
	}
	rendered := m.renderTranscriptUncached(key.Start, key.End, key.ActiveRun)
	m.transcriptRenderKey = key
	m.transcriptRender = rendered
	m.transcriptRenderValid = true
	return key, rendered
}

func (m *Model) renderTranscriptUncached(start, end int, activeRun bool) string {
	if len(m.transcript) == 0 {
		if m.everHadInput {
			return m.styles.Help.Render("transcript cleared - type a new prompt")
		}
		return m.styles.Help.Render("Type a prompt or /help to begin")
	}

	var output strings.Builder
	for i := start; i < end; i++ {
		item := &m.transcript[i]
		switch item.Kind {
		case TranscriptUser:
			output.WriteString(m.styles.Help.Render("You:"))
			output.WriteString("\n")
			output.WriteString(item.Content)
			output.WriteString("\n\n")

		case TranscriptAssistant:
			output.WriteString(m.styles.Help.Render("Assistant:"))
			output.WriteString("\n")
			// Keep streaming path cheap: render the active tail assistant block as
			// plain text while a run is active, then markdown-render/cache later.
			if activeRun && i == end-1 {
				output.WriteString(item.Content)
			} else if m.renderer != nil && item.Content != "" {
				if item.Rendered == "" {
					rendered, _ := m.renderer.Render(item.Content)
					item.Rendered = rendered
				}
				output.WriteString(item.Rendered)
			} else {
				output.WriteString(item.Content)
			}
			output.WriteString("\n\n")

		case TranscriptThinking:
			if item.Collapsed {
				label := "Thinking"
				if item.Streaming {
					label = "Thinking..."
				} else if item.CharCount > 0 {
					label = fmt.Sprintf("Thinking (%d chars) Ctrl+T to expand", item.CharCount)
				}
				output.WriteString(m.styles.ThinkingCollapsed.Render(label))
				output.WriteString("\n")
				output.WriteString("\n\n")
			} else {
				header := fmt.Sprintf("Thinking (%d chars) Ctrl+T to collapse", item.CharCount)
				output.WriteString(m.styles.ThinkingExpanded.Render(header))
				output.WriteString("\n")
				output.WriteString(m.styles.ThinkingBox.Render(item.Content))
				output.WriteString("\n\n")
			}

		case TranscriptTool:
			toolPanel := m.renderToolPanel(*item)
			output.WriteString(toolPanel)
			output.WriteString("\n")

		case TranscriptSystem:
			output.WriteString(m.styles.Help.Render("[System]"))
			output.WriteString(" ")
			output.WriteString(item.Content)
			output.WriteString("\n\n")
		}
		if m.heightCache != nil {
			if _, ok := m.heightCache[i]; !ok {
				m.heightCache[i] = estimateTranscriptItemLines(*item)
			}
		}
	}

	return output.String()
}

func (m *Model) transcriptCacheKey() transcriptCacheKey {
	start, end := m.transcriptWindowBounds()
	return transcriptCacheKey{
		MutationSig: m.transcriptMutationSignature(),
		Start:       start,
		End:         end,
		ActiveRun:   m.store.Get().ActiveRun,
		Width:       m.width,
		StyleSig:    m.transcriptStyleSignature(),
		EverHad:     m.everHadInput,
	}
}

func (m *Model) ensureViewportContent() bool {
	key, rendered := m.renderTranscriptCached()
	if m.viewportRenderValid && m.viewportRenderKey == key {
		return false
	}
	m.viewport.SetContent(rendered)
	m.viewportRenderKey = key
	m.viewportRenderValid = true
	return true
}

func (m *Model) invalidateTranscriptRenderCache() {
	m.transcriptRenderValid = false
	m.viewportRenderValid = false
}

func (m *Model) invalidateAssistantMarkdownCache() {
	for i := range m.transcript {
		if m.transcript[i].Kind == TranscriptAssistant && m.transcript[i].Rendered != "" {
			m.transcript[i].Rendered = ""
		}
	}
}

func (m *Model) transcriptMutationSignature() uint64 {
	const offset = uint64(1469598103934665603)
	const prime = uint64(1099511628211)
	sig := offset
	sig ^= uint64(len(m.transcript))
	sig *= prime
	for i := range m.transcript {
		item := m.transcript[i]
		sig ^= uint64(i + 1)
		sig *= prime
		sig ^= uint64(item.Kind[0])
		sig *= prime
		sig ^= boolToUint64(item.Collapsed)
		sig *= prime
		sig ^= boolToUint64(item.Streaming)
		sig *= prime
		sig ^= uint64(item.CharCount)
		sig *= prime
		sig = hashStringSample(sig, item.Content)
		sig = hashStringSample(sig, item.Error)
		sig = hashStringSample(sig, item.ToolID)
		sig = hashStringSample(sig, item.ToolName)
	}
	return sig
}

func (m *Model) transcriptStyleSignature() uint64 {
	const offset = uint64(1469598103934665603)
	sig := offset
	sig = hashStringSample(sig, m.styles.Help.Render("h"))
	sig = hashStringSample(sig, m.styles.ThinkingCollapsed.Render("t"))
	sig = hashStringSample(sig, m.styles.ThinkingExpanded.Render("t"))
	sig = hashStringSample(sig, m.styles.ThinkingBox.Render("t"))
	sig = hashStringSample(sig, m.styles.ToolPanel.Render("p"))
	sig = hashStringSample(sig, m.styles.StatusError.Render("e"))
	return sig
}

func hashStringSample(sig uint64, s string) uint64 {
	const prime = uint64(1099511628211)
	sig ^= uint64(len(s))
	sig *= prime
	if len(s) == 0 {
		return sig
	}
	prefix := 16
	if len(s) < prefix {
		prefix = len(s)
	}
	for i := 0; i < prefix; i++ {
		sig ^= uint64(s[i])
		sig *= prime
	}
	start := len(s) - prefix
	if start < prefix {
		start = prefix
	}
	for i := start; i < len(s); i++ {
		sig ^= uint64(s[i])
		sig *= prime
	}
	return sig
}

func boolToUint64(v bool) uint64 {
	if v {
		return 1
	}
	return 0
}

func (m *Model) transcriptWindow() []TranscriptItem {
	start, end := m.transcriptWindowBounds()
	if start >= end || start < 0 || end < 0 {
		return nil
	}
	return m.transcript[start:end]
}

func (m *Model) transcriptWindowBounds() (int, int) {
	if len(m.transcript) == 0 {
		return 0, 0
	}
	if !m.viewport.AtBottom() {
		return 0, len(m.transcript)
	}
	lineBudget := m.transcriptVirtualLineBudget()
	lines := 0
	start := len(m.transcript)
	for i := len(m.transcript) - 1; i >= 0; i-- {
		h := 0
		if m.heightCache != nil {
			h = m.heightCache[i]
		}
		if h <= 0 {
			h = estimateTranscriptItemLines(m.transcript[i])
		}
		if start < len(m.transcript) && lines+h > lineBudget {
			break
		}
		start = i
		lines += h
	}
	return start, len(m.transcript)
}

func (m *Model) transcriptVirtualLineBudget() int {
	h := m.computeViewportHeight()
	if h < 1 {
		h = 20
	}
	size := h * 10
	if size < 300 {
		size = 300
	}
	if size > 4000 {
		size = 4000
	}
	return size
}

func estimateTranscriptItemLines(item TranscriptItem) int {
	switch item.Kind {
	case TranscriptTool:
		return 4
	case TranscriptThinking:
		if item.Collapsed {
			return 3
		}
		return 4 + countLines(item.Content)
	default:
		return 3 + countLines(item.Content)
	}
}

func countLines(s string) int {
	if s == "" {
		return 1
	}
	return strings.Count(s, "\n") + 1
}

func (m *Model) handleTranscriptSearch(args []string) {
	if len(args) == 0 {
		m.transcript = append(m.transcript, CreateSystemItem("[Error: Usage: /search <query>|clear]"))
		m.refreshViewportContent(true)
		return
	}
	if strings.EqualFold(strings.TrimSpace(args[0]), "clear") {
		m.searchQuery = ""
		m.searchMatches = nil
		m.searchPos = 0
		m.transcript = append(m.transcript, CreateSystemItem("[Search cleared]"))
		m.refreshViewportContent(true)
		return
	}
	q := strings.ToLower(strings.TrimSpace(strings.Join(args, " ")))
	if q == "" {
		m.transcript = append(m.transcript, CreateSystemItem("[Error: empty search query]"))
		m.refreshViewportContent(true)
		return
	}
	m.searchQuery = q
	m.searchMatches = m.searchMatches[:0]
	m.searchPos = 0
	for i := range m.transcript {
		if strings.Contains(strings.ToLower(m.transcript[i].Content), q) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}
	if len(m.searchMatches) == 0 {
		m.transcript = append(m.transcript, CreateSystemItem(fmt.Sprintf("[Search: no matches for %q]", q)))
		m.refreshViewportContent(true)
		return
	}
	m.scrollToTranscriptIndex(m.searchMatches[0])
	m.transcript = append(m.transcript, CreateSystemItem(
		fmt.Sprintf("[Search: %d matches for %q | n next | N prev]", len(m.searchMatches), q),
	))
	m.refreshViewportContent(true)
}

func (m *Model) scrollToTranscriptIndex(idx int) {
	if idx < 0 || idx >= len(m.transcript) {
		return
	}
	offset := 0
	for i := 0; i < idx; i++ {
		h := estimateTranscriptItemLines(m.transcript[i])
		if m.heightCache != nil {
			if ch, ok := m.heightCache[i]; ok && ch > 0 {
				h = ch
			}
		}
		offset += h
	}
	m.viewport.SetYOffset(offset)
}

// renderInput renders the input area.
func (m *Model) renderInput() string {
	if m.vim.IsNormal() {
		return lipgloss.NewStyle().Foreground(lipgloss.Color("243")).Render("-- NORMAL -- (i to insert)")
	}

	inputBox := m.styles.Border.Render(m.input.View())
	if !m.picker.Visible {
		return inputBox
	}
	return lipgloss.JoinVertical(lipgloss.Left, inputBox, m.renderPicker())
}

func (m *Model) renderPicker() string {
	rows := m.picker.Items
	limit := m.pickerMaxRows()
	if len(rows) > limit {
		rows = rows[:limit]
	}
	if len(rows) == 0 {
		no := m.styles.PickerItem.Render("No matches")
		hint := m.styles.PickerHint.Render("Enter submit · Esc close")
		return lipgloss.JoinVertical(lipgloss.Left, m.styles.PickerPanel.Render(no), hint)
	}

	contentWidth := m.width - 6
	if contentWidth < 20 {
		contentWidth = 20
	}
	detailWidth := 10
	lines := make([]string, 0, len(rows))
	for i, item := range rows {
		left := m.highlightMatch(item.Display, item.MatchRunes)
		right := m.styles.PickerDetail.Render(item.Detail)
		mainWidth := contentWidth - detailWidth
		if mainWidth < 8 {
			mainWidth = contentWidth
		}
		left = lipgloss.NewStyle().Width(mainWidth).MaxWidth(mainWidth).Render(left)
		row := left
		if mainWidth != contentWidth {
			row = left + m.styles.PickerDetail.Width(detailWidth).Align(lipgloss.Right).Render(right)
		}
		if i == m.picker.Index {
			row = m.styles.PickerSelected.Render("› " + row)
		} else {
			row = m.styles.PickerItem.Render("  " + row)
		}
		lines = append(lines, row)
	}
	panel := m.styles.PickerPanel.Render(strings.Join(lines, "\n"))
	hintText := "Tab/Enter accept · ↑↓ navigate · Esc close"
	if m.picker.Trigger == picker.TriggerFile {
		hintText = "Tab/Enter accept · Shift+Tab accept dir · ↑↓ navigate · Esc close"
	}
	if m.fileIndex != nil && m.picker.Trigger == picker.TriggerFile && m.fileIndex.Truncated() {
		hintText += " · index truncated"
	}
	hint := m.styles.PickerHint.Render(hintText)
	return lipgloss.JoinVertical(lipgloss.Left, panel, hint)
}

func (m *Model) highlightMatch(display string, matched []int) string {
	if len(matched) == 0 {
		return display
	}
	sort.Ints(matched)
	matchSet := make(map[int]struct{}, len(matched))
	for _, idx := range matched {
		matchSet[idx] = struct{}{}
	}
	runes := []rune(display)
	var b strings.Builder
	for i, r := range runes {
		if _, ok := matchSet[i]; ok {
			b.WriteString(m.styles.PickerMatch.Render(string(r)))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// renderStatusBar renders the status bar.
func (m *Model) renderStatusBar(appState state.App) string {
	status := fmt.Sprintf("Model: %s | Mode: %s", appState.ActiveModel, m.vim.Mode)
	if appState.LLMProvider == string(llm.ProviderOllamaCloudAPI) {
		status += " | Provider: Ollama Cloud"
	}
	if appState.CoordinatorMode {
		status += " | [COORDINATOR]"
	}
	runState := m.snapshotRunUIState(appState, time.Now())
	switch runState.Phase {
	case RunPhasePermissionRequired:
		status += " | [Permission required]"
	case RunPhaseRunningTool:
		for _, tool := range appState.ActiveTools {
			if !tool.Done {
				elapsed := ""
				if !tool.StartedAt.IsZero() {
					elapsed = " " + formatElapsedCompact(time.Since(tool.StartedAt))
				}
				status += fmt.Sprintf(" | [Running %s%s]", tool.Name, elapsed)
				break
			}
		}
	case RunPhaseCompacting:
		status += " | [Compacting...]"
	case RunPhaseRetrying:
		status += " | [Retrying...]"
	case RunPhaseThinking:
		status += " | [Thinking...]"
	case RunPhaseStreaming:
		status += " | [Streaming...]"
	case RunPhaseWaitingModel:
		status += " | [Waiting for model...]"
	case RunPhaseQueued:
		status += fmt.Sprintf(" | [%d queued]", runState.QueuedCount)
	}
	if m.backgroundedRun && appState.ActiveRun {
		status += " | [Background]"
	}
	if strings.TrimSpace(m.pendingBTW) != "" {
		status += " | [BTW queued]"
	}
	// Count running tasks
	runningTasks := 0
	for _, t := range appState.Tasks {
		if t.Status == "running" {
			runningTasks++
		}
	}
	if runningTasks > 0 {
		status += fmt.Sprintf(" | [%d tasks running]", runningTasks)
	}
	if appState.CoordinatorMode {
		status += fmt.Sprintf(" | [workers: %d]", appState.WorkerCount)
	}
	if progress := strings.TrimSpace(m.renderIndexProgressStatus()); progress != "" {
		status += " | [" + progress + "]"
	}
	if m.meter != nil {
		snap := m.meter.Snapshot()
		if snap.TotalTokens > 0 {
			status += fmt.Sprintf(" | session tokens: %d", snap.TotalTokens)
		}
	}
	return m.styles.StatusBar.Render(status)
}

func (m *Model) renderActiveTaskLine(appState state.App) string {
	if !appState.ActiveRun {
		return m.styles.SemMuted.Render("Activity: idle")
	}
	runState := m.snapshotRunUIState(appState, time.Now())
	label := strings.TrimSpace(runState.Label)
	if label == "" {
		label = string(runState.Phase)
	}
	line := fmt.Sprintf("Activity: %s", label)
	if m.backgroundedRun {
		line += " (background)"
	}
	return m.styles.SemInfo.Render(line)
}

func (m *Model) renderTipLine(appState state.App) string {
	if appState.ActiveRun && !m.backgroundedRun {
		return m.styles.SemMuted.Render("Tip: use /bg to background this run or /btw <question> for a side question.")
	}
	if strings.TrimSpace(m.pendingBTW) != "" {
		return m.styles.SemWarning.Render("Tip: queued /btw will run after the current active run.")
	}
	return m.styles.SemMuted.Render("Tip: use /help for commands.")
}

func (m *Model) shouldRunTick(now time.Time) bool {
	appState := m.store.Get()
	if appState.ActiveRun {
		return true
	}
	if m.compactingActive {
		return true
	}
	return !m.retryActiveUntil.IsZero() && now.Before(m.retryActiveUntil)
}

func (m *Model) nextTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) shouldRefreshStreamingEvent() bool {
	// Keep tests/deterministic local model updates eager unless we're in
	// interactive program mode, where tick-driven refresh can smooth bursty
	// stream updates.
	if m.program == nil {
		return true
	}
	now := time.Now()
	if m.lastStreamRenderAt.IsZero() || now.Sub(m.lastStreamRenderAt) >= 50*time.Millisecond {
		m.lastStreamRenderAt = now
		return true
	}
	return false
}

func formatElapsedCompact(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return "<1s"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Round(time.Second).Seconds()))
	}
	mins := int(d / time.Minute)
	secs := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", mins, secs)
}

func (m *Model) syncBindingContexts() {
	if m.bindingStack == nil {
		m.bindingStack = NewBindingStack()
	}
	// Keep global pinned as fallback.
	stack := []BindingContext{ContextGlobal}
	hasModal := m.store.Get().PermissionPrompt != nil || m.cloudCredentialPrompt != nil
	if m.vim != nil && m.vim.IsNormal() {
		stack = append(stack, ContextVimNormal)
	} else {
		stack = append(stack, ContextVimInsert)
	}
	if !hasModal && len(m.transcript) > 0 {
		stack = append(stack, ContextScroll)
	}
	if hasModal {
		// Modal must be the highest-priority context.
		stack = append(stack, ContextModal)
	}
	m.bindingStack.stack = stack
}

func hasRunningTool(tools map[string]state.ToolUse) bool {
	for _, tool := range tools {
		if !tool.Done {
			return true
		}
	}
	return false
}

// renderToolPanel renders a tool panel.
func (m *Model) renderToolPanel(item TranscriptItem) string {
	content := fmt.Sprintf("🔧 %s (%s): %s", item.ToolName, shortID(item.ToolID, 8), item.Content)
	if item.Error != "" {
		return m.styles.StatusError.Render(content)
	}
	return m.styles.ToolPanel.Render(content)
}

// renderPermissionModal renders the permission prompt modal.
func (m *Model) renderPermissionModal(prompt *state.PermissionPrompt) string {
	mode := m.store.Get().PermissionMode
	modalContent := fmt.Sprintf(`%s

Tool: %s
Target: %s
Mode: %s

Reason: %s

[a] Allow once  [d] Deny  [A] Always Allow  [Esc] Deny
`,
		m.styles.ModalTitle.Render("Permission Required"),
		prompt.ToolName,
		prompt.Target,
		mode,
		prompt.Reason,
	)

	return m.styles.Modal.Render(modalContent)
}

// overlayModal overlays a modal on top of the main content.
func (m *Model) overlayModal(content, modal string) string {
	if m.width <= 0 {
		return lipgloss.JoinVertical(lipgloss.Left, content, modal)
	}
	baseLines := strings.Split(content, "\n")
	modalLines := strings.Split(modal, "\n")
	if len(modalLines) == 0 || len(baseLines) == 0 {
		return content
	}
	start := (len(baseLines) - len(modalLines)) / 2
	if start < 0 {
		start = 0
	}
	for i, line := range modalLines {
		row := start + i
		if row >= len(baseLines) {
			break
		}
		baseLines[row] = lipgloss.Place(m.width, 1, lipgloss.Center, lipgloss.Center, line)
	}
	return strings.Join(baseLines, "\n")
}

func (m *Model) handleMemoryEdit(args []string) tea.Cmd {
	if len(args) < 2 {
		m.transcript = append(m.transcript, CreateSystemItem("[Error: Usage: /memory edit <name>]"))
		m.refreshViewportContent(true)
		return nil
	}
	path, err := commands.PrepareMemoryEditFile(m.cmdCtx.MemoryDir, args[1])
	if err != nil {
		m.transcript = append(m.transcript, CreateSystemItem("[Error: "+err.Error()+"]"))
		m.refreshViewportContent(true)
		return nil
	}
	editor := strings.TrimSpace(os.Getenv("EDITOR"))
	if editor == "" {
		editor = "vi"
	}
	cmd := exec.Command(editor, path)
	return tea.ExecProcess(cmd, func(err error) tea.Msg {
		return memoryEditDoneMsg{Path: path, Err: err}
	})
}

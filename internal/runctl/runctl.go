// Package runctl holds the surface-neutral pieces of agent-run control: the
// cancel and manual-compact channels for one active run (Controller), the
// end-of-run state update (ApplyTerminal), and the reusable /clear and
// /compact state operations. The TUI, the browser server and --print each
// used to carry their own copy; they call these instead.
package runctl

import (
	"context"
	"errors"
	"sync"
)

// ErrRunActive is returned by Begin when a run is already in progress.
var ErrRunActive = errors.New("runctl: a run is already active")

// Kind distinguishes a normal run from an isolated side question (/btw),
// whose result is not added to the conversation history.
type Kind string

const (
	KindMain Kind = "main"
	KindSide Kind = "side"
)

// CompactRequestResult reports what RequestCompact did.
type CompactRequestResult int

const (
	// CompactNoRun means no run is active; the caller compacts the stored
	// history directly (see CompactConversation).
	CompactNoRun CompactRequestResult = iota
	// CompactRequested means the signal was delivered to the active run.
	CompactRequested
	// CompactPending means an earlier request has not been consumed yet.
	CompactPending
)

// Run is the set of handles one agent run needs. Pass Ctx as the run context,
// ParentAbort as agent.Input.ParentAbort and CompactRequest as
// agent.Input.CompactRequest.
type Run struct {
	Ctx            context.Context
	ParentAbort    <-chan struct{}
	CompactRequest <-chan struct{}

	id uint64
}

// Controller owns the cancel and compact control of at most one active run.
// It is safe for concurrent use. The zero value is not usable; call New.
type Controller struct {
	mu        sync.Mutex
	nextID    uint64
	activeID  uint64
	active    bool
	kind      Kind
	cancel    context.CancelFunc
	compactCh chan struct{}
}

// New returns an idle Controller.
func New() *Controller { return &Controller{} }

// Begin starts control of a main run. It returns ErrRunActive when a run is
// already active. Call Finish when the run's Terminal event has been handled.
func (c *Controller) Begin(parent context.Context) (Run, error) {
	return c.BeginKind(parent, KindMain)
}

// BeginKind is Begin with an explicit run kind.
func (c *Controller) BeginKind(parent context.Context, kind Kind) (Run, error) {
	if parent == nil {
		parent = context.Background()
	}
	if kind == "" {
		kind = KindMain
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.active {
		return Run{}, ErrRunActive
	}
	ctx, cancel := context.WithCancel(parent)
	abort := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(abort)
	}()
	c.nextID++
	c.activeID = c.nextID
	c.active = true
	c.kind = kind
	c.cancel = cancel
	c.compactCh = make(chan struct{}, 1)
	return Run{Ctx: ctx, ParentAbort: abort, CompactRequest: c.compactCh, id: c.activeID}, nil
}

// Cancel aborts the active run. It reports whether a run was active. The run
// still ends with its normal Terminal event; calling Cancel again is harmless.
func (c *Controller) Cancel() bool {
	c.mu.Lock()
	cancel, active := c.cancel, c.active
	c.mu.Unlock()
	if !active || cancel == nil {
		return false
	}
	cancel()
	return true
}

// RequestCompact asks the active run to compact its context at the next safe
// point. With no active run it returns CompactNoRun and does nothing.
func (c *Controller) RequestCompact() CompactRequestResult {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		return CompactNoRun
	}
	select {
	case c.compactCh <- struct{}{}:
		return CompactRequested
	default:
		return CompactPending
	}
}

// Active reports whether a run is in progress.
func (c *Controller) Active() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.active
}

// Kind returns the kind of the active run, or the last one when idle.
func (c *Controller) Kind() Kind {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.kind
}

// Finish releases the run and cancels its context so nothing leaks. A Finish
// for a run that is no longer current (stale) is ignored.
func (c *Controller) Finish(r Run) {
	c.mu.Lock()
	if !c.active || c.activeID != r.id {
		c.mu.Unlock()
		return
	}
	cancel := c.cancel
	c.active = false
	c.cancel = nil
	c.compactCh = nil
	c.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

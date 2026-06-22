package tui

import "time"

// BindingContext identifies a keybinding resolution context.
type BindingContext string

const (
	ContextGlobal    BindingContext = "global"
	ContextScroll    BindingContext = "scroll"
	ContextVimInsert BindingContext = "vim_insert"
	ContextVimNormal BindingContext = "vim_normal"
	ContextModal     BindingContext = "modal"
)

// BindingStack keeps ordered context priority (last wins).
type BindingStack struct {
	stack []BindingContext
}

func NewBindingStack() *BindingStack {
	return &BindingStack{stack: []BindingContext{ContextGlobal}}
}

func (s *BindingStack) Push(ctx BindingContext) {
	if len(s.stack) > 0 && s.stack[len(s.stack)-1] == ctx {
		return
	}
	s.stack = append(s.stack, ctx)
}

func (s *BindingStack) Pop(ctx BindingContext) {
	for i := len(s.stack) - 1; i >= 0; i-- {
		if s.stack[i] != ctx {
			continue
		}
		s.stack = append(s.stack[:i], s.stack[i+1:]...)
		return
	}
}

func (s *BindingStack) Top() BindingContext {
	if len(s.stack) == 0 {
		return ContextGlobal
	}
	return s.stack[len(s.stack)-1]
}

func (s *BindingStack) Snapshot() []BindingContext {
	out := make([]BindingContext, len(s.stack))
	copy(out, s.stack)
	return out
}

// ChordInterceptor tracks short timed key chords like g+g and g+G.
type ChordInterceptor struct {
	firstKey string
	at       time.Time
	timeout  time.Duration
}

func NewChordInterceptor(timeout time.Duration) *ChordInterceptor {
	return &ChordInterceptor{timeout: timeout}
}

func (c *ChordInterceptor) Start(key string, now time.Time) {
	c.firstKey = key
	c.at = now
}

func (c *ChordInterceptor) Match(secondKey string, now time.Time) (string, bool) {
	if c.firstKey == "" || now.Sub(c.at) > c.timeout {
		c.Reset()
		return "", false
	}
	action := ""
	switch c.firstKey + secondKey {
	case "gg":
		action = "goto_top"
	case "gG":
		action = "goto_bottom"
	}
	c.Reset()
	if action == "" {
		return "", false
	}
	return action, true
}

func (c *ChordInterceptor) Expired(now time.Time) bool {
	if c.firstKey == "" {
		return false
	}
	return now.Sub(c.at) > c.timeout
}

func (c *ChordInterceptor) Reset() {
	c.firstKey = ""
	c.at = time.Time{}
}


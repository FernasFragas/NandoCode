package tui

import "unicode"

// VimMode represents the current Vim-like editor mode.
type VimMode string

const (
	VimModeInsert VimMode = "insert"
	VimModeNormal VimMode = "normal"
	VimModeVisual VimMode = "visual"
)

type Operator string

const (
	OpDelete Operator = "d"
	OpChange Operator = "c"
	OpYank   Operator = "y"
	OpIndent Operator = ">"
	OpOutdent Operator = "<"
)

type TextObjScope string

const (
	TextObjInner  TextObjScope = "inner"
	TextObjAround TextObjScope = "around"
)

// CommandState is the parser state for Normal-mode key sequences.
type CommandState interface{ isCommandState() }

type CmdIdle struct{}
type CmdCount struct{ Digits string }
type CmdOperator struct {
	Op    Operator
	Count int
}
type CmdOperatorCount struct {
	Op     Operator
	Count  int
	Digits string
}
type CmdOperatorFind struct {
	Op    Operator
	Count int
}
type CmdOperatorTextObj struct {
	Op    Operator
	Count int
	Scope TextObjScope
}
type CmdFind struct{ Count int }
type CmdGPrefix struct{ Count int }
type CmdOperatorG struct {
	Op    Operator
	Count int
}
type CmdReplace struct{ Count int }
type CmdIndent struct {
	Dir   Operator
	Count int
}

func (CmdIdle) isCommandState()            {}
func (CmdCount) isCommandState()           {}
func (CmdOperator) isCommandState()        {}
func (CmdOperatorCount) isCommandState()   {}
func (CmdOperatorFind) isCommandState()    {}
func (CmdOperatorTextObj) isCommandState() {}
func (CmdFind) isCommandState()            {}
func (CmdGPrefix) isCommandState()         {}
func (CmdOperatorG) isCommandState()       {}
func (CmdReplace) isCommandState()         {}
func (CmdIndent) isCommandState()          {}

// VimState tracks the Vim editor state.
type VimState struct {
	Mode         VimMode
	CommandState CommandState
}

// NewVimState creates a new Vim state (starts in insert mode).
func NewVimState() *VimState {
	return &VimState{
		Mode:         VimModeInsert,
		CommandState: CmdIdle{},
	}
}

func (v *VimState) resetCmd() { v.CommandState = CmdIdle{} }

// EnterInsert switches to insert mode.
func (v *VimState) EnterInsert() {
	v.Mode = VimModeInsert
	v.resetCmd()
}

// EnterNormal switches to normal mode.
func (v *VimState) EnterNormal() {
	v.Mode = VimModeNormal
	v.resetCmd()
}

// EnterVisual switches to visual mode.
func (v *VimState) EnterVisual() {
	v.Mode = VimModeVisual
	v.resetCmd()
}

// IsInsert returns true if in insert mode.
func (v *VimState) IsInsert() bool { return v.Mode == VimModeInsert }

// IsNormal returns true if in normal mode.
func (v *VimState) IsNormal() bool { return v.Mode == VimModeNormal }

// IsVisual returns true if in visual mode.
func (v *VimState) IsVisual() bool { return v.Mode == VimModeVisual }

func opFromKey(k rune) (Operator, bool) {
	switch k {
	case 'd':
		return OpDelete, true
	case 'c':
		return OpChange, true
	case 'y':
		return OpYank, true
	case '>':
		return OpIndent, true
	case '<':
		return OpOutdent, true
	default:
		return "", false
	}
}

func parseCount(digits string) int {
	if digits == "" {
		return 1
	}
	n := 0
	for _, r := range digits {
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return 1
	}
	return n
}

// HandleNormalKey advances the command parser state for one key in normal mode.
func (v *VimState) HandleNormalKey(key rune) {
	switch st := v.CommandState.(type) {
	case CmdIdle:
		switch {
		case unicode.IsDigit(key) && key != '0':
			v.CommandState = CmdCount{Digits: string(key)}
		case key == 'g':
			v.CommandState = CmdGPrefix{Count: 1}
		case key == 'f' || key == 'F' || key == 't' || key == 'T':
			v.CommandState = CmdFind{Count: 1}
		case key == 'r':
			v.CommandState = CmdReplace{Count: 1}
		default:
			if op, ok := opFromKey(key); ok {
				if op == OpIndent || op == OpOutdent {
					v.CommandState = CmdIndent{Dir: op, Count: 1}
				} else {
					v.CommandState = CmdOperator{Op: op, Count: 1}
				}
				return
			}
			v.CommandState = CmdIdle{}
		}
	case CmdCount:
		switch {
		case unicode.IsDigit(key):
			v.CommandState = CmdCount{Digits: st.Digits + string(key)}
		case key == 'g':
			v.CommandState = CmdGPrefix{Count: parseCount(st.Digits)}
		case key == 'f' || key == 'F' || key == 't' || key == 'T':
			v.CommandState = CmdFind{Count: parseCount(st.Digits)}
		case key == 'r':
			v.CommandState = CmdReplace{Count: parseCount(st.Digits)}
		default:
			if op, ok := opFromKey(key); ok {
				if op == OpIndent || op == OpOutdent {
					v.CommandState = CmdIndent{Dir: op, Count: parseCount(st.Digits)}
				} else {
					v.CommandState = CmdOperator{Op: op, Count: parseCount(st.Digits)}
				}
				return
			}
			v.CommandState = CmdIdle{}
		}
	case CmdOperator:
		switch {
		case unicode.IsDigit(key):
			v.CommandState = CmdOperatorCount{Op: st.Op, Count: st.Count, Digits: string(key)}
		case key == 'f' || key == 'F' || key == 't' || key == 'T':
			v.CommandState = CmdOperatorFind{Op: st.Op, Count: st.Count}
		case key == 'i':
			v.CommandState = CmdOperatorTextObj{Op: st.Op, Count: st.Count, Scope: TextObjInner}
		case key == 'a':
			v.CommandState = CmdOperatorTextObj{Op: st.Op, Count: st.Count, Scope: TextObjAround}
		case key == 'g':
			v.CommandState = CmdOperatorG{Op: st.Op, Count: st.Count}
		default:
			v.CommandState = CmdIdle{}
		}
	case CmdOperatorCount:
		switch {
		case unicode.IsDigit(key):
			v.CommandState = CmdOperatorCount{Op: st.Op, Count: st.Count, Digits: st.Digits + string(key)}
		case key == 'f' || key == 'F' || key == 't' || key == 'T':
			v.CommandState = CmdOperatorFind{Op: st.Op, Count: parseCount(st.Digits)}
		case key == 'i':
			v.CommandState = CmdOperatorTextObj{Op: st.Op, Count: parseCount(st.Digits), Scope: TextObjInner}
		case key == 'a':
			v.CommandState = CmdOperatorTextObj{Op: st.Op, Count: parseCount(st.Digits), Scope: TextObjAround}
		case key == 'g':
			v.CommandState = CmdOperatorG{Op: st.Op, Count: parseCount(st.Digits)}
		default:
			v.CommandState = CmdIdle{}
		}
	case CmdOperatorFind, CmdOperatorTextObj, CmdFind, CmdGPrefix, CmdOperatorG, CmdReplace, CmdIndent:
		// One more key completes these compound states.
		v.CommandState = CmdIdle{}
	default:
		v.CommandState = CmdIdle{}
	}
}


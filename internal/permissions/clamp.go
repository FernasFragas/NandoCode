package permissions

// strictness ranks modes from loosest (0) to strictest. A sub-agent may run in
// any mode at least as strict as its parent's, never a looser one.
//
// bubble ranks with default: it asks for everything and defers to the parent.
// dontAsk and plan both deny anything that would need a prompt; plan is the
// strictest because it also refuses classifier-approved-by-prompt writes.
func strictness(m Mode) int {
	switch m {
	case ModeBypass:
		return 0
	case ModeAcceptEdits:
		return 1
	case ModeAuto:
		return 2
	case ModeDefault, ModeBubble:
		return 3
	case ModeDontAsk:
		return 4
	case ModePlan:
		return 5
	default:
		return 3
	}
}

// ClampMode returns the permission mode a sub-agent may use given its parent's
// mode and the mode it asked for. The result is never looser than the parent's:
// an empty or unknown request inherits the parent's mode, and a request that is
// looser than the parent's is replaced by the parent's mode.
//
// When the parent's mode is itself empty it is treated as ModeDefault.
func ClampMode(parent, requested Mode) Mode {
	parent = parent.Normalize()
	if requested == "" {
		return parent
	}
	known := false
	switch requested {
	case ModeBypass, ModeDontAsk, ModeAuto, ModeAcceptEdits, ModeDefault, ModePlan, ModeBubble:
		known = true
	}
	if !known {
		return parent
	}
	if strictness(requested) < strictness(parent) {
		return parent
	}
	return requested
}

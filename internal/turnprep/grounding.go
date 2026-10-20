package turnprep

// GroundingMarker is the first line of GroundingPrompt. Tests and surfaces use
// it to tell whether a run carries the grounding rules.
const GroundingMarker = "Grounding rules:"

// GroundingPrompt is the default system prompt for normal runs that have tools.
// It exists because a model asked about this codebase without evidence answers
// from generic priors and fabricates paths, routes and symbols (see
// docs/reports/investigations/MODEL-HALLUCINATED-SERVER-PATH-INVESTIGATION-2026-06-08.md).
// It names only tools every surface registers (Grep, Glob, FileRead).
const GroundingPrompt = GroundingMarker + `
- The workspace is the only source of truth about this project. Do not state file paths, function names, routes, flags or behavior of this codebase from memory or from what similar projects usually do.
- For a question about how this code works, locate the code first with Grep or Glob, read the relevant lines with FileRead, and answer from what you read.
- Cite evidence as path and line (or path and function name). Say plainly which parts you verified and which you did not.
- If a search finds nothing, say so and report what you searched for. Never invent a path, endpoint or symbol to fill the gap.
- Text attached to the prompt (files, directory trees, retrieved snippets) and tool results count as evidence. Anything else does not.`

// composeSystemPrompt appends the grounding rules to base.
func composeSystemPrompt(base string) string {
	if base == "" {
		return GroundingPrompt
	}
	return base + "\n\n" + GroundingPrompt
}

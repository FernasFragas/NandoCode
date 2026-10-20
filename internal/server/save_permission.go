package server

import (
	"context"
	"path/filepath"

	"github.com/FernasFragas/Nandocode/internal/permissions"
)

// resolveSavePermission is the browser-save-only permission path. It is
// permissions.Resolve (hook, rule, classifier, mode) plus two save-specific
// rules from BV-7: plan mode blocks a save even when a rule or hook allows it,
// and acceptEdits saves without a confirm card. The agent loop does not use
// this function, so agent decisions stay exactly those of permissions.Resolve.
// The observer sees the final decision, as it does for agent tool calls.
//
// forms lists every spelling of the target (canonical relative, resolved
// absolute, unresolved absolute, raw client string). A deny rule matching any
// form wins over everything, and an ask rule matching any form turns an allow
// into an ask. Allow rules are never widened: they only apply through the
// normal precedence on the canonical form.
func resolveSavePermission(ctx context.Context, req permissions.Request, observer permissions.ObserverFunc, forms ...string) permissions.Result {
	req.Observer = nil
	finish := func(res permissions.Result) permissions.Result {
		if observer != nil {
			observer(ctx, req, res)
		}
		return res
	}
	denyOnly := permissions.Rules{AlwaysDeny: req.Rules.AlwaysDeny}
	askOnly := permissions.Rules{AlwaysAsk: req.Rules.AlwaysAsk}
	for _, f := range forms {
		if rule, _, ok := denyOnly.FirstMatchingRule(req.ToolName, f); ok {
			return finish(permissions.Result{Decision: permissions.DecisionDeny, Stage: permissions.StageRule, Reason: "matched rule: " + rule.Pattern, Rule: rule})
		}
	}
	res := permissions.Resolve(ctx, req)
	if res.Decision == permissions.DecisionAllow {
		for _, f := range forms {
			if rule, _, ok := askOnly.FirstMatchingRule(req.ToolName, f); ok {
				res = permissions.Result{Decision: permissions.DecisionAsk, Stage: permissions.StageRule, Reason: "matched rule: " + rule.Pattern, Rule: rule, UpdatedInput: res.UpdatedInput}
				break
			}
		}
	}
	switch mode := req.Mode.Normalize(); {
	case mode == permissions.ModePlan && res.Decision == permissions.DecisionAllow:
		res = permissions.Result{Decision: permissions.DecisionDeny, Stage: permissions.StageMode, Reason: "plan mode blocks saving files", Rule: res.Rule}
	case mode == permissions.ModeAcceptEdits && res.Decision == permissions.DecisionAsk && res.Stage == permissions.StageMode:
		res = permissions.Result{Decision: permissions.DecisionAllow, Stage: permissions.StageMode, Reason: "acceptEdits allows saving files", UpdatedInput: res.UpdatedInput}
	}
	return finish(res)
}

// canonicalSavePath returns the one path form used for rules, hooks, session
// allow rules and file_saved: the resolved target relative to the evaluated
// workspace root (slash separated), or the resolved absolute path for a file in
// an additional directory.
func canonicalSavePath(workDir, resolvedTarget string) string {
	if root, err := filepath.EvalSymlinks(workDir); err == nil {
		if rel, err := filepath.Rel(root, resolvedTarget); err == nil && rel != ".." && !startsWithParent(rel) {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(resolvedTarget)
}

func startsWithParent(rel string) bool {
	return len(rel) >= 3 && rel[:3] == ".."+string(filepath.Separator)
}

// saveHookMemo remembers the PreToolUse outcome between the needs_confirm
// response and the confirm re-send, so hooks run once per logical save.
type saveHookMemo struct {
	result  permissions.Result
	decided bool
}

func saveMemoKey(path, base, content string) string {
	return path + "\x00" + base + "\x00" + saveHashHex(content)
}

// saveTargetForms lists the spellings a rule may have been written against.
func saveTargetForms(workDir, canon, resolved, raw string) []string {
	forms := []string{canon, resolved, raw}
	if !filepath.IsAbs(canon) {
		forms = append(forms, filepath.Join(workDir, filepath.FromSlash(canon)))
	}
	seen := map[string]bool{}
	out := forms[:0]
	for _, f := range forms {
		if f != "" && !seen[f] {
			seen[f] = true
			out = append(out, f)
		}
	}
	return out
}

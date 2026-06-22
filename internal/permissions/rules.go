package permissions

// Source indicates where a permission rule originated.
type Source int

const (
	// SourcePolicy means the rule comes from system/policy.
	SourcePolicy Source = iota
	// SourceUser means the rule comes from the user's home or global config.
	SourceUser
	// SourceProject means the rule comes from project-level config.
	SourceProject
	// SourceLocal means the rule comes from local session state.
	SourceLocal
	// SourceCLI means the rule comes from CLI arguments.
	SourceCLI
	// SourceSession means the rule comes from the current session.
	SourceSession
)

// String returns a human-readable string for the source.
func (s Source) String() string {
	switch s {
	case SourcePolicy:
		return "policy"
	case SourceUser:
		return "user"
	case SourceProject:
		return "project"
	case SourceLocal:
		return "local"
	case SourceCLI:
		return "cli"
	case SourceSession:
		return "session"
	default:
		return "unknown"
	}
}

// Rule represents a single permission rule that matches tool calls by pattern.
type Rule struct {
	// Pattern is in the form "ToolName(arg-glob)" where arg-glob is matched
	// against the tool input's permission target.
	Pattern string
	// Source indicates where this rule originated.
	Source Source
}

// Rules holds three buckets of rules organized by decision type.
// Effective precedence during matching is: AlwaysDeny > AlwaysAsk > AlwaysAllow.
type Rules struct {
	// AlwaysAllow contains patterns that should be allowed regardless of mode.
	AlwaysAllow []Rule
	// AlwaysDeny contains patterns that should be denied regardless of mode.
	AlwaysDeny []Rule
	// AlwaysAsk contains patterns that should ask regardless of mode.
	AlwaysAsk []Rule
}

// Empty reports whether the rules set has no rules in any bucket.
func (r Rules) Empty() bool {
	return len(r.AlwaysAllow) == 0 && len(r.AlwaysDeny) == 0 && len(r.AlwaysAsk) == 0
}

// Merge combines two rule sets, preserving source provenance and duplicates.
// The function maintains stable source ordering: policy < user < project < local < cli < session.
// Decision precedence (deny > ask > allow) is implicit in matching order, not merge order.
func Merge(a, b Rules) Rules {
	// Separate rules by source and decision type for stable ordering.
	type keySource struct {
		decision Decision
		source   Source
	}
	
	// Collect all rules keyed by decision and source.
	allRules := make(map[keySource][]Rule)
	for _, rule := range a.AlwaysAllow {
		key := keySource{DecisionAllow, rule.Source}
		allRules[key] = append(allRules[key], rule)
	}
	for _, rule := range b.AlwaysAllow {
		key := keySource{DecisionAllow, rule.Source}
		allRules[key] = append(allRules[key], rule)
	}
	for _, rule := range a.AlwaysDeny {
		key := keySource{DecisionDeny, rule.Source}
		allRules[key] = append(allRules[key], rule)
	}
	for _, rule := range b.AlwaysDeny {
		key := keySource{DecisionDeny, rule.Source}
		allRules[key] = append(allRules[key], rule)
	}
	for _, rule := range a.AlwaysAsk {
		key := keySource{DecisionAsk, rule.Source}
		allRules[key] = append(allRules[key], rule)
	}
	for _, rule := range b.AlwaysAsk {
		key := keySource{DecisionAsk, rule.Source}
		allRules[key] = append(allRules[key], rule)
	}

	// Reconstruct into three buckets, preserving duplicates and source order.
	result := Rules{}
	sources := []Source{SourcePolicy, SourceUser, SourceProject, SourceLocal, SourceCLI, SourceSession}
	for _, src := range sources {
		if rules, ok := allRules[keySource{DecisionAllow, src}]; ok {
			result.AlwaysAllow = append(result.AlwaysAllow, rules...)
		}
	}
	for _, src := range sources {
		if rules, ok := allRules[keySource{DecisionAsk, src}]; ok {
			result.AlwaysAsk = append(result.AlwaysAsk, rules...)
		}
	}
	for _, src := range sources {
		if rules, ok := allRules[keySource{DecisionDeny, src}]; ok {
			result.AlwaysDeny = append(result.AlwaysDeny, rules...)
		}
	}

	return result
}

// FirstMatchingRule checks each decision bucket in precedence order
// and returns the first matching rule and its decision, or (nil, "", false) if none match.
func (r Rules) FirstMatchingRule(toolName, target string) (*Rule, Decision, bool) {
	for i := range r.AlwaysDeny {
		if matches(r.AlwaysDeny[i].Pattern, toolName, target) {
			return &r.AlwaysDeny[i], DecisionDeny, true
		}
	}
	for i := range r.AlwaysAsk {
		if matches(r.AlwaysAsk[i].Pattern, toolName, target) {
			return &r.AlwaysAsk[i], DecisionAsk, true
		}
	}
	for i := range r.AlwaysAllow {
		if matches(r.AlwaysAllow[i].Pattern, toolName, target) {
			return &r.AlwaysAllow[i], DecisionAllow, true
		}
	}
	return nil, "", false
}

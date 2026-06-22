package picker

import (
	"sort"
	"strings"

	"github.com/FernasFragas/nandocodego/internal/commands"
)

// CommandProvider suggests slash commands.
type CommandProvider struct {
	registry *commands.Registry
}

var commandDetail = map[string]string{
	"help":            "session",
	"clear":           "session",
	"exit":            "session",
	"model":           "model",
	"models":          "model",
	"pull":            "model",
	"memory":          "memory",
	"context":         "context",
	"hooks":           "hooks",
	"permissions":     "permissions",
	"skills":          "skills",
	"cost":            "usage",
	"trace":           "trace",
	"prompt":          "prompt",
	"init":            "setup",
	"agents":          "agents",
	"queue":           "queue",
	"compact":         "context",
	"refresh-index":   "index",
	"analyze-project": "analysis",
	"checkpoint":      "checkpoint",
	"bg":              "run",
	"btw":             "run",
}

func NewCommandProvider(reg *commands.Registry) *CommandProvider {
	return &CommandProvider{registry: reg}
}

func (p *CommandProvider) Suggest(query string, limit int) []Suggestion {
	if p == nil || p.registry == nil || limit <= 0 {
		return nil
	}
	names := p.registry.Names()
	if len(names) == 0 {
		return nil
	}
	q := strings.ToLower(strings.TrimSpace(query))
	sugs := make([]Suggestion, 0, len(names))
	for _, name := range names {
		display := "/" + name
		if q != "" && !strings.Contains(strings.ToLower(name), q) {
			continue
		}
		detail := commandDetail[name]
		if detail == "" {
			detail = "cmd"
		}
		score := 0.0
		if q == "" {
			score = 1
		} else {
			if strings.HasPrefix(strings.ToLower(name), q) {
				score += 20
			}
			if fuzzy, matches, ok := fuzzyScore(q, strings.ToLower(name)); ok {
				score += fuzzy
				sugs = append(sugs, Suggestion{
					Display:    display,
					Insert:     name,
					Detail:     detail,
					IsDir:      false,
					MatchRunes: shiftMatches(matches, 1),
					Score:      score,
				})
				continue
			}
		}
		sugs = append(sugs, Suggestion{
			Display: display,
			Insert:  name,
			Detail:  detail,
			Score:   score,
		})
	}
	sort.Slice(sugs, func(i, j int) bool {
		if sugs[i].Score == sugs[j].Score {
			return sugs[i].Display < sugs[j].Display
		}
		return sugs[i].Score > sugs[j].Score
	})
	if len(sugs) > limit {
		sugs = sugs[:limit]
	}
	return sugs
}

func shiftMatches(in []int, n int) []int {
	if len(in) == 0 || n == 0 {
		return in
	}
	out := make([]int, len(in))
	for i := range in {
		out[i] = in[i] + n
	}
	return out
}

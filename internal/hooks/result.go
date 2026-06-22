package hooks

import (
	"encoding/json"
	"strings"
)

type Decision string

const (
	DecisionNone  Decision = ""
	DecisionAllow Decision = "allow"
	DecisionDeny  Decision = "deny"
	DecisionAsk   Decision = "ask"
)

func (d Decision) Valid() bool {
	switch d {
	case DecisionNone, DecisionAllow, DecisionDeny, DecisionAsk:
		return true
	default:
		return false
	}
}

type Result struct {
	Decision          Decision        `json:"decision,omitempty"`
	Reason            string          `json:"reason,omitempty"`
	UpdatedInput      json.RawMessage `json:"updated_input,omitempty"`
	AdditionalContext string          `json:"additional_context,omitempty"`
	Warning           string          `json:"warning,omitempty"`
}

func (r Result) Decisive() bool {
	return r.Decision == DecisionAllow || r.Decision == DecisionDeny || r.Decision == DecisionAsk
}

func aggregate(results []Result) Result {
	var out Result
	for _, r := range results {
		if strings.TrimSpace(r.Warning) != "" {
			if out.Warning == "" {
				out.Warning = r.Warning
			} else {
				out.Warning += "; " + r.Warning
			}
		}
		if strings.TrimSpace(r.Reason) != "" && out.Reason == "" {
			out.Reason = r.Reason
		}
		switch r.Decision {
		case DecisionDeny:
			out.Decision = DecisionDeny
			if r.Reason != "" {
				out.Reason = r.Reason
			}
			return out
		case DecisionAsk:
			if out.Decision != DecisionDeny {
				out.Decision = DecisionAsk
				if r.Reason != "" {
					out.Reason = r.Reason
				}
			}
		case DecisionAllow:
			if out.Decision == DecisionNone {
				out.Decision = DecisionAllow
				if r.Reason != "" {
					out.Reason = r.Reason
				}
			}
		}
	}
	return out
}

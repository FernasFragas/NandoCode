package modelruntime

import (
	"context"
	"errors"

	"github.com/FernasFragas/Nandocode/internal/llm"
)

// ActivateResult is a completed model switch plus the refreshed model limits.
type ActivateResult struct {
	SwitchResult
	// Limits holds the limits derived from the new model's details. It is only
	// meaningful when HasLimits is true.
	Limits    llm.ModelLimits
	HasLimits bool
	// LimitsErr is why the limits could not be refreshed. It is not fatal: the
	// switch itself succeeded and the caller keeps its previous limits.
	LimitsErr error
}

// Activate switches to the requested model and refreshes its limits. It never
// prompts: a cloud model without a stored credential returns
// ErrCredentialRequired and leaves the runtime unchanged, so a caller (such as
// the browser) can collect the key and call Activate again.
func (s *Service) Activate(ctx context.Context, requested string) (ActivateResult, error) {
	if s == nil {
		return ActivateResult{}, errors.New("model runtime service unavailable")
	}
	res, err := s.Switch(ctx, SwitchOptions{RequestedModel: requested, AllowPrompt: false})
	if err != nil {
		return ActivateResult{}, err
	}
	out := ActivateResult{SwitchResult: res}
	details, err := s.Runtime.ShowModel(ctx, res.Resolved.Model)
	if err != nil {
		out.LimitsErr = err
		return out, nil
	}
	out.Limits = llm.ComputeLimits(details)
	out.HasLimits = true
	return out, nil
}

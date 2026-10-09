package openrouter

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/stdevmac/payverge/backend/internal/llm"
)

const (
	maxRetries              = 2
	retryBaseDelay          = 250 * time.Millisecond
	retryCapDelay           = 2 * time.Second
	budgetAccountingTimeout = 5 * time.Second
)

// Generate runs doGenerate with a full-jitter retry loop and fires the observer
// exactly once on the terminal outcome (success or error).
//
// When a CallBudget is installed, the call is wrapped as:
//
//	reserve conservative max → provider call → finalize actual | release on error
//
// so spend caps hold across replicas. Budget decisions never use DailyCostRollup.
func (p *Provider) Generate(ctx context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	start := time.Now()

	// Privacy class is mandatory and fail-closed before any reservation or network.
	if err := llm.ApplyFeaturePrivacy(&req); err != nil {
		return nil, err
	}
	if err := req.ValidatePrivacy(); err != nil {
		return nil, err
	}

	var reservationID string
	if p.budget != nil {
		id, applied, rerr := p.budget.ReserveForRequest(ctx, req)
		if rerr != nil {
			// Surface budget / unpriced failures before any provider spend.
			if p.observer != nil {
				ci := llm.CallInfo{
					Feature:    req.Feature,
					Model:      req.Model,
					Latency:    time.Since(start),
					Err:        rerr,
					BusinessID: req.BusinessID,
				}
				llm.AnnotateCost(&ci, 0)
				p.observer(ci)
			}
			return nil, rerr
		}
		if applied {
			reservationID = id
		}
	}

	resp, err := p.generateWithRetry(ctx, req)

	if reservationID != "" && p.budget != nil {
		// Provider outcome accounting must outlive a request deadline that fires
		// just as the upstream response arrives. Preserve trace/value context, but
		// detach cancellation and impose a short independent bound so a stuck DB
		// can never hold the request goroutine indefinitely.
		accountingCtx, cancelAccounting := budgetAccountingContext(ctx)
		defer cancelAccounting()
		if err != nil {
			if relErr := p.budget.ReleaseRequest(accountingCtx, reservationID); relErr != nil {
				slog.Warn("llm_budget_release_failed",
					"reservation_id", reservationID,
					"error", relErr.Error(),
				)
			}
		} else {
			if finErr := p.budget.FinalizeRequest(accountingCtx, reservationID, resp, req.Model); finErr != nil {
				slog.Error("llm_budget_finalize_failed",
					"reservation_id", reservationID,
					"error", finErr.Error(),
				)
				// Provider spend already happened. Never report success when its
				// durable accounting failed; the stale-reservation reconciler keeps
				// the conservative hold fail-closed until it can finalize it.
				err = finErr
			}
		}
	}

	if p.observer != nil {
		ci := llm.CallInfo{
			Feature: req.Feature,
			Model:   req.Model,
			Latency: time.Since(start),
			Err:     err,
		}
		if resp != nil {
			ci.ServedModel = resp.Model
			ci.InputTokens = resp.Usage.PromptTokens
			ci.OutputTokens = resp.Usage.CompletionTokens
		}
		ci.BusinessID = req.BusinessID
		cached := 0
		if resp != nil {
			cached = resp.Usage.CachedTokens
		}
		llm.AnnotateCost(&ci, cached)
		p.observer(ci)
	}
	return resp, err
}

func budgetAccountingContext(requestCtx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(requestCtx), budgetAccountingTimeout)
}

// generateWithRetry runs doGenerate with the full-jitter retry loop.
func (p *Provider) generateWithRetry(ctx context.Context, req llm.GenerateRequest) (*llm.Response, error) {
	var resp *llm.Response
	var err error
	for attempt := 0; ; attempt++ {
		resp, err = p.doGenerate(ctx, req)
		if err == nil {
			return resp, nil
		}
		if attempt >= maxRetries || !isRetryable(err) {
			return nil, err
		}
		if serr := p.sleep(ctx, p.backoff(attempt)); serr != nil {
			return nil, err
		}
	}
}

// isRetryable reports whether err is a transient class worth retrying.
func isRetryable(err error) bool {
	if errors.Is(err, llm.ErrRateLimited) {
		return true
	}
	if !errors.Is(err, llm.ErrUpstream) {
		return false
	}
	var apiErr *llm.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status >= 500
	}
	return true
}

// backoff returns the full-jitter delay for the given zero-based attempt.
func (p *Provider) backoff(attempt int) time.Duration {
	ceiling := retryBaseDelay << attempt
	if ceiling > retryCapDelay || ceiling <= 0 {
		ceiling = retryCapDelay
	}
	return time.Duration(p.randFloat() * float64(ceiling))
}

// ctxSleep is the production sleeper: it waits d or returns ctx.Err() early.
func ctxSleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

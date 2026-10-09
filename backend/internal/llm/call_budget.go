package llm

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Daily ledger scopes (ai_daily_spend.feature_scope). Business-attributed
// calls charge their audience scope for that business AND the instance-wide
// global scope. Guest waiter turns additionally charge the instance-wide guest
// pool, a sub-cap inside global that every venue's guests share. Owner/staff
// calls never draw from the guest pool, so the remainder of global stays
// reserved for owners and staff.
const (
	// BudgetScopeOwner is the per-business owner/staff scope. It keeps the
	// historical "" value so existing ledger rows and gates stay valid.
	BudgetScopeOwner = ""
	// BudgetScopeGuest is the per-business guest-facing scope (AI waiter on
	// web and WhatsApp, and the guardrail calls that screen it).
	BudgetScopeGuest = "guest"
	// BudgetScopeGlobal is the instance-wide ceiling across every lane
	// (business 0). It bounds total provider spend for a self-hosted install.
	BudgetScopeGlobal = "global"
	// BudgetScopeGuestPool is the instance-wide sub-cap (business 0) shared by
	// guest waiter turns (web and WhatsApp) at every business. It sits inside
	// global; each business's guests may only spend a fraction of it (see
	// CallBudget.GuestCapMicro), so no single venue can close guest AI at the
	// others.
	BudgetScopeGuestPool = "guest_pool"

	// BudgetAudienceGuest / BudgetAudienceOwner are GenerateRequest.BudgetAudience values.
	BudgetAudienceGuest = "guest"
	BudgetAudienceOwner = "owner"
)

// CallBudget wraps BudgetStore with per-request reserve → finalize/release
// policy for the OpenRouter provider path. It is the enforcement point;
// DailyCostRollup is metrics-only.
type CallBudget struct {
	Store *BudgetStore

	// BusinessCapMicro is the per-business daily ceiling of the owner scope
	// (""), and of the guest scope ("guest") when GuestCapMicro is 0. 0
	// disables.
	BusinessCapMicro int64
	// GuestCapMicro, when > 0, is the per-business ceiling of the guest scope
	// instead of BusinessCapMicro: each venue's share of the guest pool.
	GuestCapMicro int64
	// GlobalCapMicro is the instance-wide daily ceiling (business_id=0,
	// feature_scope="global") every reservation must also fit under. 0 disables.
	GlobalCapMicro int64
	// GuestPoolCapMicro is the instance-wide sub-cap (business_id=0,
	// feature_scope="guest_pool") that guest reservations at every business
	// must also fit under. Owner/staff calls never charge it.
	// 0 disables.
	GuestPoolCapMicro int64

	ReservationTTL time.Duration
}

// NewCallBudget builds a reserver. A nil store disables all reservations.
func NewCallBudget(store *BudgetStore, businessCapMicro int64) *CallBudget {
	return &CallBudget{
		Store:            store,
		BusinessCapMicro: businessCapMicro,
		ReservationTTL:   DefaultReservationTTL,
	}
}

// WithGlobalCap sets the instance-wide daily ceiling in micro-USD.
func (b *CallBudget) WithGlobalCap(globalCapMicro int64) *CallBudget {
	if b == nil {
		return nil
	}
	b.GlobalCapMicro = globalCapMicro
	return b
}

// WithGuestPoolCap sets the instance-wide guest pool sub-cap in micro-USD.
func (b *CallBudget) WithGuestPoolCap(poolCapMicro int64) *CallBudget {
	if b == nil {
		return nil
	}
	b.GuestPoolCapMicro = poolCapMicro
	return b
}

// WithGuestCap sets the per-business guest-scope ceiling in micro-USD
// (0 = use BusinessCapMicro).
func (b *CallBudget) WithGuestCap(guestCapMicro int64) *CallBudget {
	if b == nil {
		return nil
	}
	b.GuestCapMicro = guestCapMicro
	return b
}

// Active reports whether any cap is enforced via a live store.
func (b *CallBudget) Active() bool {
	return b != nil && b.Store != nil &&
		(b.BusinessCapMicro > 0 || b.GuestCapMicro > 0 ||
			b.GlobalCapMicro > 0 || b.GuestPoolCapMicro > 0)
}

// ReserveForRequest holds a conservative max cost before the provider call.
// Returns applied=false when no budget applies (uncapped path).
func (b *CallBudget) ReserveForRequest(ctx context.Context, req GenerateRequest) (reservationID string, applied bool, err error) {
	if !b.Active() {
		return "", false, nil
	}
	// Validate restaurant attribution before touching the store so a malformed
	// restaurant request fails with the stable contract error even if the
	// durable store is temporarily unavailable.
	if req.BusinessID == 0 && businessScopedFeature(req.Feature) {
		return "", false, ErrMissingBusinessID
	}
	shutdown, shutdownErr := b.Store.UnpricedModelShutdownActive(ctx)
	if shutdownErr != nil {
		return "", false, shutdownErr
	}
	if shutdown {
		return "", false, ErrUnpricedModel
	}
	scopes := b.scopesFor(req)
	if len(scopes) == 0 {
		return "", false, nil
	}

	model := req.Model
	if !budgetPriceKnown(model) {
		return "", false, ErrUnpricedModel
	}
	inputTok := EstimateRequestInputTokens(req)
	maxOut := req.MaxTokens
	if maxOut <= 0 {
		maxOut = DefaultConservativeMaxOutputTokens
	}
	conservative, priced := EstimateCostMicroUSD(model, inputTok, 0, maxOut)
	if !priced {
		return "", false, ErrUnpricedModel
	}
	// Zero-cost priced models still create a reservation row for auditability
	// but never trip the cap on amount alone.
	now := time.Now().UTC()
	ttl := b.ReservationTTL
	if ttl <= 0 {
		ttl = DefaultReservationTTL
	}
	id := uuid.New().String()
	err = b.Store.Reserve(ctx, ReserveParams{
		ReservationID:        id,
		BusinessID:           scopes[0].BusinessID,
		FeatureScope:         scopes[0].FeatureScope,
		UsageDate:            UTCDate(now),
		CapMicroUSD:          scopes[0].CapMicroUSD,
		ExtraScopes:          scopes[1:],
		ConservativeMicroUSD: conservative,
		Model:                model,
		InputTokens:          inputTok,
		MaxOutputTokens:      maxOut,
		ExpiresAt:            now.Add(ttl),
		Now:                  now,
	})
	if err != nil {
		return "", false, err
	}
	return id, true, nil
}

// FinalizeRequest records actual spend after a successful provider response.
// An unpriced served model atomically records the conservative charge and sets
// a durable cluster-wide shutdown before reconciling the daily ledger.
func (b *CallBudget) FinalizeRequest(ctx context.Context, reservationID string, resp *Response, reqModel string) error {
	if b == nil || b.Store == nil || reservationID == "" {
		return nil
	}
	now := time.Now().UTC()
	served := ""
	inTok, outTok := 0, 0
	if resp != nil {
		served = resp.Model
		inTok = resp.Usage.PromptTokens
		outTok = resp.Usage.CompletionTokens
	}
	if served == "" {
		served = reqModel
	}

	actual, priced := EstimateCostMicroUSD(served, inTok, 0, outTok)
	if !priced {
		// Unknown served model: latch fail-closed so no further calls proceed,
		// and charge a best-effort actual — re-price with the (priced) requested
		// model and actual tokens as a lower bound, falling back to 0 only if
		// that is also unpriced. Finalize releases the conservative hold either
		// way, so budget headroom is never permanently pinned by this call.
		slog.Error("llm_unpriced_served_model",
			"served_model", served,
			"requested_model", reqModel,
			"reservation_id", reservationID,
			"action", "fail_closed",
		)
		if err := b.Store.RecordUnpricedConsumed(ctx, FinalizeParams{
			ReservationID: reservationID, ServedModel: served,
			InputTokens: inTok, OutputTokens: outTok, Now: now,
		}); err != nil {
			return err
		}
		return b.Store.ReconcileConsumed(ctx, reservationID, now)
	}

	return b.Store.Finalize(ctx, FinalizeParams{
		ReservationID:  reservationID,
		ActualMicroUSD: actual,
		ServedModel:    served,
		InputTokens:    inTok,
		OutputTokens:   outTok,
		Now:            now,
	})
}

// ReleaseRequest returns the reserved headroom after a provider failure.
func (b *CallBudget) ReleaseRequest(ctx context.Context, reservationID string) error {
	if b == nil || b.Store == nil || reservationID == "" {
		return nil
	}
	return b.Store.Release(ctx, reservationID, time.Now().UTC())
}

// scopesFor returns every ledger scope the request must fit under, primary
// first: the business's audience scope, then the
// guest pool for guest turns, then the instance-wide global scope. Empty means
// the call is uncapped.
func (b *CallBudget) scopesFor(req GenerateRequest) []ScopeCap {
	scopes := make([]ScopeCap, 0, 3)
	guest := BudgetScopeFor(req) == BudgetScopeGuest
	if req.BusinessID != 0 {
		capMicro := b.BusinessCapMicro
		if guest && b.GuestCapMicro > 0 {
			capMicro = b.GuestCapMicro
		}
		if capMicro > 0 {
			scopes = append(scopes, ScopeCap{BusinessID: req.BusinessID, FeatureScope: BudgetScopeFor(req), CapMicroUSD: capMicro})
		}
	}
	if b.GuestPoolCapMicro > 0 && guest {
		scopes = append(scopes, ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGuestPool, CapMicroUSD: b.GuestPoolCapMicro})
	}
	if b.GlobalCapMicro > 0 {
		scopes = append(scopes, ScopeCap{BusinessID: 0, FeatureScope: BudgetScopeGlobal, CapMicroUSD: b.GlobalCapMicro})
	}
	return scopes
}

// BudgetScopeFor maps a business-attributed request to its per-business
// ledger scope (guest vs owner).
func BudgetScopeFor(req GenerateRequest) string {
	switch req.BudgetAudience {
	case BudgetAudienceGuest:
		return BudgetScopeGuest
	case BudgetAudienceOwner:
		return BudgetScopeOwner
	}
	if guestFeature(req.Feature) {
		return BudgetScopeGuest
	}
	return BudgetScopeOwner
}

func guestFeature(feature string) bool {
	return feature == "waiter" || feature == "waiter_whatsapp"
}

func businessScopedFeature(feature string) bool {
	switch feature {
	case "waiter", "waiter_whatsapp", "director", "wizard", "extraction", "image", "marketing", "ops_assistant", "guardrail":
		return true
	default:
		return false
	}
}

// ValidateConfiguredModelsPriced ensures every primary and fallback model in
// cfg has a checked-in price entry. Unpriced models fail closed (startup hard
// failure or gate error).
func ValidateConfiguredModelsPriced(cfg ModelConfig) error {
	seen := map[string]struct{}{}
	check := func(model string) error {
		model = trimModel(model)
		if model == "" {
			return nil
		}
		if _, dup := seen[model]; dup {
			return nil
		}
		seen[model] = struct{}{}
		if !ModelPriced(model) {
			return fmtUnpriced(model)
		}
		return nil
	}
	for _, m := range []string{cfg.Chat, cfg.Menu, cfg.Image, cfg.Director, cfg.Guardrail} {
		if err := check(m); err != nil {
			return err
		}
	}
	for _, list := range [][]string{
		cfg.ChatFallbacks, cfg.MenuFallbacks, cfg.ImageFallbacks,
		cfg.DirectorFallbacks, cfg.GuardrailFallbacks,
	} {
		for _, m := range list {
			if err := check(m); err != nil {
				return err
			}
		}
	}
	return nil
}

func trimModel(m string) string { return strings.TrimSpace(m) }

func fmtUnpriced(model string) error {
	return &unpricedModelError{model: model}
}

// unpricedModelError wraps ErrUnpricedModel with the model name while remaining
// errors.Is-compatible with ErrUnpricedModel.
type unpricedModelError struct {
	model string
}

func (e *unpricedModelError) Error() string {
	return "llm: configured model has no price entry: " + e.model
}

func (e *unpricedModelError) Unwrap() error { return ErrUnpricedModel }

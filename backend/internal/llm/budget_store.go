package llm

import (
	"context"
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Micro-USD: 1 USD = 1_000_000 micro-USD. All durable budget math uses int64
// micro-USD; never floats for stored or compared amounts.
const MicroUSDPerUSD int64 = 1_000_000

// DefaultConservativeMaxOutputTokens is used when GenerateRequest.MaxTokens is 0
// so the pre-call reservation still prices a finite worst-case completion.
const DefaultConservativeMaxOutputTokens = 8192

// DefaultReservationTTL bounds how long a reserved amount may hold budget
// before ExpireStale recovers it (provider hang / crashed replica).
const DefaultReservationTTL = 2 * time.Minute

// Reservation status values (persisted).
const (
	ReservationReserved  = "reserved"
	ReservationConsumed  = "consumed"
	ReservationFinalized = "finalized"
	ReservationReleased  = "released"
	ReservationExpired   = "expired"
)

// ErrBudgetExceeded is returned when a reservation would push finalized+reserved
// past the configured daily cap for (business, feature_scope, UTC date).
var ErrBudgetExceeded = errors.New("llm: daily budget exceeded")

// ErrUnpricedModel is returned when a configured or served model has no entry
// in the checked-in pricing table (fail-closed).
var ErrUnpricedModel = errors.New("llm: model has no price entry")

// ErrReservationNotFound is returned when Finalize/Release target an unknown id.
var ErrReservationNotFound = errors.New("llm: spend reservation not found")

// ErrMissingBusinessID prevents a restaurant-owned call from bypassing the
// per-business cap because a caller forgot its tenant budget key.
var ErrMissingBusinessID = errors.New("llm: business-scoped request missing business id")

// DollarsToMicroUSD converts a dollar amount to micro-USD integers (rounded).
func DollarsToMicroUSD(dollars float64) int64 {
	if dollars <= 0 {
		return 0
	}
	return int64(math.Round(dollars * float64(MicroUSDPerUSD)))
}

// MicroUSDToDollars converts micro-USD integers to a float dollar amount for
// metrics/logging only — never use the float for authorization math.
func MicroUSDToDollars(micro int64) float64 {
	return float64(micro) / float64(MicroUSDPerUSD)
}

// UTCDate returns the calendar date of t in UTC (time truncated to midnight UTC).
func UTCDate(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// EstimateCostMicroUSD prices a call in micro-USD using the checked-in table.
// ok is false when the model is unpriced (caller must fail closed), unless
// SetUnpricedModelsFree(true) is active, in which case unpriced models cost $0.
func EstimateCostMicroUSD(model string, inputTokens, cachedInput, outputTokens int) (int64, bool) {
	if !ModelPriced(model) {
		if unpricedModelsFree.Load() {
			return 0, true
		}
		return 0, false
	}
	usd := EstimateCostUSD(model, inputTokens, cachedInput, outputTokens)
	return DollarsToMicroUSD(usd), true
}

// EstimateRequestInputTokens returns a conservative (overestimate) input token
// count for pre-call reservation. No tokenizer ships with the binary, so we
// use a chars/2 heuristic that over-counts English text.
func EstimateRequestInputTokens(req GenerateRequest) int {
	n := len(req.System)
	for _, m := range req.Messages {
		n += len(m.Text)
		n += len(m.Images) * 1024 // multimodal: fixed conservative addend per image
		for _, tc := range m.ToolCalls {
			n += len(tc.Name) + 64
		}
	}
	for _, tool := range req.Tools {
		n += len(tool.Name) + len(tool.Description) + 128
	}
	// ~2 chars/token lower bound ⇒ overestimate tokens.
	tok := (n + 1) / 2
	if tok < 1 {
		tok = 1
	}
	return tok
}

// aiDailySpend is the durable per-(business, feature_scope, UTC day) ledger row.
type aiDailySpend struct {
	ID                int64     `gorm:"primaryKey;column:id"`
	BusinessID        uint      `gorm:"column:business_id;not null"`
	FeatureScope      string    `gorm:"column:feature_scope;not null;default:''"`
	UsageDate         time.Time `gorm:"column:usage_date;type:date;not null"`
	FinalizedMicroUSD int64     `gorm:"column:finalized_micro_usd;not null;default:0"`
	ReservedMicroUSD  int64     `gorm:"column:reserved_micro_usd;not null;default:0"`
	CreatedAt         time.Time `gorm:"column:created_at"`
	UpdatedAt         time.Time `gorm:"column:updated_at"`
}

func (aiDailySpend) TableName() string { return "ai_daily_spend" }

// aiSpendReservation is one conservative hold against the daily ledger.
type aiSpendReservation struct {
	ID                   string    `gorm:"primaryKey;column:id;type:uuid"`
	BusinessID           uint      `gorm:"column:business_id;not null"`
	FeatureScope         string    `gorm:"column:feature_scope;not null;default:''"`
	UsageDate            time.Time `gorm:"column:usage_date;type:date;not null"`
	ConservativeMicroUSD int64     `gorm:"column:conservative_micro_usd;not null"`
	ActualMicroUSD       *int64    `gorm:"column:actual_micro_usd"`
	Status               string    `gorm:"column:status;not null"`
	ExpiresAt            time.Time `gorm:"column:expires_at;not null"`
	Model                string    `gorm:"column:model;not null;default:''"`
	InputTokens          int       `gorm:"column:input_tokens;not null;default:0"`
	OutputTokens         int       `gorm:"column:output_tokens;not null;default:0"`
	MaxOutputTokens      int       `gorm:"column:max_output_tokens;not null;default:0"`
	ServedModel          string    `gorm:"column:served_model;not null;default:''"`
	// ParentReservationID links an extra-scope hold (e.g. the instance-wide
	// "global" scope) to the primary reservation the provider finalizes by id.
	// NULL on primary rows.
	ParentReservationID *string   `gorm:"column:parent_reservation_id;type:uuid"`
	CreatedAt           time.Time `gorm:"column:created_at"`
	UpdatedAt           time.Time `gorm:"column:updated_at"`
}

func (aiSpendReservation) TableName() string { return "ai_spend_reservations" }

type aiBudgetControl struct {
	ID                    int16     `gorm:"primaryKey;column:id"`
	UnpricedModelShutdown bool      `gorm:"column:unpriced_model_shutdown;not null;default:false"`
	ShutdownReason        string    `gorm:"column:shutdown_reason;not null;default:''"`
	UpdatedAt             time.Time `gorm:"column:updated_at"`
}

func (aiBudgetControl) TableName() string { return "ai_budget_controls" }

// BudgetStore is a Postgres-backed, replica-safe AI spend ledger.
type BudgetStore struct {
	db *gorm.DB
}

// NewBudgetStore constructs a store over an existing *gorm.DB (shared with the app).
// A nil db yields a store that errors on every operation (callers should not wire one).
func NewBudgetStore(db *gorm.DB) *BudgetStore {
	return &BudgetStore{db: db}
}

// ScopeCap is one daily ledger scope a reservation must fit under.
type ScopeCap struct {
	BusinessID   uint
	FeatureScope string
	CapMicroUSD  int64
}

// ReserveParams is the input to Reserve.
type ReserveParams struct {
	ReservationID string // UUID; generated if empty
	BusinessID    uint
	FeatureScope  string // BudgetScopeOwner / BudgetScopeGuest / BudgetScopeGuestPool / BudgetScopeGlobal
	UsageDate     time.Time
	CapMicroUSD   int64
	// ExtraScopes are additional (business, scope) ceilings the same hold must
	// fit under, checked atomically with the primary scope — e.g. the
	// instance-wide global scope. Each becomes a child reservation row.
	ExtraScopes          []ScopeCap
	ConservativeMicroUSD int64
	Model                string
	InputTokens          int
	MaxOutputTokens      int
	ExpiresAt            time.Time
	Now                  time.Time // testability; defaults to time.Now
}

// FinalizeParams is the input to Finalize after a successful provider call.
type FinalizeParams struct {
	ReservationID  string
	ActualMicroUSD int64
	ServedModel    string
	InputTokens    int
	OutputTokens   int
	Now            time.Time
}

// Reserve atomically holds ConservativeMicroUSD against the daily cap.
// Idempotent on ReservationID: a second Reserve with the same key is a no-op
// success when the prior row is still reserved or finalized.
func (s *BudgetStore) Reserve(ctx context.Context, p ReserveParams) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("llm: budget store not configured")
	}
	if p.ConservativeMicroUSD < 0 {
		return fmt.Errorf("llm: conservative amount must be non-negative")
	}
	if p.CapMicroUSD <= 0 {
		return fmt.Errorf("llm: cap must be positive for reservation")
	}
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	usageDate := p.UsageDate
	if usageDate.IsZero() {
		usageDate = UTCDate(now)
	} else {
		usageDate = UTCDate(usageDate)
	}
	expiresAt := p.ExpiresAt
	if expiresAt.IsZero() {
		expiresAt = now.Add(DefaultReservationTTL)
	}
	id := p.ReservationID
	if id == "" {
		id = uuid.New().String()
	}
	scopes, err := reservationScopes(p)
	if err != nil {
		return err
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Idempotency: existing reservation with same key.
		var existing aiSpendReservation
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", id).
			First(&existing).Error
		if err == nil {
			switch existing.Status {
			case ReservationReserved, ReservationFinalized:
				return nil
			case ReservationReleased, ReservationExpired:
				// Key reuse after terminal state is not supported.
				return fmt.Errorf("llm: reservation %s already %s", id, existing.Status)
			default:
				return fmt.Errorf("llm: reservation %s has unknown status %q", id, existing.Status)
			}
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		// Lock every scope's daily row in canonical (business_id, scope)
		// order so concurrent multi-scope reservations never deadlock, then
		// check all ceilings before touching any of them.
		dailies := make([]*aiDailySpend, len(scopes))
		for i, sc := range scopes {
			daily, err := lockOrCreateDaily(tx, sc.BusinessID, sc.FeatureScope, usageDate, now)
			if err != nil {
				return err
			}
			if daily.FinalizedMicroUSD+daily.ReservedMicroUSD+p.ConservativeMicroUSD > sc.CapMicroUSD {
				return fmt.Errorf("%w (business %d scope %q)", ErrBudgetExceeded, sc.BusinessID, scopeLabel(sc.FeatureScope))
			}
			dailies[i] = daily
		}
		// Every row is already locked: one statement increments all scopes
		// (was one full-row Save per scope).
		dailyIDs := make([]int64, len(dailies))
		for i, daily := range dailies {
			dailyIDs[i] = daily.ID
		}
		if err := tx.Exec(
			`UPDATE ai_daily_spend SET reserved_micro_usd = reserved_micro_usd + ?, updated_at = ? WHERE id IN ?`,
			p.ConservativeMicroUSD, now, dailyIDs,
		).Error; err != nil {
			return err
		}

		rows := make([]aiSpendReservation, 0, len(scopes))
		rows = append(rows, aiSpendReservation{
			ID:                   id,
			BusinessID:           p.BusinessID,
			FeatureScope:         p.FeatureScope,
			UsageDate:            usageDate,
			ConservativeMicroUSD: p.ConservativeMicroUSD,
			Status:               ReservationReserved,
			ExpiresAt:            expiresAt,
			Model:                p.Model,
			InputTokens:          p.InputTokens,
			MaxOutputTokens:      p.MaxOutputTokens,
			CreatedAt:            now,
			UpdatedAt:            now,
		})
		for _, sc := range scopes {
			if sc.BusinessID == p.BusinessID && sc.FeatureScope == p.FeatureScope {
				continue // primary row above
			}
			parent := id
			rows = append(rows, aiSpendReservation{
				ID:                   uuid.New().String(),
				BusinessID:           sc.BusinessID,
				FeatureScope:         sc.FeatureScope,
				UsageDate:            usageDate,
				ConservativeMicroUSD: p.ConservativeMicroUSD,
				Status:               ReservationReserved,
				ExpiresAt:            expiresAt,
				Model:                p.Model,
				InputTokens:          p.InputTokens,
				MaxOutputTokens:      p.MaxOutputTokens,
				ParentReservationID:  &parent,
				CreatedAt:            now,
				UpdatedAt:            now,
			})
		}
		return tx.Create(&rows).Error
	})
}

// reservationScopes returns the primary scope plus ExtraScopes, validated and
// sorted into the canonical daily-row lock order (business_id, feature_scope).
func reservationScopes(p ReserveParams) ([]ScopeCap, error) {
	scopes := make([]ScopeCap, 0, 1+len(p.ExtraScopes))
	scopes = append(scopes, ScopeCap{BusinessID: p.BusinessID, FeatureScope: p.FeatureScope, CapMicroUSD: p.CapMicroUSD})
	scopes = append(scopes, p.ExtraScopes...)
	seen := make(map[ScopeCap]struct{}, len(scopes))
	for _, sc := range scopes {
		if sc.CapMicroUSD <= 0 {
			return nil, fmt.Errorf("llm: cap must be positive for reservation scope %d/%q", sc.BusinessID, sc.FeatureScope)
		}
		key := ScopeCap{BusinessID: sc.BusinessID, FeatureScope: sc.FeatureScope}
		if _, dup := seen[key]; dup {
			return nil, fmt.Errorf("llm: duplicate reservation scope %d/%q", sc.BusinessID, sc.FeatureScope)
		}
		seen[key] = struct{}{}
	}
	sort.Slice(scopes, func(i, j int) bool { return scopeLess(scopes[i], scopes[j]) })
	return scopes, nil
}

func scopeLess(a, b ScopeCap) bool {
	if a.BusinessID != b.BusinessID {
		return a.BusinessID < b.BusinessID
	}
	return a.FeatureScope < b.FeatureScope
}

func scopeLabel(scope string) string {
	if scope == "" {
		return "owner"
	}
	return scope
}

// lockReservationFamily locks the primary reservation and every extra-scope
// child row in canonical (business_id, feature_scope, id) order, returning the
// family and the index of the primary row. ErrReservationNotFound when the
// primary id does not exist.
func lockReservationFamily(tx *gorm.DB, reservationID string) ([]aiSpendReservation, int, error) {
	if _, err := uuid.Parse(reservationID); err != nil {
		return nil, -1, ErrReservationNotFound
	}
	var rows []aiSpendReservation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ? OR parent_reservation_id = ?", reservationID, reservationID).
		Order("business_id, feature_scope, id").
		Find(&rows).Error; err != nil {
		return nil, -1, err
	}
	for i := range rows {
		if rows[i].ID == reservationID {
			return rows, i, nil
		}
	}
	return nil, -1, ErrReservationNotFound
}

// RecordConsumed durably records provider spend before releasing any
// conservative hold. A later reconciliation can therefore survive a process
// crash or ledger-update failure without erasing already-consumed cost.
func (s *BudgetStore) RecordConsumed(ctx context.Context, p FinalizeParams) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("llm: budget store not configured")
	}
	if p.ActualMicroUSD < 0 {
		return fmt.Errorf("llm: actual amount must be non-negative")
	}
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		family, primary, err := lockReservationFamily(tx, p.ReservationID)
		if err != nil {
			return err
		}
		switch family[primary].Status {
		case ReservationConsumed, ReservationFinalized:
			return nil
		case ReservationReserved:
		default:
			return fmt.Errorf("llm: cannot record consumed reservation in status %q", family[primary].Status)
		}
		// The same actual spend counts against every scope the call held.
		for i := range family {
			res := &family[i]
			if res.Status != ReservationReserved {
				continue // a child already recovered by ExpireStale
			}
			actual := p.ActualMicroUSD
			res.ActualMicroUSD = &actual
			res.Status = ReservationConsumed
			res.ServedModel = p.ServedModel
			if p.InputTokens > 0 {
				res.InputTokens = p.InputTokens
			}
			if p.OutputTokens > 0 {
				res.OutputTokens = p.OutputTokens
			}
			res.UpdatedAt = now
			if err := tx.Save(res).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// RecordUnpricedConsumed atomically latches the cluster-wide shutdown and
// records the full conservative reservation as consumed. An unknown served
// model must never be finalized using a potentially cheaper requested model.
func (s *BudgetStore) RecordUnpricedConsumed(ctx context.Context, p FinalizeParams) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("llm: budget store not configured")
	}
	now := p.Now
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		family, primary, err := lockReservationFamily(tx, p.ReservationID)
		if err != nil {
			return err
		}
		if st := family[primary].Status; st != ReservationReserved && st != ReservationConsumed && st != ReservationFinalized {
			return fmt.Errorf("llm: cannot record unpriced reservation in status %q", st)
		}
		for i := range family {
			res := &family[i]
			if res.Status != ReservationReserved {
				continue
			}
			actual := res.ConservativeMicroUSD
			res.ActualMicroUSD = &actual
			res.Status = ReservationConsumed
			res.ServedModel = p.ServedModel
			if p.InputTokens > 0 {
				res.InputTokens = p.InputTokens
			}
			if p.OutputTokens > 0 {
				res.OutputTokens = p.OutputTokens
			}
			res.UpdatedAt = now
			if err := tx.Save(res).Error; err != nil {
				return err
			}
		}
		control := aiBudgetControl{ID: 1, UnpricedModelShutdown: true, ShutdownReason: "unpriced served model: " + p.ServedModel, UpdatedAt: now}
		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "id"}},
			DoUpdates: clause.AssignmentColumns([]string{"unpriced_model_shutdown", "shutdown_reason", "updated_at"}),
		}).Create(&control).Error
	})
}

// Finalize records provider spend, then reconciles it into the daily ledger.
// The split is deliberate: if reconciliation fails, status=consumed remains
// durable and ExpireStale will reconcile it rather than release the hold.
func (s *BudgetStore) Finalize(ctx context.Context, p FinalizeParams) error {
	if err := s.RecordConsumed(ctx, p); err != nil {
		return err
	}
	return s.ReconcileConsumed(ctx, p.ReservationID, p.Now)
}

// ReconcileConsumed transfers a durable consumed row into finalized spend.
func (s *BudgetStore) ReconcileConsumed(ctx context.Context, reservationID string, now time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("llm: budget store not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		family, primary, err := lockReservationFamily(tx, reservationID)
		if err != nil {
			return err
		}
		switch st := family[primary].Status; st {
		case ReservationFinalized:
			// Idempotent for the primary; fall through so a child left
			// consumed by an interrupted reconcile is still transferred.
		case ReservationConsumed:
		default:
			return fmt.Errorf("llm: cannot finalize reservation in status %q", st)
		}
		for i := range family {
			res := &family[i]
			if res.Status != ReservationConsumed {
				continue
			}
			if res.ActualMicroUSD == nil {
				return fmt.Errorf("llm: consumed reservation has no actual spend")
			}
			daily, err := lockDaily(tx, res.BusinessID, res.FeatureScope, res.UsageDate)
			if err != nil {
				return err
			}
			daily.ReservedMicroUSD -= res.ConservativeMicroUSD
			if daily.ReservedMicroUSD < 0 {
				daily.ReservedMicroUSD = 0
			}
			daily.FinalizedMicroUSD += *res.ActualMicroUSD
			daily.UpdatedAt = now
			if err := tx.Save(daily).Error; err != nil {
				return err
			}
			res.Status = ReservationFinalized
			res.UpdatedAt = now
			if err := tx.Save(res).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Release returns a reserved amount to the daily headroom (provider failure).
// Idempotent for already-released or expired rows.
func (s *BudgetStore) Release(ctx context.Context, reservationID string, now time.Time) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("llm: budget store not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		family, primary, err := lockReservationFamily(tx, reservationID)
		if err != nil {
			return err
		}
		switch family[primary].Status {
		case ReservationReleased, ReservationExpired:
			return nil // idempotent
		case ReservationFinalized:
			return fmt.Errorf("llm: cannot release finalized reservation")
		case ReservationReserved:
			// continue
		default:
			return fmt.Errorf("llm: cannot release reservation in status %q", family[primary].Status)
		}
		for i := range family {
			res := &family[i]
			if res.Status != ReservationReserved {
				continue
			}
			daily, err := lockDaily(tx, res.BusinessID, res.FeatureScope, res.UsageDate)
			if err != nil {
				return err
			}
			daily.ReservedMicroUSD -= res.ConservativeMicroUSD
			if daily.ReservedMicroUSD < 0 {
				daily.ReservedMicroUSD = 0
			}
			daily.UpdatedAt = now
			if err := tx.Save(daily).Error; err != nil {
				return err
			}
			res.Status = ReservationReleased
			res.UpdatedAt = now
			if err := tx.Save(res).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ExpireStale fail-closed reconciles stale reserved/consumed calls. A reserved
// row is charged at its conservative hold because a crashed replica cannot
// prove whether the provider consumed it; a consumed row uses recorded actual.
func (s *BudgetStore) ExpireStale(ctx context.Context, now time.Time) (int, error) {
	if s == nil || s.db == nil {
		return 0, fmt.Errorf("llm: budget store not configured")
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}

	var expired int
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var rows []aiSpendReservation
		// Lock candidate rows so concurrent ExpireStale / Finalize cannot race.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("status IN ? AND expires_at < ?", []string{ReservationReserved, ReservationConsumed}, now).
			Order("business_id, feature_scope, id").
			Find(&rows).Error; err != nil {
			return err
		}
		for i := range rows {
			res := &rows[i]
			daily, err := lockDaily(tx, res.BusinessID, res.FeatureScope, res.UsageDate)
			if err != nil {
				return err
			}
			actual := res.ConservativeMicroUSD
			if res.Status == ReservationConsumed && res.ActualMicroUSD != nil {
				actual = *res.ActualMicroUSD
			}
			daily.ReservedMicroUSD -= res.ConservativeMicroUSD
			if daily.ReservedMicroUSD < 0 {
				daily.ReservedMicroUSD = 0
			}
			daily.UpdatedAt = now
			daily.FinalizedMicroUSD += actual
			if err := tx.Save(daily).Error; err != nil {
				return err
			}
			res.ActualMicroUSD = &actual
			res.Status = ReservationFinalized
			res.UpdatedAt = now
			if err := tx.Save(res).Error; err != nil {
				return err
			}
			expired++
		}
		return nil
	})
	return expired, err
}

// UnpricedModelShutdownActive reports the durable cluster-wide fail-closed
// latch set when any replica observes a served model absent from pricing.
func (s *BudgetStore) UnpricedModelShutdownActive(ctx context.Context) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("llm: budget store not configured")
	}
	var control aiBudgetControl
	err := s.db.WithContext(ctx).Where("id = ?", 1).First(&control).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return control.UnpricedModelShutdown, nil
}

// CommittedSpend returns finalized + reserved micro-USD for the scope/day.
// Missing rows report 0,0 (not an error).
func (s *BudgetStore) CommittedSpend(ctx context.Context, businessID uint, featureScope string, usageDate time.Time) (finalized, reserved int64, err error) {
	if s == nil || s.db == nil {
		return 0, 0, fmt.Errorf("llm: budget store not configured")
	}
	usageDate = UTCDate(usageDate)
	var daily aiDailySpend
	err = s.db.WithContext(ctx).
		Where("business_id = ? AND feature_scope = ? AND usage_date = ?", businessID, featureScope, usageDate).
		First(&daily).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	return daily.FinalizedMicroUSD, daily.ReservedMicroUSD, nil
}

// CommittedSpendScopes returns finalized+reserved micro-USD for each scope on
// usageDate, aligned with scopes, in a single round trip. A scope with no
// ledger row yet reports 0. Only BusinessID/FeatureScope are read from each
// ScopeCap; callers compare the totals against their own caps.
func (s *BudgetStore) CommittedSpendScopes(ctx context.Context, scopes []ScopeCap, usageDate time.Time) ([]int64, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("llm: budget store not configured")
	}
	out := make([]int64, len(scopes))
	if len(scopes) == 0 {
		return out, nil
	}
	usageDate = UTCDate(usageDate)
	var where strings.Builder
	args := make([]interface{}, 0, 1+2*len(scopes))
	args = append(args, usageDate)
	where.WriteString("usage_date = ? AND (")
	for i, sc := range scopes {
		if i > 0 {
			where.WriteString(" OR ")
		}
		where.WriteString("(business_id = ? AND feature_scope = ?)")
		args = append(args, sc.BusinessID, sc.FeatureScope)
	}
	where.WriteString(")")
	var rows []aiDailySpend
	if err := s.db.WithContext(ctx).
		Select("business_id", "feature_scope", "finalized_micro_usd", "reserved_micro_usd").
		Where(where.String(), args...).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		for i, sc := range scopes {
			if row.BusinessID == sc.BusinessID && row.FeatureScope == sc.FeatureScope {
				out[i] = row.FinalizedMicroUSD + row.ReservedMicroUSD
			}
		}
	}
	return out, nil
}

// DailySpendMetric is one business/feature line for UTC-date metrics emission.
type DailySpendMetric struct {
	BusinessID        uint
	FeatureScope      string
	UsageDate         time.Time
	FinalizedMicroUSD int64
	ReservedMicroUSD  int64
}

// MetricsForUTCDate returns durable daily spend rows for metrics (not auth).
func (s *BudgetStore) MetricsForUTCDate(ctx context.Context, usageDate time.Time) ([]DailySpendMetric, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("llm: budget store not configured")
	}
	usageDate = UTCDate(usageDate)
	var rows []aiDailySpend
	if err := s.db.WithContext(ctx).
		Where("usage_date = ?", usageDate).
		Order("business_id, feature_scope").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]DailySpendMetric, 0, len(rows))
	for _, r := range rows {
		out = append(out, DailySpendMetric{
			BusinessID:        r.BusinessID,
			FeatureScope:      r.FeatureScope,
			UsageDate:         r.UsageDate,
			FinalizedMicroUSD: r.FinalizedMicroUSD,
			ReservedMicroUSD:  r.ReservedMicroUSD,
		})
	}
	return out, nil
}

func lockOrCreateDaily(tx *gorm.DB, businessID uint, featureScope string, usageDate, now time.Time) (*aiDailySpend, error) {
	usageDate = UTCDate(usageDate)
	// Upsert shell row, then lock for the check-and-increment.
	if err := tx.Exec(`
		INSERT INTO ai_daily_spend (business_id, feature_scope, usage_date, finalized_micro_usd, reserved_micro_usd, created_at, updated_at)
		VALUES (?, ?, ?, 0, 0, ?, ?)
		ON CONFLICT (business_id, feature_scope, usage_date) DO NOTHING
	`, businessID, featureScope, usageDate, now, now).Error; err != nil {
		return nil, err
	}
	return lockDaily(tx, businessID, featureScope, usageDate)
}

func lockDaily(tx *gorm.DB, businessID uint, featureScope string, usageDate time.Time) (*aiDailySpend, error) {
	usageDate = UTCDate(usageDate)
	var daily aiDailySpend
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("business_id = ? AND feature_scope = ? AND usage_date = ?", businessID, featureScope, usageDate).
		First(&daily).Error
	if err != nil {
		return nil, err
	}
	return &daily, nil
}

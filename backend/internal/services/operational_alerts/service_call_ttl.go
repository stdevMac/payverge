package operational_alerts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// ServiceCallTTL is one typical dining seating. Water / ready-to-order /
// empty-table check-please leave the live list after this window. Unpaid
// same-seating check-please and claimed/assigned calls do not auto-clear.
const ServiceCallTTL = 90 * time.Minute

// ServiceCallExpireReason is recorded on the resolve event when the seating
// SLA lapses. Guest create skips the post-resolve cooldown for this reason
// because GetServiceCallStatus already projects stale rows as "none".
const ServiceCallExpireReason = "expired"

const serviceCallExpireBatch = 200

// serviceCallCheckReasonJSON is a dialect-agnostic metadata match for the
// guest check-please enum. The persist path writes a closed {"reason":"check"}
// object. The clause must CAST the column to TEXT: metadata is jsonb on
// Postgres and `jsonb NOT LIKE` has no operator (SQLSTATE 42883), which
// 500'd every dashboard alerts list (#819). CAST(... AS TEXT) is valid on
// both SQLite (tests) and Postgres (prod).
const serviceCallCheckReasonJSON = `%"reason":"check"%`

// ServiceCallMinEventAt is the exclusive lower bound for a usable SLA clock.
// Year-1 / Unix-zero last_event_at must not match `last_event_at < cutoff`
// (GET already treats a zero clock as not stale; expire must match).
var ServiceCallMinEventAt = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// ServiceCallCutoff is the exclusive last_event_at floor for a still-live
// service call at `now`.
func ServiceCallCutoff(now time.Time) time.Time {
	return now.Add(-ServiceCallTTL)
}

// IsStaleServiceCall reports whether a service-call clock is at or past the
// seating SLA. A zero or pre-2000 last_event_at is not stale — fail closed
// so a missing clock cannot auto-clear a live call.
func IsStaleServiceCall(lastEventAt time.Time, now time.Time) bool {
	if !lastEventAt.After(ServiceCallMinEventAt) {
		return false
	}
	return !lastEventAt.After(ServiceCallCutoff(now))
}

// ServiceCallTTLExempt reports whether a call must stay on the live list
// past the seating SLA regardless of occupancy: claimed/assigned (staff
// owns it). reason=check is not a blanket exemption — empty checks and
// leftover occupied-with-no-bill still expire (#729 Core T1).
func ServiceCallTTLExempt(status database.OperationalAlertStatus, reason string) bool {
	return status == database.OperationalAlertStatusClaimed
}

// SameSeatingCheckPlease reports whether a stale reason=check belongs to
// the current unpaid seating. Occupancy here is server open/partial only.
// Leftover-kitchen host-stand occupied (#779/#774 / #704) is not empty on
// the floor and is not a keep for this TTL. A bill opened after the call
// is a new check and must not inherit yesterday's "check please".
func SameSeatingCheckPlease(reason string, lastEventAt, billCreatedAt time.Time, hasActiveBill bool) bool {
	if reason != "check" || !hasActiveBill {
		return false
	}
	if !lastEventAt.After(ServiceCallMinEventAt) {
		return true
	}
	return !billCreatedAt.After(lastEventAt.Add(time.Minute))
}

// ServiceCallReason is the guest enum on alert metadata (water/order/check).
func ServiceCallReason(metadata database.JSONRawMessage) string {
	return parseGuestServiceCallReason(metadata)
}

func serviceCallLapsedSQL(now time.Time) (string, []any) {
	// List/badge: hide stale unassigned water/order at SQL. Check-please is
	// post-filtered by same-seating occupancy so unpaid seated stays and
	// empty / newer-bill leftovers drop.
	return "alert_type = ? AND status = ? AND last_event_at > ? AND last_event_at < ? AND CAST(metadata AS TEXT) NOT LIKE ?",
		[]any{
			database.OperationalAlertTypeServiceCall,
			database.OperationalAlertStatusOpen,
			ServiceCallMinEventAt,
			ServiceCallCutoff(now),
			serviceCallCheckReasonJSON,
		}
}

func serviceCallExpireSQL(now time.Time) (string, []any) {
	return "alert_type = ? AND status = ? AND last_event_at > ? AND last_event_at < ?",
		[]any{
			database.OperationalAlertTypeServiceCall,
			database.OperationalAlertStatusOpen,
			ServiceCallMinEventAt,
			ServiceCallCutoff(now),
		}
}

// ExpireStaleServiceCalls resolves open, unassigned service_call alerts
// whose last_event_at is older than ServiceCallTTL unless the table still
// has a same-seating open/partial bill. Claimed rows, unpaid seated
// check-please, and zero clocks stay live. Empty checks and a newer bill
// after yesterday's call expire. Bounded per sweep. Idempotent.
func (s *Service) ExpireStaleServiceCalls(ctx context.Context, now time.Time) (int, error) {
	return s.expireStaleServiceCalls(ctx, now, 0, 0)
}

func (s *Service) expireStaleServiceCalls(ctx context.Context, now time.Time, businessID uint, tableID uint) (int, error) {
	if s == nil || s.db == nil {
		return 0, ErrInvalidAlertInput
	}
	clause, args := serviceCallExpireSQL(now)
	q := s.db.WithContext(ctx).Where(clause, args...)
	if businessID > 0 {
		q = q.Where("business_id = ?", businessID)
	}
	if tableID > 0 {
		q = q.Where("resource_type = ? AND resource_id = ?",
			database.OperationalAlertResourceTypeTable, tableID)
	}

	var alerts []database.OperationalAlert
	if err := q.Order("id ASC").Limit(serviceCallExpireBatch).Find(&alerts).Error; err != nil {
		return 0, err
	}
	if len(alerts) == 0 {
		return 0, nil
	}

	occupied, err := s.activeBillTables(ctx, alerts)
	if err != nil {
		return 0, err
	}

	expired := 0
	for _, alert := range alerts {
		reason := ServiceCallReason(alert.Metadata)
		if ServiceCallTTLExempt(alert.Status, reason) {
			continue
		}
		if !IsStaleServiceCall(alert.LastEventAt, now) {
			continue
		}
		if created, ok := occupied[tableBillKey{businessID: alert.BusinessID, tableID: uint(alert.ResourceID)}]; ok {
			if !created.After(alert.LastEventAt.Add(time.Minute)) {
				continue
			}
		}
		if _, err := s.ResolveAlertByIDForBusiness(ctx, alert.BusinessID, alert.ID, Actor{Name: "system"}, ServiceCallExpireReason); err != nil {
			if errors.Is(err, ErrAlertNotFound) || errors.Is(err, ErrAlertConflict) {
				continue
			}
			var conflict *AlertConflictError
			if errors.As(err, &conflict) {
				continue
			}
			return expired, err
		}
		expired++
	}
	return expired, nil
}

type tableBillKey struct {
	businessID uint
	tableID    uint
}

func (s *Service) activeBillTables(ctx context.Context, alerts []database.OperationalAlert) (map[tableBillKey]time.Time, error) {
	out := make(map[tableBillKey]time.Time)
	if len(alerts) == 0 {
		return out, nil
	}
	businessIDs := make([]uint, 0, len(alerts))
	tableIDs := make([]int64, 0, len(alerts))
	seenBiz := map[uint]struct{}{}
	seenTbl := map[int64]struct{}{}
	for _, alert := range alerts {
		if _, ok := seenBiz[alert.BusinessID]; !ok {
			seenBiz[alert.BusinessID] = struct{}{}
			businessIDs = append(businessIDs, alert.BusinessID)
		}
		if _, ok := seenTbl[alert.ResourceID]; !ok {
			seenTbl[alert.ResourceID] = struct{}{}
			tableIDs = append(tableIDs, alert.ResourceID)
		}
	}

	var rows []struct {
		BusinessID uint
		TableID    uint
		CreatedAt  time.Time
	}
	err := s.db.WithContext(ctx).
		Model(&database.Bill{}).
		Select("business_id, table_id, created_at").
		Where("business_id IN ? AND table_id IN ? AND status IN ?",
			businessIDs, tableIDs, database.ActiveBillStatuses()).
		Order("created_at DESC").
		Find(&rows).Error
	if err != nil {
		// Tests that never migrate bills must not fail the sweep: treat as
		// no occupancy so empty-table water / check-please can still expire.
		if isMissingBillsRelation(err) {
			return out, nil
		}
		return nil, err
	}
	for _, row := range rows {
		if row.TableID == 0 {
			continue
		}
		key := tableBillKey{businessID: row.BusinessID, tableID: row.TableID}
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = row.CreatedAt
	}
	return out, nil
}

func (s *Service) hasSameSeatingActiveBill(ctx context.Context, businessID, tableID uint, lastEventAt time.Time) bool {
	if s == nil || s.db == nil || tableID == 0 {
		return false
	}
	var row struct {
		CreatedAt time.Time
	}
	err := s.db.WithContext(ctx).
		Model(&database.Bill{}).
		Select("created_at").
		Where("business_id = ? AND table_id = ? AND status IN ?",
			businessID, tableID, database.ActiveBillStatuses()).
		Order("created_at DESC").
		Limit(1).
		Take(&row).Error
	if err != nil {
		return false
	}
	return SameSeatingCheckPlease("check", lastEventAt, row.CreatedAt, true)
}

func (s *Service) omitEmptyTableStaleCheckPlease(ctx context.Context, alerts []database.OperationalAlert, now time.Time) []database.OperationalAlert {
	staleCheck := make([]database.OperationalAlert, 0)
	for _, alert := range alerts {
		if alert.AlertType != database.OperationalAlertTypeServiceCall {
			continue
		}
		if ServiceCallTTLExempt(alert.Status, ServiceCallReason(alert.Metadata)) {
			continue
		}
		if !IsStaleServiceCall(alert.LastEventAt, now) {
			continue
		}
		if ServiceCallReason(alert.Metadata) != "check" {
			continue
		}
		staleCheck = append(staleCheck, alert)
	}
	if len(staleCheck) == 0 {
		return alerts
	}
	occupied, err := s.activeBillTables(ctx, staleCheck)
	if err != nil {
		return alerts
	}
	out := make([]database.OperationalAlert, 0, len(alerts))
	for _, alert := range alerts {
		if alert.AlertType == database.OperationalAlertTypeServiceCall &&
			!ServiceCallTTLExempt(alert.Status, ServiceCallReason(alert.Metadata)) &&
			IsStaleServiceCall(alert.LastEventAt, now) &&
			ServiceCallReason(alert.Metadata) == "check" {
			created, ok := occupied[tableBillKey{businessID: alert.BusinessID, tableID: uint(alert.ResourceID)}]
			if !SameSeatingCheckPlease("check", alert.LastEventAt, created, ok) {
				continue
			}
		}
		out = append(out, alert)
	}
	return out
}

func isMissingBillsRelation(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no such table: bills") ||
		strings.Contains(msg, `relation "bills" does not exist`)
}

package operational_alerts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
)

var (
	ErrAlertNotFound       = errors.New("operational alert not found")
	ErrInvalidAlertInput   = errors.New("invalid operational alert input")
	ErrInvalidAlertSetting = errors.New("invalid operational alert settings")
	// ErrAlertConflict is returned when a claim/resolve/snooze is attempted
	// from a state the transition does not allow: claiming an alert someone
	// else holds, claiming or snoozing a resolved/dismissed alert, etc.
	// Handlers map it to HTTP 409.
	ErrAlertConflict = errors.New("operational alert state conflict")
)

// AlertConflictError wraps ErrAlertConflict and carries the current alert row
// so handlers can tell the caller WHO holds the claim (claimed_by_name) and
// what state blocked the transition.
type AlertConflictError struct {
	Alert database.OperationalAlert
}

func (e *AlertConflictError) Error() string {
	return fmt.Sprintf("operational alert state conflict: alert %d is %s", e.Alert.ID, e.Alert.Status)
}

func (e *AlertConflictError) Unwrap() error { return ErrAlertConflict }

// init wires the database package's batch alert-resolution path (bill close /
// void cancels pending orders and resolves their alerts inside the same tx)
// to the SSE hub. The database package cannot import internal/events (events
// imports database), so it exposes a publisher hook instead; registering it
// here keeps the frame shape identical to publishAlert's.
func init() {
	database.SetOperationalAlertResolvedPublisher(func(alert database.OperationalAlert) {
		publishAlert("alert.resolved", alert)
	})
}

// sensitiveMetadataKeys is the denylist stripped from ALL alert metadata
// before it is persisted or published.
//
// RBAC rationale: operational alerts fan out over SSE + REST to every holder
// of alerts:read, which is a strictly broader audience than financial:read.
// Callers (notably the payments handlers) historically passed amount_cents /
// tip_cents / settlement details into alert metadata, silently leaking money
// data past the financial:read gate. The FE alert copy
// (operationalAlertCopy.ts) never displays amounts — it only uses keys like
// customer_name, table_id/table_name, reason, conversation_id — so stripping
// centrally here loses nothing and closes the leak for every current and
// future caller.
var sensitiveMetadataKeys = []string{
	"amount_cents",
	"tip_cents",
	"tip_amount_cents",
	"settlement_source",
	"method",
	"payment_method",
}

type Service struct {
	db *gorm.DB
}

func NewService(db *gorm.DB) *Service {
	return &Service{db: db}
}

type Actor struct {
	StaffID *uint
	UserID  *uint
	Name    string
}

func (a Actor) eventName() string {
	if a.Name != "" {
		return a.Name
	}
	if a.StaffID != nil {
		return fmt.Sprintf("staff:%d", *a.StaffID)
	}
	if a.UserID != nil {
		return fmt.Sprintf("user:%d", *a.UserID)
	}
	return "system"
}

type UpsertAlertInput struct {
	BusinessID   uint
	AlertType    database.OperationalAlertType
	ResourceType database.OperationalAlertResourceType
	ResourceID   uint
	Priority     database.OperationalAlertPriority
	Title        string
	Body         string
	Metadata     map[string]any
	// MergeKey opts this upsert into evidence-preserving merges. It names the
	// event (for example "tx_hash_conflict:<hash>") and is used where several
	// producers share one dedup key: the bill-scoped payment_refund_review row
	// carries crypto settlement failures, tx-hash conflicts and wrong-amount
	// transfers. When an active row exists for the key:
	//   - the same MergeKey refreshes the row, as an upsert without it does;
	//   - an event that outranks the row becomes the headline, and the row's
	//     previous headline is kept in metadata.followup_events;
	//   - any other event leaves the row's priority, title, body and metadata
	//     alone and is appended to metadata.followup_events.
	// Priority never drops and no earlier event's evidence is overwritten.
	MergeKey string
}

const (
	mergeKeyMetadataKey       = "merge_key"
	followUpEventsMetadataKey = "followup_events"
	// maxFollowUpEvents bounds the trail kept on one alert row; the oldest
	// follow-ups are dropped first.
	maxFollowUpEvents = 20
)

func (in UpsertAlertInput) validate() error {
	if in.BusinessID == 0 || in.AlertType == "" || in.ResourceType == "" || in.ResourceID == 0 {
		return ErrInvalidAlertInput
	}
	if in.Priority == "" {
		return nil
	}
	switch in.Priority {
	case database.OperationalAlertPriorityLow,
		database.OperationalAlertPriorityNormal,
		database.OperationalAlertPriorityHigh,
		database.OperationalAlertPriorityUrgent:
		return nil
	default:
		return ErrInvalidAlertInput
	}
}

type ListOptions struct {
	Statuses []database.OperationalAlertStatus
	Types    []database.OperationalAlertType
	Since    *time.Time
	Limit    int // 0 = no explicit limit (legacy active-list behavior)
}

func (s *Service) ListAlerts(ctx context.Context, businessID uint, opts ListOptions) ([]database.OperationalAlert, error) {
	statuses := opts.Statuses
	if len(statuses) == 0 {
		statuses = []database.OperationalAlertStatus{
			database.OperationalAlertStatusOpen,
			database.OperationalAlertStatusClaimed,
		}
	}
	q := s.db.WithContext(ctx).
		Where("business_id = ? AND status IN ?", businessID, statuses).
		Order("last_event_at DESC, id DESC")
	if len(opts.Types) > 0 {
		q = q.Where("alert_type IN ?", opts.Types)
	}
	if opts.Since != nil {
		q = q.Where("last_event_at >= ?", *opts.Since)
	}
	if opts.Limit > 0 {
		q = q.Limit(opts.Limit)
	}
	// Seating-SLA: hide unassigned water/order past the clock at SQL.
	// Empty-table / newer-bill stale check-please is dropped after the
	// occupancy read so unpaid same-seating check-please stays on the
	// queue and sidebar badge. Zero last_event_at is fail-closed.
	now := time.Now()
	lapsed, lapsedArgs := serviceCallLapsedSQL(now)
	q = q.Where("NOT ("+lapsed+")", lapsedArgs...)
	var alerts []database.OperationalAlert
	if err := q.Find(&alerts).Error; err != nil {
		return nil, err
	}
	return s.omitEmptyTableStaleCheckPlease(ctx, alerts, now), nil
}

func (s *Service) ListActiveAlerts(ctx context.Context, businessID uint, statuses []database.OperationalAlertStatus, types []database.OperationalAlertType) ([]database.OperationalAlert, error) {
	return s.ListAlerts(ctx, businessID, ListOptions{Statuses: statuses, Types: types})
}

func (s *Service) UpsertAlert(ctx context.Context, in UpsertAlertInput) (*database.OperationalAlert, error) {
	if s == nil || s.db == nil {
		return nil, ErrInvalidAlertInput
	}
	if err := in.validate(); err != nil {
		return nil, err
	}
	now := time.Now()
	priority := in.Priority
	if priority == "" {
		priority = database.OperationalAlertPriorityNormal
	}
	metadataIn := in.Metadata
	if in.MergeKey != "" {
		metadataIn = make(map[string]any, len(in.Metadata)+1)
		for k, v := range in.Metadata {
			metadataIn[k] = v
		}
		metadataIn[mergeKeyMetadataKey] = in.MergeKey
	}
	metadata, err := marshalMetadata(metadataIn)
	if err != nil {
		return nil, err
	}

	var out database.OperationalAlert
	var eventType database.OperationalAlertEventType
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		findActive := func(dest *database.OperationalAlert) error {
			return tx.Where(
				"business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ? AND status IN ?",
				in.BusinessID,
				in.AlertType,
				in.ResourceType,
				in.ResourceID,
				[]database.OperationalAlertStatus{database.OperationalAlertStatusOpen, database.OperationalAlertStatusClaimed},
			).First(dest).Error
		}

		var existing database.OperationalAlert
		findErr := findActive(&existing)

		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			eventType = database.OperationalAlertEventTypeCreated
			out = database.OperationalAlert{
				BusinessID:   in.BusinessID,
				AlertType:    in.AlertType,
				ResourceType: in.ResourceType,
				ResourceID:   int64(in.ResourceID),
				Status:       database.OperationalAlertStatusOpen,
				Priority:     priority,
				Title:        in.Title,
				Body:         in.Body,
				LastEventAt:  now,
				Metadata:     metadata,
			}
			createErr := tx.Create(&out).Error
			if createErr == nil {
				return recordEventTx(tx, out.ID, out.BusinessID, eventType, Actor{Name: "system"}, map[string]any{
					"alert_type":    in.AlertType,
					"resource_type": in.ResourceType,
					"resource_id":   in.ResourceID,
				})
			}
			if !isUniqueViolation(createErr) {
				return createErr
			}
			// Lost a first-alert race: a concurrent caller inserted the active
			// row between our find and our INSERT, and
			// idx_operational_alerts_active_unique rejected ours. Fall through
			// to the update path against the winner's row instead of 500ing.
			if refindErr := findActive(&existing); refindErr != nil {
				return createErr
			}
		} else if findErr != nil {
			return findErr
		}

		eventType = database.OperationalAlertEventTypeStatusChanged
		updates := map[string]any{
			"priority":      priority,
			"title":         in.Title,
			"body":          in.Body,
			"metadata":      metadata,
			"last_event_at": now,
			"updated_at":    now,
		}
		if in.MergeKey != "" {
			merged, mergeErr := mergeAlertEvent(existing, in, priority, metadata, now)
			if mergeErr != nil {
				return mergeErr
			}
			for k, v := range merged {
				updates[k] = v
			}
		}
		if existing.Status == database.OperationalAlertStatusClaimed {
			eventType = database.OperationalAlertEventTypeReopened
			updates["status"] = database.OperationalAlertStatusOpen
			updates["claimed_by_staff_id"] = nil
			updates["claimed_by_user_id"] = nil
			updates["claimed_by_name"] = ""
			updates["claimed_at"] = nil
			updates["snoozed_until"] = nil
		}
		if err := tx.Model(&database.OperationalAlert{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := tx.First(&out, existing.ID).Error; err != nil {
			return err
		}
		return recordEventTx(tx, out.ID, out.BusinessID, eventType, Actor{Name: "system"}, map[string]any{
			"alert_type":    in.AlertType,
			"resource_type": in.ResourceType,
			"resource_id":   in.ResourceID,
		})
	})
	if err != nil {
		return nil, err
	}
	publishAlert(eventNameForEventType(eventType), out)
	return &out, nil
}

func (s *Service) ClaimAlertForBusiness(ctx context.Context, businessID, alertID uint, actor Actor, source string) (*database.OperationalAlert, error) {
	return s.transitionAlert(ctx, businessID, alertID, actor, database.OperationalAlertStatusClaimed, database.OperationalAlertEventTypeClaimed, map[string]any{"source": source}, nil)
}

func (s *Service) ResolveAlert(ctx context.Context, alertID uint, actor Actor, reason string) (*database.OperationalAlert, error) {
	now := time.Now()
	return s.transitionAlert(ctx, 0, alertID, actor, database.OperationalAlertStatusResolved, database.OperationalAlertEventTypeResolved, map[string]any{"reason": reason}, &now)
}

func (s *Service) ResolveAlertByIDForBusiness(ctx context.Context, businessID, alertID uint, actor Actor, reason string) (*database.OperationalAlert, error) {
	now := time.Now()
	return s.transitionAlert(ctx, businessID, alertID, actor, database.OperationalAlertStatusResolved, database.OperationalAlertEventTypeResolved, map[string]any{"reason": reason}, &now)
}

func (s *Service) SnoozeAlertForBusiness(ctx context.Context, businessID, alertID uint, actor Actor, until time.Time) (*database.OperationalAlert, error) {
	return s.snoozeAlert(ctx, businessID, alertID, actor, until)
}

func (s *Service) snoozeAlert(ctx context.Context, businessID, alertID uint, actor Actor, until time.Time) (*database.OperationalAlert, error) {
	var out database.OperationalAlert
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", alertID)
		if businessID > 0 {
			query = query.Where("business_id = ?", businessID)
		}
		if err := query.First(&out).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertNotFound
			}
			return err
		}
		// Snoozing only makes sense for an active alert; a resolved/dismissed
		// one is a state conflict, not a silent no-op.
		if out.Status != database.OperationalAlertStatusOpen && out.Status != database.OperationalAlertStatusClaimed {
			return &AlertConflictError{Alert: out}
		}
		guarded := tx.Model(&database.OperationalAlert{}).
			Where("id = ? AND status IN ?", alertID, []database.OperationalAlertStatus{
				database.OperationalAlertStatusOpen,
				database.OperationalAlertStatusClaimed,
			}).
			Updates(map[string]any{
				"snoozed_until": until,
				"updated_at":    time.Now(),
			})
		if guarded.Error != nil {
			return guarded.Error
		}
		if guarded.RowsAffected == 0 {
			if err := tx.First(&out, alertID).Error; err != nil {
				return err
			}
			return &AlertConflictError{Alert: out}
		}
		if err := recordEventTx(tx, out.ID, out.BusinessID, database.OperationalAlertEventTypeSnoozed, actor, map[string]any{"snoozed_until": until}); err != nil {
			return err
		}
		return tx.First(&out, alertID).Error
	})
	if err != nil {
		return nil, err
	}
	publishAlert("alert.updated", out)
	return &out, nil
}

func (s *Service) ResolveAlertForResource(ctx context.Context, businessID uint, resourceType database.OperationalAlertResourceType, resourceID uint, actor Actor, reason string) error {
	return s.ResolveAlertsForResources(ctx, businessID, resourceType, []uint{resourceID}, actor, reason)
}

// ResolveAlertsForResources resolves every open/claimed alert of the business
// attached to any of the given resources. One narrow id lookup keyed on the
// resource (riding idx_operational_alerts_active_unique) replaces a full
// active-alert list per resource, so a batch of N resources costs one read
// plus one transition per alert that is actually open.
func (s *Service) ResolveAlertsForResources(ctx context.Context, businessID uint, resourceType database.OperationalAlertResourceType, resourceIDs []uint, actor Actor, reason string) error {
	if len(resourceIDs) == 0 {
		return nil
	}
	ids := make([]int64, 0, len(resourceIDs))
	for _, id := range resourceIDs {
		ids = append(ids, int64(id))
	}
	var alertIDs []uint
	if err := s.db.WithContext(ctx).
		Model(&database.OperationalAlert{}).
		Where("business_id = ? AND resource_type = ? AND resource_id IN ? AND status IN ?",
			businessID, resourceType, ids, []database.OperationalAlertStatus{
				database.OperationalAlertStatusOpen,
				database.OperationalAlertStatusClaimed,
			}).
		Order("id ASC").
		Pluck("id", &alertIDs).Error; err != nil {
		return err
	}
	for _, alertID := range alertIDs {
		if _, err := s.ResolveAlert(ctx, alertID, actor, reason); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) GetAlertEventsForBusiness(ctx context.Context, businessID, alertID uint) ([]database.OperationalAlertEvent, error) {
	return s.getAlertEvents(ctx, businessID, alertID)
}

func (s *Service) getAlertEvents(ctx context.Context, businessID, alertID uint) ([]database.OperationalAlertEvent, error) {
	var alert database.OperationalAlert
	query := s.db.WithContext(ctx).Where("id = ?", alertID)
	if businessID > 0 {
		query = query.Where("business_id = ?", businessID)
	}
	if err := query.First(&alert).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAlertNotFound
		}
		return nil, err
	}
	var eventsOut []database.OperationalAlertEvent
	if err := s.db.WithContext(ctx).Where("alert_id = ?", alertID).Order("created_at DESC, id DESC").Find(&eventsOut).Error; err != nil {
		return nil, err
	}
	return eventsOut, nil
}

func (s *Service) GetSettings(ctx context.Context, businessID uint) (*database.BusinessAlertSettings, error) {
	var settings database.BusinessAlertSettings
	err := s.db.WithContext(ctx).Where("business_id = ?", businessID).First(&settings).Error
	if err == nil {
		// Self-heal rows created before reservation_approval existed: a zero
		// value (Priority == "") means the type was never configured, not that
		// the operator disabled it. Default it to the reservation_new setting
		// so approval alerts inherit the operator's reservation preference.
		if settings.EventSettings.ReservationApproval.Priority == "" {
			settings.EventSettings.ReservationApproval = settings.EventSettings.ReservationNew
			if settings.EventSettings.ReservationApproval.Priority == "" {
				settings.EventSettings.ReservationApproval = database.AlertTypeSetting{
					Enabled:   true,
					Repeating: true,
					Priority:  database.OperationalAlertPriorityUrgent,
				}
			}
		}
		// Self-heal rows created before service_call existed: default it to
		// urgent + repeating, same as reservation_approval — a guest asking
		// for help at the table should not go quiet.
		if settings.EventSettings.ServiceCall.Priority == "" {
			settings.EventSettings.ServiceCall = database.AlertTypeSetting{
				Enabled:   true,
				Repeating: true,
				Priority:  database.OperationalAlertPriorityUrgent,
			}
		}
		// Self-heal rows created before payment_refund_review existed: default
		// to enabled/high/non-repeating (same as the fresh-row default and the
		// FE fallback) so refund-review alerts can ring without the operator
		// re-saving settings.
		if settings.EventSettings.PaymentRefundReview.Priority == "" {
			settings.EventSettings.PaymentRefundReview = database.AlertTypeSetting{
				Enabled:   true,
				Repeating: false,
				Priority:  database.OperationalAlertPriorityHigh,
			}
		}
		// Self-heal rows created before ai_takeover existed: default it to
		// urgent + repeating — a guest talking to a paused AI with no human on
		// the claim is waiting on nobody.
		if settings.EventSettings.AITakeover.Priority == "" {
			settings.EventSettings.AITakeover = database.AlertTypeSetting{
				Enabled:   true,
				Repeating: true,
				Priority:  database.OperationalAlertPriorityUrgent,
			}
		}
		if settings.RepeatIntervalSeconds < database.MinBusinessAlertRepeatIntervalSeconds {
			settings.RepeatIntervalSeconds = database.MinBusinessAlertRepeatIntervalSeconds
		}
		if settings.RepeatIntervalSeconds > database.MaxBusinessAlertRepeatIntervalSeconds {
			settings.RepeatIntervalSeconds = database.MaxBusinessAlertRepeatIntervalSeconds
		}
		return &settings, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	settings = database.DefaultBusinessAlertSettings(businessID)
	if err := s.db.WithContext(ctx).Create(&settings).Error; err != nil {
		return nil, err
	}
	return &settings, nil
}

func (s *Service) UpdateSettings(ctx context.Context, businessID uint, next database.BusinessAlertSettings, _ Actor) (*database.BusinessAlertSettings, error) {
	if next.Volume < 0 || next.Volume > 1 || next.RepeatIntervalSeconds < database.MinBusinessAlertRepeatIntervalSeconds || next.RepeatIntervalSeconds > database.MaxBusinessAlertRepeatIntervalSeconds {
		return nil, ErrInvalidAlertSetting
	}
	current, err := s.GetSettings(ctx, businessID)
	if err != nil {
		return nil, err
	}
	updates := map[string]any{
		"enabled":                       next.Enabled,
		"browser_notifications_enabled": next.BrowserNotificationsEnabled,
		"sound_enabled":                 next.SoundEnabled,
		"volume":                        next.Volume,
		"repeat_interval_seconds":       next.RepeatIntervalSeconds,
		"event_settings":                next.EventSettings,
		"updated_at":                    time.Now(),
	}
	if err := s.db.WithContext(ctx).Model(&database.BusinessAlertSettings{}).Where("id = ?", current.ID).Updates(updates).Error; err != nil {
		return nil, err
	}
	return s.GetSettings(ctx, businessID)
}

func (s *Service) CreateOrderNewAlert(ctx context.Context, order database.Order) error {
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   order.BusinessID,
		AlertType:    database.OperationalAlertTypeOrderNew,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   order.ID,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        fmt.Sprintf("New order #%s", order.OrderNumber),
		Body:         fmt.Sprintf("Order #%s needs review", order.OrderNumber),
		Metadata: map[string]any{
			"order_id":     order.ID,
			"order_number": order.OrderNumber,
			"bill_id":      order.BillID,
		},
	})
	return err
}

func (s *Service) CreateKitchenReadyAlert(ctx context.Context, order database.Order) error {
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   order.BusinessID,
		AlertType:    database.OperationalAlertTypeKitchenOrderReady,
		ResourceType: database.OperationalAlertResourceTypeOrder,
		ResourceID:   order.ID,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        fmt.Sprintf("Kitchen order #%s", order.OrderNumber),
		Body:         fmt.Sprintf("Order #%s is ready for kitchen", order.OrderNumber),
		Metadata: map[string]any{
			"order_id":     order.ID,
			"order_number": order.OrderNumber,
			"bill_id":      order.BillID,
		},
	})
	return err
}

// CreateReservationAlert upserts a reservation alert with the caller-chosen
// type and priority: reservation_approval for pending guest requests (urgent —
// they expire), reservation_new for auto-confirmed bookings (urgent only when
// the party arrives soon).
func (s *Service) CreateReservationAlert(ctx context.Context, reservation database.TableReservation, alertType database.OperationalAlertType, priority database.OperationalAlertPriority) error {
	title := "New reservation"
	if alertType == database.OperationalAlertTypeReservationApproval {
		title = "Reservation needs approval"
	}
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   reservation.BusinessID,
		AlertType:    alertType,
		ResourceType: database.OperationalAlertResourceTypeReservation,
		ResourceID:   reservation.ID,
		Priority:     priority,
		Title:        title,
		Body:         fmt.Sprintf("Reservation from %s", reservation.CustomerName),
		Metadata: map[string]any{
			"reservation_id": reservation.ID,
			"customer_name":  reservation.CustomerName,
		},
	})
	return err
}

func (s *Service) CreateDeliveryNewAlert(ctx context.Context, delivery database.DeliveryOrder) error {
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   delivery.BusinessID,
		AlertType:    database.OperationalAlertTypeDeliveryNew,
		ResourceType: database.OperationalAlertResourceTypeDelivery,
		ResourceID:   delivery.ID,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        fmt.Sprintf("New delivery #%s", delivery.DeliveryNumber),
		Body:         fmt.Sprintf("Delivery for %s needs dispatch", delivery.CustomerName),
		Metadata: map[string]any{
			"delivery_id":     delivery.ID,
			"delivery_number": delivery.DeliveryNumber,
			"bill_id":         delivery.BillID,
		},
	})
	return err
}

func (s *Service) CreateBillNewAlert(ctx context.Context, bill database.Bill) error {
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypeBillNew,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   bill.ID,
		Priority:     database.OperationalAlertPriorityNormal,
		Title:        fmt.Sprintf("New bill #%s", bill.BillNumber),
		Body:         "A new bill was created",
		Metadata: map[string]any{
			"bill_id":     bill.ID,
			"bill_number": bill.BillNumber,
		},
	})
	return err
}

func (s *Service) CreatePaymentRequestedAlertForResource(ctx context.Context, bill database.Bill, resourceType database.OperationalAlertResourceType, resourceID uint, metadata map[string]any) error {
	if resourceID == 0 {
		resourceID = bill.ID
	}
	if resourceType == "" {
		resourceType = database.OperationalAlertResourceTypeAltPayment
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRequested,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Priority:     database.OperationalAlertPriorityNormal,
		Title:        "Payment requested",
		Body:         fmt.Sprintf("Payment requested for bill #%s", bill.BillNumber),
		Metadata:     metadata,
	})
	return err
}

func (s *Service) CreatePaymentReceivedAlertForResource(ctx context.Context, bill database.Bill, resourceType database.OperationalAlertResourceType, resourceID uint, metadata map[string]any) error {
	if resourceID == 0 {
		resourceID = bill.ID
	}
	if resourceType == "" {
		resourceType = database.OperationalAlertResourceTypePayment
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentReceived,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Priority:     database.OperationalAlertPriorityNormal,
		Title:        "Payment received",
		Body:         fmt.Sprintf("Payment received for bill #%s", bill.BillNumber),
		Metadata:     metadata,
	})
	return err
}

// CreatePaymentRefundReviewAlert surfaces an out-of-band partial provider refund
// for manual reconciliation. We do not auto-reverse partial refunds (a full
// reversal over-reverses; a partial mutation would desync the Payment row vs
// accounting), so the operator must reconcile via a manual ledger entry / the
// PSP dashboard. resourceID should be the local Payment row id when known,
// otherwise the bill id.
func (s *Service) CreatePaymentRefundReviewAlert(ctx context.Context, bill database.Bill, resourceID uint, metadata map[string]any) error {
	if resourceID == 0 {
		resourceID = bill.ID
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypePayment,
		ResourceID:   resourceID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Partial refund needs manual reconciliation",
		Body:         fmt.Sprintf("A partial provider refund was received for bill #%s. Reconcile it manually (manual ledger entry / PSP dashboard) — automatic reversal was skipped to avoid over-reversing the books.", bill.BillNumber),
		Metadata:     metadata,
	})
	return err
}

// CreatePaymentWebhookReviewAlert flags a verified provider refund, dispute or
// reversal that could not be tied to a bill and was acknowledged without
// touching the ledger. It is scoped to the webhook_events row so each event
// raises exactly one alert.
func (s *Service) CreatePaymentWebhookReviewAlert(ctx context.Context, businessID, webhookEventID uint, metadata map[string]any) error {
	if businessID == 0 || webhookEventID == 0 {
		return fmt.Errorf("payment webhook review alert needs a business and a webhook event")
	}
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["webhook_event_id"] = webhookEventID
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypeWebhookEvent,
		ResourceID:   webhookEventID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Provider refund or dispute needs manual reconciliation",
		Body:         "A payment provider reported a refund, dispute or reversal for a payment Payverge never recorded. Check it against the provider dashboard and reconcile the ledger by hand.",
		Metadata:     metadata,
	})
	return err
}

// Crypto payment-review reasons carried in alert metadata.reason. The bill-
// scoped payment_refund_review row is shared by all three, so each producer
// upserts with a MergeKey: an Urgent settlement failure is never downgraded or
// overwritten by a later High conflict or wrong-amount event, and the later
// events are kept in metadata.followup_events.
const (
	CryptoReviewReasonSettlementFailed = "crypto_settlement_failed"
	CryptoReviewReasonTxHashConflict   = "tx_hash_conflict"
	CryptoReviewReasonAmountMismatch   = "amount_mismatch"
)

func cryptoReviewMergeKey(reason, txHash string) string {
	return reason + ":" + txHash
}

// CreateCryptoSettlementReviewAlert flags a crypto transfer that was verified
// on-chain but failed to settle into the bill — money received with no record.
func (s *Service) CreateCryptoSettlementReviewAlert(ctx context.Context, bill database.Bill, txHash string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["reason"] = CryptoReviewReasonSettlementFailed
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	metadata["transaction_hash"] = txHash
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   bill.ID,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "Verified crypto payment failed to settle",
		Body: fmt.Sprintf(
			"A verified on-chain USDC transfer for bill %s (tx %s) could not be recorded as a payment. Reconcile manually before closing the bill.",
			bill.BillNumber, txHash,
		),
		Metadata: metadata,
		MergeKey: cryptoReviewMergeKey(CryptoReviewReasonSettlementFailed, txHash),
	})
	return err
}

// CreateCryptoTxHashConflictAlert flags a guest attempt to settle a bill with
// an on-chain transfer the ledger already holds (for another bill, or for this
// bill under another quote or amount). The transfer verified, so this is the
// shape of a payment replay: someone claiming a transfer they may not have
// made. No money is missing (the transfer is already recorded once); the
// operator must not settle or refund anything on the strength of it. holder,
// when known, is the payment that already records the transfer: the alert
// names that bill only when it belongs to the same business (another venue's
// bill numbers never leak across tenants). High, not urgent. It shares the
// bill-scoped payment-review dedup key with
// CreateCryptoSettlementReviewAlert: repeated attempts with the same transfer
// refresh one row, and an open settlement-review alert keeps its Urgent
// headline with this event appended to its follow-ups.
func (s *Service) CreateCryptoTxHashConflictAlert(ctx context.Context, bill database.Bill, txHash string, holder *database.PaymentTxHashHolder, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["reason"] = CryptoReviewReasonTxHashConflict
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	metadata["transaction_hash"] = txHash

	title := "Crypto payment reused on another bill"
	body := fmt.Sprintf(
		"Someone tried to settle bill %s with on-chain transfer %s, which is already recorded as a payment. The bill was not marked paid. Do not settle or refund anything by hand on the strength of this transfer; check who paid first.",
		bill.BillNumber, txHash,
	)
	if holder != nil && holder.BusinessID != 0 && holder.BusinessID == bill.BusinessID {
		metadata["recorded_bill_id"] = holder.BillID
		metadata["recorded_bill_number"] = holder.BillNumber
		metadata["recorded_payment_id"] = holder.PaymentID
		if holder.BillID == bill.ID {
			title = "Crypto payment presented twice"
			body = fmt.Sprintf(
				"Someone presented on-chain transfer %s for bill %s again under a different payment quote. It is already recorded on this bill, so it was not counted a second time. Do not settle or refund anything by hand on the strength of this transfer.",
				txHash, bill.BillNumber,
			)
		} else {
			body = fmt.Sprintf(
				"Someone tried to settle bill %s with on-chain transfer %s, which already paid bill %s. Bill %s was not marked paid. Do not settle or refund anything with this transfer: it belongs to bill %s.",
				bill.BillNumber, txHash, holder.BillNumber, bill.BillNumber, holder.BillNumber,
			)
		}
	}
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   bill.ID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        title,
		Body:         body,
		Metadata:     metadata,
		MergeKey:     cryptoReviewMergeKey(CryptoReviewReasonTxHashConflict, txHash),
	})
	return err
}

// CreateCryptoAmountMismatchAlert records a guest USDC transfer that went to
// the venue wallet but does not carry the quote's exact amount. The exact
// amount is what binds a transfer to one guest's quote, so the transfer is
// refused and not recorded on the bill; the guest is told to ask staff. This
// alert is the staff-side record of that refusal, so whoever the guest asks
// can find the transfer. High, not urgent: the transfer may be a genuine
// wrong-amount payment or a transfer that belongs to someone else (it may even
// settle another bill later, when its real payer submits it with their own
// quote), so the body tells staff to check both before acting. Amounts
// stay out of metadata (alerts:read is broader than financial:read); the
// transaction hash is the lookup key. It shares the bill-scoped payment-review
// row with the other crypto review alerts through MergeKey.
func (s *Service) CreateCryptoAmountMismatchAlert(ctx context.Context, bill database.Bill, txHash string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["reason"] = CryptoReviewReasonAmountMismatch
	metadata["bill_id"] = bill.ID
	metadata["bill_number"] = bill.BillNumber
	metadata["transaction_hash"] = txHash
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   bill.BusinessID,
		AlertType:    database.OperationalAlertTypePaymentRefundReview,
		ResourceType: database.OperationalAlertResourceTypeBill,
		ResourceID:   bill.ID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Crypto transfer with the wrong amount",
		Body: fmt.Sprintf(
			"A guest presented on-chain transfer %s for bill %s. It went to the venue wallet but not for the quoted amount, so it was not recorded as a payment. It may belong to another guest. Before settling or refunding anything by hand, check on a block explorer that the guest's own wallet sent it, and check that no bill has since recorded it as a payment.",
			txHash, bill.BillNumber,
		),
		Metadata: metadata,
		MergeKey: cryptoReviewMergeKey(CryptoReviewReasonAmountMismatch, txHash),
	})
	return err
}

// CreateReorgSuspectedAlert surfaces a previously-confirmed on-chain payment or
// crypto refund whose transaction has fallen out of the canonical chain (a
// suspected blockchain reorganization). We deliberately do NOT auto-reverse the
// ledger on this signal — a flaky RPC transiently reporting a live tx as missing
// must not flip a paid bill to unpaid — so the operator verifies on-chain and
// remediates via the audited void/refund path. High priority. resourceID is the
// payments.id or payment_refunds.id; kind is "payment" | "refund". Upsert-backed:
// repeated sweeps of the same still-orphaned record refresh one row rather than
// piling up duplicates.
func (s *Service) CreateReorgSuspectedAlert(ctx context.Context, businessID uint, resourceID uint, kind string, metadata map[string]any) error {
	if metadata == nil {
		metadata = map[string]any{}
	}
	metadata["kind"] = kind
	// Scope the alert's resource type by kind so a payment and a refund that
	// share the same numeric id in one business never collide on the alert dedup
	// key (payments and payment_refunds have independent id sequences). A
	// collision would let one orphaned record's alert overwrite/mask the other's.
	resourceType := database.OperationalAlertResourceTypePayment
	if kind == "refund" {
		resourceType = database.OperationalAlertResourceTypePaymentRefund
	}
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypePaymentReorgSuspected,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Suspected chain reorg — verify settlement",
		Body:         fmt.Sprintf("A confirmed on-chain %s transaction is no longer in the canonical chain (suspected reorganization). Verify it on-chain before treating the funds as final; remediate via the audited void/refund path if it was orphaned.", kind),
		Metadata:     metadata,
	})
	return err
}

// ServiceCallStatus is the guest-readable projection of a table's service-call
// alert. It deliberately exposes NOTHING but the state machine — no staff
// names, ids, or timestamps beyond what the guest UI needs.
type ServiceCallStatus string

const (
	ServiceCallStatusNone         ServiceCallStatus = "none"
	ServiceCallStatusOpen         ServiceCallStatus = "open"
	ServiceCallStatusAcknowledged ServiceCallStatus = "acknowledged"
	ServiceCallStatusResolved     ServiceCallStatus = "resolved"
)

// CreateServiceCallAlert upserts a service_call alert for a table (guest
// raising a hand for water / order / check). Like the other Upsert-backed
// helpers, a second call while the alert is still open/claimed reopens or
// refreshes the SAME row (bumping last_event_at) rather than piling up
// duplicates; a resolved call starts a fresh row, which is what we want —
// GetServiceCallStatus always reads the newest row for the table.
//
// Contract: reason must be a caller-validated enum value, never guest free
// text — it is embedded verbatim into staff-facing alert copy.
func (s *Service) CreateServiceCallAlert(ctx context.Context, businessID uint, tableID uint, tableName, reason string) error {
	// Persist a lapsed call before upsert so a guest re-raise after the
	// seating SLA starts a fresh row (new created_at) instead of refreshing
	// yesterday's Open ticket.
	_, _ = s.expireStaleServiceCalls(ctx, time.Now(), businessID, tableID)
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypeServiceCall,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   tableID,
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        fmt.Sprintf("Service call — %s", tableName),
		Body:         fmt.Sprintf("A guest at %s asked for help (%s)", tableName, reason),
		Metadata: map[string]any{
			"table_id": tableID,
			// table_name is required by the FE copy layer: localized dictionary
			// overrides replace the English title (which embeds the table name)
			// with a static string, so without this key operators on non-English
			// locales cannot tell WHICH table raised its hand.
			"table_name": tableName,
			"reason":     reason,
		},
	})
	return err
}

// CreateAITakeoverAlert upserts an ai_takeover alert for a conversation: a
// guest wrote into a paused AI conversation that no operator currently holds,
// so nobody (bot or human) will answer. Upsert-backed: repeat guest messages
// while the alert is still open/claimed refresh the SAME row instead of
// piling up duplicates.
func (s *Service) CreateAITakeoverAlert(ctx context.Context, businessID uint, conversationID int64) error {
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypeAITakeover,
		ResourceType: database.OperationalAlertResourceTypeAIConversation,
		ResourceID:   uint(conversationID),
		Priority:     database.OperationalAlertPriorityUrgent,
		Title:        "Guest waiting for a human",
		Body:         "A guest wrote in a paused AI conversation — open the AI Waiter workspace to reply",
		Metadata:     map[string]any{"conversation_id": conversationID},
	})
	return err
}

// ResolveAITakeoverAlert closes the open takeover alert for a conversation
// (called when an operator claims it). A missing alert is not an error.
func (s *Service) ResolveAITakeoverAlert(ctx context.Context, businessID uint, conversationID int64, actor Actor) error {
	return s.ResolveAlertForResource(ctx, businessID,
		database.OperationalAlertResourceTypeAIConversation, uint(conversationID), actor, "claimed")
}

// GetServiceCallStatus returns the guest-facing state, the last enum reason
// (water/order/check, empty when unknown), and when the call last resolved
// (for the guest-side cooldown). Narrow projection: status + reason metadata
// + last_event_at (seating SLA). Newest row for the table. Unassigned water/
// order older than ServiceCallTTL project as none. Stale check-please stays
// only when the table still has a same-seating unpaid bill; empty checks
// and a newer bill do not inherit yesterday's "check please". Claimed calls
// stay live. The bills EXISTS runs only on the stale-check branch so the
// common poll stays one operational_alerts LIMIT 1.
func (s *Service) GetServiceCallStatus(ctx context.Context, businessID uint, tableID uint) (ServiceCallStatus, string, *time.Time, error) {
	var row struct {
		Status      database.OperationalAlertStatus
		ResolvedAt  *time.Time
		Metadata    database.JSONRawMessage
		LastEventAt time.Time
	}
	err := s.db.WithContext(ctx).
		Model(&database.OperationalAlert{}).
		Select("status, resolved_at, metadata, last_event_at").
		Where("business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ?",
			businessID, database.OperationalAlertTypeServiceCall,
			database.OperationalAlertResourceTypeTable, tableID).
		Order("last_event_at DESC, id DESC").
		Limit(1).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ServiceCallStatusNone, "", nil, nil
	}
	if err != nil {
		return ServiceCallStatusNone, "", nil, err
	}
	reason := parseGuestServiceCallReason(row.Metadata)
	// Past the seating SLA: guest chrome must not keep leftover water or
	// an empty-table / inherited check-please. Returning none (not resolved)
	// also skips the 2-minute post-handle cooldown for truly lapsed calls.
	if IsStaleServiceCall(row.LastEventAt, time.Now()) && !ServiceCallTTLExempt(row.Status, reason) {
		if reason == "check" && s.hasSameSeatingActiveBill(ctx, businessID, tableID, row.LastEventAt) {
			// unpaid seated check-please — fall through
		} else {
			return ServiceCallStatusNone, "", nil, nil
		}
	}
	switch row.Status {
	case database.OperationalAlertStatusOpen:
		return ServiceCallStatusOpen, reason, nil, nil
	case database.OperationalAlertStatusClaimed:
		return ServiceCallStatusAcknowledged, reason, nil, nil
	default:
		return ServiceCallStatusResolved, reason, row.ResolvedAt, nil
	}
}

func parseGuestServiceCallReason(metadata database.JSONRawMessage) string {
	if len(metadata) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(metadata, &payload); err != nil {
		return ""
	}
	raw, _ := payload["reason"].(string)
	switch raw {
	case "water", "order", "check":
		return raw
	default:
		return ""
	}
}

// actorHoldsClaim reports whether the alert's current claim belongs to the
// acting staff member / user.
func actorHoldsClaim(alert database.OperationalAlert, actor Actor) bool {
	if actor.StaffID != nil && alert.ClaimedByStaffID != nil && *actor.StaffID == *alert.ClaimedByStaffID {
		return true
	}
	if actor.UserID != nil && alert.ClaimedByUserID != nil && *actor.UserID == *alert.ClaimedByUserID {
		return true
	}
	return false
}

// transitionAlert enforces the alert state machine:
//
//	claim:   open → claimed. Re-claiming an alert you already hold is an
//	         idempotent success; an alert claimed by someone else — or already
//	         resolved/dismissed — is an AlertConflictError (last-write-wins
//	         claim stealing and "resurrecting" resolved alerts both violated
//	         idx_operational_alerts_active_unique when a newer active alert
//	         existed for the same resource → 500).
//	resolve: open/claimed → resolved. Resolving an already-resolved alert is
//	         an idempotent success (row returned, no duplicate event or SSE
//	         frame). The original claim attribution is preserved — the
//	         resolver is recorded on the event row, not on claimed_by_*.
//
// The UPDATE folds the allowed source states into its WHERE and re-checks
// RowsAffected so a concurrent transition between the read and the write
// still cannot slip through.
func (s *Service) transitionAlert(ctx context.Context, businessID, alertID uint, actor Actor, status database.OperationalAlertStatus, eventType database.OperationalAlertEventType, metadata map[string]any, resolvedAt *time.Time) (*database.OperationalAlert, error) {
	var out database.OperationalAlert
	idempotent := false
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		query := tx.Where("id = ?", alertID)
		if businessID > 0 {
			query = query.Where("business_id = ?", businessID)
		}
		if err := query.First(&out).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrAlertNotFound
			}
			return err
		}

		allowedFrom := []database.OperationalAlertStatus{
			database.OperationalAlertStatusOpen,
			database.OperationalAlertStatusClaimed,
		}
		switch status {
		case database.OperationalAlertStatusClaimed:
			allowedFrom = []database.OperationalAlertStatus{database.OperationalAlertStatusOpen}
			switch out.Status {
			case database.OperationalAlertStatusOpen:
				// proceed
			case database.OperationalAlertStatusClaimed:
				if actorHoldsClaim(out, actor) {
					idempotent = true
					return nil
				}
				return &AlertConflictError{Alert: out}
			default:
				return &AlertConflictError{Alert: out}
			}
		case database.OperationalAlertStatusResolved:
			switch out.Status {
			case database.OperationalAlertStatusOpen, database.OperationalAlertStatusClaimed:
				// proceed
			case database.OperationalAlertStatusResolved:
				idempotent = true
				return nil
			default:
				return &AlertConflictError{Alert: out}
			}
		default:
			if out.Status != database.OperationalAlertStatusOpen && out.Status != database.OperationalAlertStatusClaimed {
				return &AlertConflictError{Alert: out}
			}
		}

		updates := map[string]any{
			"status":     status,
			"updated_at": time.Now(),
		}
		if status == database.OperationalAlertStatusClaimed {
			now := time.Now()
			updates["claimed_by_staff_id"] = actor.StaffID
			updates["claimed_by_user_id"] = actor.UserID
			updates["claimed_by_name"] = actor.eventName()
			updates["claimed_at"] = now
			updates["snoozed_until"] = nil
		} else if out.ClaimedByStaffID == nil && out.ClaimedByUserID == nil && out.ClaimedByName == "" {
			// Direct transition of a never-claimed alert: attribute it to the
			// actor (legacy behavior the FE relies on for "resolved by X" on
			// unclaimed alerts). When a claim exists, it is preserved — the
			// resolver differing from the claimer is recorded on the event row.
			updates["claimed_by_staff_id"] = actor.StaffID
			updates["claimed_by_user_id"] = actor.UserID
			updates["claimed_by_name"] = actor.eventName()
		}
		if resolvedAt != nil {
			updates["resolved_at"] = *resolvedAt
		}
		guarded := tx.Model(&database.OperationalAlert{}).
			Where("id = ? AND status IN ?", alertID, allowedFrom).
			Updates(updates)
		if guarded.Error != nil {
			return guarded.Error
		}
		if guarded.RowsAffected == 0 {
			// Lost a race between the read and the guarded write.
			if err := tx.First(&out, alertID).Error; err != nil {
				return err
			}
			if status == database.OperationalAlertStatusResolved && out.Status == database.OperationalAlertStatusResolved {
				idempotent = true
				return nil
			}
			return &AlertConflictError{Alert: out}
		}
		if err := recordEventTx(tx, out.ID, out.BusinessID, eventType, actor, metadata); err != nil {
			return err
		}
		return tx.First(&out, alertID).Error
	})
	if err != nil {
		return nil, err
	}
	if idempotent {
		// No state change: return the current row without a duplicate event or
		// SSE frame.
		return &out, nil
	}
	publishAlert(eventNameForEventType(eventType), out)
	return &out, nil
}

// isUniqueViolation detects a unique-index rejection across the drivers we
// run on (Postgres in production, SQLite in tests) plus GORM's translated
// sentinel when error translation is enabled.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key value") || // Postgres 23505
		strings.Contains(msg, "UNIQUE constraint failed") // SQLite
}

func recordEventTx(tx *gorm.DB, alertID, businessID uint, eventType database.OperationalAlertEventType, actor Actor, metadata map[string]any) error {
	raw, err := marshalMetadata(metadata)
	if err != nil {
		return err
	}
	return tx.Create(&database.OperationalAlertEvent{
		AlertID:      alertID,
		BusinessID:   businessID,
		EventType:    eventType,
		ActorStaffID: actor.StaffID,
		ActorUserID:  actor.UserID,
		ActorName:    actor.eventName(),
		Metadata:     raw,
	}).Error
}

// marshalMetadata sanitizes then marshals metadata. It is the single choke
// point for every metadata write in this package (alert rows AND event rows),
// so the sensitiveMetadataKeys denylist cannot be bypassed by a new caller.
func marshalMetadata(metadata map[string]any) (database.JSONRawMessage, error) {
	sanitized := make(map[string]any, len(metadata))
	for k, v := range metadata {
		sanitized[k] = v
	}
	for _, key := range sensitiveMetadataKeys {
		delete(sanitized, key)
	}
	raw, err := json.Marshal(sanitized)
	if err != nil {
		return nil, err
	}
	return database.JSONRawMessage(raw), nil
}

var alertPriorityRank = map[database.OperationalAlertPriority]int{
	database.OperationalAlertPriorityLow:    1,
	database.OperationalAlertPriorityNormal: 2,
	database.OperationalAlertPriorityHigh:   3,
	database.OperationalAlertPriorityUrgent: 4,
}

// mergeAlertEvent folds a MergeKey upsert into the active row it collides
// with and returns the priority, title, body and metadata to write. See
// UpsertAlertInput.MergeKey for the rules.
func mergeAlertEvent(existing database.OperationalAlert, in UpsertAlertInput, priority database.OperationalAlertPriority, incoming database.JSONRawMessage, now time.Time) (map[string]any, error) {
	existingMeta := decodeAlertMetadata(existing.Metadata)
	incomingMeta := decodeAlertMetadata(incoming)
	followUps := takeFollowUpEvents(existingMeta)
	delete(incomingMeta, followUpEventsMetadataKey)

	newRank := alertPriorityRank[priority]
	existingRank := alertPriorityRank[existing.Priority]

	var (
		headPriority = priority
		headTitle    = in.Title
		headBody     = in.Body
		headMeta     = incomingMeta
	)
	switch existingKey, _ := existingMeta[mergeKeyMetadataKey].(string); {
	case existingKey == in.MergeKey:
		// The same event again: refresh the row, keep the trail.
		if existingRank > newRank {
			headPriority = existing.Priority
		}
	case newRank > existingRank:
		// This event outranks the row: it becomes the headline and the row's
		// previous headline joins the trail.
		followUps = appendFollowUpEvent(followUps, followUpEvent(existing.Title, existing.Priority, existingMeta, existing.LastEventAt))
	default:
		// The row's headline stays; this event joins the trail.
		headPriority = existing.Priority
		headTitle = existing.Title
		headBody = existing.Body
		headMeta = existingMeta
		followUps = appendFollowUpEvent(followUps, followUpEvent(in.Title, priority, incomingMeta, now))
	}
	if len(followUps) > 0 {
		headMeta[followUpEventsMetadataKey] = followUps
	}
	metadata, err := marshalMetadata(headMeta)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"priority": headPriority,
		"title":    headTitle,
		"body":     headBody,
		"metadata": metadata,
	}, nil
}

func decodeAlertMetadata(raw database.JSONRawMessage) map[string]any {
	out := map[string]any{}
	if len(raw) == 0 {
		return out
	}
	if err := json.Unmarshal(raw, &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func takeFollowUpEvents(meta map[string]any) []any {
	raw, ok := meta[followUpEventsMetadataKey]
	delete(meta, followUpEventsMetadataKey)
	if !ok {
		return nil
	}
	list, _ := raw.([]any)
	return list
}

func followUpEvent(title string, priority database.OperationalAlertPriority, meta map[string]any, at time.Time) map[string]any {
	event := make(map[string]any, len(meta)+3)
	for k, v := range meta {
		if k == followUpEventsMetadataKey {
			continue
		}
		event[k] = v
	}
	event["title"] = title
	event["priority"] = priority
	event["recorded_at"] = at.UTC().Format(time.RFC3339)
	return event
}

// appendFollowUpEvent adds event to the trail, replacing an earlier entry
// with the same merge key so a repeated event does not grow the list.
func appendFollowUpEvent(list []any, event map[string]any) []any {
	key, _ := event[mergeKeyMetadataKey].(string)
	out := make([]any, 0, len(list)+1)
	for _, item := range list {
		if key != "" {
			if m, ok := item.(map[string]any); ok {
				if k, _ := m[mergeKeyMetadataKey].(string); k == key {
					continue
				}
			}
		}
		out = append(out, item)
	}
	out = append(out, event)
	if len(out) > maxFollowUpEvents {
		out = out[len(out)-maxFollowUpEvents:]
	}
	return out
}

func eventNameForEventType(eventType database.OperationalAlertEventType) string {
	switch eventType {
	case database.OperationalAlertEventTypeCreated:
		return "alert.created"
	case database.OperationalAlertEventTypeClaimed:
		return "alert.claimed"
	case database.OperationalAlertEventTypeResolved:
		return "alert.resolved"
	case database.OperationalAlertEventTypeDismissed:
		return "alert.dismissed"
	case database.OperationalAlertEventTypeReopened:
		return "alert.reopened"
	default:
		return "alert.updated"
	}
}

func publishAlert(eventName string, alert database.OperationalAlert) {
	events.GetHub().PublishJSON(alert.BusinessID, eventName, alert)
}

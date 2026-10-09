package services

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/notifications"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type DeliveryService struct {
	db                  *gorm.DB
	notificationManager *notifications.NotificationManager
}

var (
	ErrDeliveryValidation     = errors.New("delivery validation failed")
	ErrDeliveryScopedNotFound = errors.New("delivery scoped resource not found")
)

func NewDeliveryService(db *gorm.DB, notificationManager *notifications.NotificationManager) *DeliveryService {
	return &DeliveryService{
		db:                  db,
		notificationManager: notificationManager,
	}
}

// CreateDeliveryOrderRequest represents the request to create a delivery order
type CreateDeliveryOrderRequest struct {
	BusinessID           uint                     `json:"business_id"`
	BillID               uint                     `json:"bill_id"`
	OrderID              *uint                    `json:"order_id"`
	CustomerID           *uint                    `json:"customer_id"`
	DeliveryType         database.DeliveryType    `json:"delivery_type"`
	CustomerName         string                   `json:"customer_name"`
	CustomerPhone        string                   `json:"customer_phone"`
	CustomerEmail        string                   `json:"customer_email"`
	DeliveryAddress      database.DeliveryAddress `json:"delivery_address"`
	PickupLocation       database.Location        `json:"pickup_location"`
	DropoffLocation      database.Location        `json:"dropoff_location"`
	DeliveryFee          float64                  `json:"delivery_fee"`
	DeliveryInstructions string                   `json:"delivery_instructions"`
	ContactlessDelivery  bool                     `json:"contactless_delivery"`
	LeaveAtDoor          bool                     `json:"leave_at_door"`
}

// CreateDeliveryOrder creates a new delivery order
func (s *DeliveryService) CreateDeliveryOrder(req CreateDeliveryOrderRequest) (*database.DeliveryOrder, error) {
	if req.BusinessID == 0 {
		return nil, fmt.Errorf("%w: business ID is required", ErrDeliveryValidation)
	}
	if req.BillID == 0 {
		return nil, fmt.Errorf("%w: bill ID is required", ErrDeliveryValidation)
	}
	if strings.TrimSpace(req.CustomerName) == "" || strings.TrimSpace(req.CustomerPhone) == "" {
		return nil, fmt.Errorf("%w: customer name and phone are required", ErrDeliveryValidation)
	}
	if req.DeliveryFee < 0 {
		return nil, fmt.Errorf("%w: delivery fee cannot be negative", ErrDeliveryValidation)
	}
	if req.DeliveryType == "" {
		req.DeliveryType = database.DeliveryTypeInHouse
	}

	var bill database.Bill
	if err := s.db.Where("id = ? AND business_id = ?", req.BillID, req.BusinessID).First(&bill).Error; err != nil {
		return nil, fmt.Errorf("%w: bill not found for business", ErrDeliveryScopedNotFound)
	}

	if req.OrderID != nil {
		var order database.Order
		if err := s.db.Where("id = ? AND business_id = ?", *req.OrderID, req.BusinessID).First(&order).Error; err != nil {
			return nil, fmt.Errorf("%w: order not found for business", ErrDeliveryScopedNotFound)
		}
		if order.BillID != req.BillID {
			return nil, fmt.Errorf("%w: order does not belong to bill", ErrDeliveryValidation)
		}
	}

	// Generate unique delivery number
	deliveryNumber := s.generateDeliveryNumber()

	// Calculate estimated delivery time (prep time + travel time)
	estimatedDeliveryTime := time.Now().Add(45 * time.Minute) // Default 45 minutes

	deliveryOrder := &database.DeliveryOrder{
		BusinessID:            req.BusinessID,
		BillID:                req.BillID,
		OrderID:               req.OrderID,
		CustomerID:            req.CustomerID,
		DeliveryNumber:        deliveryNumber,
		DeliveryType:          req.DeliveryType,
		Status:                database.DeliveryStatusPending,
		Priority:              database.PriorityNormal,
		CustomerName:          req.CustomerName,
		CustomerPhone:         req.CustomerPhone,
		CustomerEmail:         req.CustomerEmail,
		DeliveryAddress:       req.DeliveryAddress,
		PickupLocation:        req.PickupLocation,
		DropoffLocation:       req.DropoffLocation,
		DeliveryFee:           int64(math.Round(req.DeliveryFee * 100)),
		DeliveryInstructions:  req.DeliveryInstructions,
		ContactlessDelivery:   req.ContactlessDelivery,
		LeaveAtDoor:           req.LeaveAtDoor,
		QuoteMetadata:         database.JSONRawMessage(`{}`),
		EstimatedDeliveryTime: &estimatedDeliveryTime,
		// DEL-OP-2 follow-up: snapshot the effective payment mode at creation
		// (mirrors the guest checkout path in delivery_v1.go).
		PaymentModeStored: s.EffectiveDeliveryPaymentMode(req.BusinessID),
	}

	// DEL-PAY-05: the delivery fee must land on the linked bill total or it is
	// never charged. Mirrors the guest checkout path, which folds the fee into
	// TotalAmount at creation. Both writes are one transaction so a failed fee
	// add never leaves an orphan delivery pointing at an under-charged bill.
	// Money contract: DeliveryFee arrives as float64 dollars on the wire and is
	// stored/added as int64 cents.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(deliveryOrder).Error; err != nil {
			return fmt.Errorf("failed to create delivery order: %w", err)
		}
		if deliveryOrder.DeliveryFee > 0 {
			if err := tx.Model(&database.Bill{}).
				Where("id = ? AND business_id = ?", req.BillID, req.BusinessID).
				UpdateColumn("total_amount", gorm.Expr("total_amount + ?", deliveryOrder.DeliveryFee)).Error; err != nil {
				return fmt.Errorf("failed to add delivery fee to bill: %w", err)
			}
		}
		return nil
	}); err != nil {
		return nil, err
	}

	// Create initial status history
	s.addStatusHistory(deliveryOrder.ID, database.DeliveryStatusPending, nil, "Order created", "system")

	// Send notification to business
	s.notifyBusinessNewDelivery(deliveryOrder)

	return deliveryOrder, nil
}

// GetDeliveryOrder retrieves a delivery order by ID for the operator dispatch
// detail read (GET /businesses/:id/deliveries/:delivery_id, gated by
// delivery:dispatch:read).
//
// PRELOAD-02: Business is projected to display columns only. DeliveryOrder has
// no MarshalJSON scrub for the embedded Business, so a bare Preload("Business")
// serialized the full ~94-field row — Stripe customer/subscription/price/payment
// IDs, card fields, and the onboarding_state jsonb blob — to low-privilege
// dispatch staff (servers/kitchen). The projection excludes every sensitive
// field while keeping the columns the dispatch UI renders (name, currency).
//
// Customer is projected to display columns (no PasswordHash / reset-token over-
// fetch). Driver is projected via the dispatch summary (keeps the email the
// operator UI falls back to). Bill and Order are dropped: the operator
// DeliveryOrder DTO declares neither relation and no dispatch component reads
// delivery.bill / delivery.order — the sibling list endpoint
// (GetBusinessDeliveries) likewise preloads only Driver. StatusHistory stays
// bare (own audit rows; no cross-entity PII).
func (s *DeliveryService) GetDeliveryOrder(id uint) (*database.DeliveryOrder, error) {
	var deliveryOrder database.DeliveryOrder
	if err := s.db.
		Preload("Business", preloadDeliveryDispatchBusinessSummary).
		Preload("Driver", preloadDeliveryDispatchDriverSummary).
		Preload("Customer", preloadDeliveryDispatchCustomerSummary).
		Preload("StatusHistory").
		First(&deliveryOrder, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("delivery order not found")
		}
		return nil, fmt.Errorf("failed to get delivery order: %w", err)
	}
	ptrs := []*database.DeliveryOrder{&deliveryOrder}
	if err := s.hydrateDeliveryMoneyAndGeo(ptrs); err != nil {
		return nil, err
	}
	return &deliveryOrder, nil
}

// GetDeliveryOrderByNumber retrieves a delivery order by delivery number
func (s *DeliveryService) GetDeliveryOrderByNumber(deliveryNumber string) (*database.DeliveryOrder, error) {
	var deliveryOrder database.DeliveryOrder
	if err := s.db.
		Select(
			"id",
			"business_id",
			"bill_id",
			"order_id",
			"driver_id",
			"delivery_number",
			"status",
			"estimated_delivery_time",
			"payment_expires_at",
			"cancellation_reason",
			"delivery_fee",
			"driver_tip",
			"current_latitude",
			"current_longitude",
			"current_timestamp",
			"external_tracking_url",
			"updated_at",
		).
		Preload("Business", preloadDeliveryTrackingBusinessSummary).
		Preload("Driver", preloadDeliveryTrackingDriverSummary).
		Where("delivery_number = ?", deliveryNumber).First(&deliveryOrder).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("delivery order not found")
		}
		return nil, fmt.Errorf("failed to get delivery order: %w", err)
	}
	return &deliveryOrder, nil
}

func preloadDeliveryTrackingBusinessSummary(tx *gorm.DB) *gorm.DB {
	// timezone is required so guest track/pay ETA stamps use venue wall-clock
	// (not the guest device TZ) — same honesty rule as table open/closed pill.
	return tx.Select("id", "name", "custom_url", "default_currency", "display_currency", "timezone")
}

func preloadDeliveryTrackingDriverSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "name", "phone")
}

// preloadDeliveryDispatchBusinessSummary projects the embedded Business on the
// operator dispatch detail read. It EXCLUDES every Stripe ID, card field, and
// the onboarding_state blob (PRELOAD-02) while keeping the display columns the
// dispatch UI renders. Do not widen this with any stripe_*/onboarding_* column.
func preloadDeliveryDispatchBusinessSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select(
		"id",
		"business_id",
		"name",
		"custom_url",
		"phone",
		"timezone",
		"default_currency",
		"display_currency",
	)
}

// preloadDeliveryDispatchDriverSummary projects the assigned driver for the
// dispatch detail read. Like the tracking summary but includes email, which the
// operator DispatchConsole falls back to when a driver has no name.
func preloadDeliveryDispatchDriverSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "business_id", "name", "phone", "email")
}

// preloadDeliveryDispatchCustomerSummary projects the linked customer to display-only
// columns, avoiding the full customer row (credential hashes, reset tokens,
// verification tokens) on the dispatch detail read.
func preloadDeliveryDispatchCustomerSummary(tx *gorm.DB) *gorm.DB {
	return tx.Select("id", "name", "phone", "email")
}

// DeliveryListParams holds pagination and filter parameters for GetBusinessDeliveries.
type DeliveryListParams struct {
	Limit  int
	Offset int
	// Status is the legacy single-status filter. Prefer Statuses when filtering
	// multiple values (dispatch board active/terminal windows). When both are
	// set, Statuses wins.
	Status *database.DeliveryStatus
	// Statuses filters with WHERE status IN (...). Empty means no status filter.
	// Optional — callers that omit it keep the legacy unfiltered shape.
	Statuses []database.DeliveryStatus
	Since    *time.Time // only return orders updated after this time
}

// DeliveryListResult is the paginated response for GetBusinessDeliveries.
type DeliveryListResult struct {
	Deliveries []database.DeliveryOrder `json:"deliveries"`
	Total      int64                    `json:"total"`
	HasMore    bool                     `json:"has_more"`
}

const (
	deliveryListDefaultLimit = 100
	deliveryListMaxLimit     = 500
)

// GetBusinessDeliveries retrieves paginated deliveries for a business.
func (s *DeliveryService) GetBusinessDeliveries(businessID uint, params DeliveryListParams) (*DeliveryListResult, error) {
	// Clamp / default limit.
	if params.Limit <= 0 {
		params.Limit = deliveryListDefaultLimit
	}
	if params.Limit > deliveryListMaxLimit {
		params.Limit = deliveryListMaxLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	statuses, err := normalizeDeliveryStatusFilters(params.Status, params.Statuses)
	if err != nil {
		return nil, err
	}

	base := s.db.Model(&database.DeliveryOrder{}).Where("business_id = ?", businessID)
	switch len(statuses) {
	case 0:
		// no status filter
	case 1:
		base = base.Where("status = ?", statuses[0])
	default:
		base = base.Where("status IN ?", statuses)
	}
	if params.Since != nil {
		base = base.Where("updated_at > ?", *params.Since)
	}

	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count business deliveries: %w", err)
	}

	// Driver is projected exactly like the detail read (DEL-AUTHZ-1): a bare
	// Preload("Driver") serialized total_earnings, license_number,
	// vehicle_plate and current_location to any role holding
	// delivery:dispatch:read.
	var deliveries []database.DeliveryOrder
	if err := base.
		Preload("Driver", preloadDeliveryDispatchDriverSummary).
		Order("created_at DESC").
		Limit(params.Limit).
		Offset(params.Offset).
		Find(&deliveries).Error; err != nil {
		return nil, fmt.Errorf("failed to get business deliveries: %w", err)
	}

	ptrs := make([]*database.DeliveryOrder, len(deliveries))
	for i := range deliveries {
		ptrs[i] = &deliveries[i]
	}
	if err := s.hydrateDeliveryMoneyAndGeo(ptrs); err != nil {
		return nil, err
	}

	return &DeliveryListResult{
		Deliveries: deliveries,
		Total:      total,
		HasMore:    int64(params.Offset)+int64(len(deliveries)) < total,
	}, nil
}

type deliveryMoneyGeoRow struct {
	ID              uint     `gorm:"column:id"`
	BillTotalCents  *int64   `gorm:"column:bill_total_cents"`
	DisplayCurrency string   `gorm:"column:list_display_currency"`
	DefaultCurrency string   `gorm:"column:list_default_currency"`
	BizLatitude     *float64 `gorm:"column:biz_latitude"`
	BizLongitude    *float64 `gorm:"column:biz_longitude"`
}

// hydrateDeliveryMoneyAndGeo attaches the bill total (in CENTS — DeliveryOrder.
// MarshalJSON owns the cents→dollars conversion) and the resolved business
// currency onto each row, and fills 0,0 pickup from the venue coordinates when
// the delivery row never stored a pin.
//
// Exactly ONE query regardless of row count — a single JOIN, not a Bill/Order
// preload (the dispatch UI still does not serialize those aggregates) and not a
// per-row reload. TestGetBusinessDeliveries_MoneyGeoHydrateIsOneQueryForAnyN
// pins that shape.
//
// Currency comes from display_currency → default_currency → USD via
// database.ResolveDispatchCurrency, the same precedence bills and orders use;
// businesses.default_currency has no DB default, so reading it alone left the
// currency blank for every business that never picked one.
//
// Known gap (#896): only PICKUP coordinates are healed. Dropoff pins are passed
// through as stored, so rows seeded without a dropoff still render at 0,0 on
// that side — that needs the delivery row itself to carry a geocoded dropoff.
func (s *DeliveryService) hydrateDeliveryMoneyAndGeo(orders []*database.DeliveryOrder) error {
	if len(orders) == 0 {
		return nil
	}
	ids := make([]uint, 0, len(orders))
	for _, order := range orders {
		if order != nil {
			ids = append(ids, order.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var rows []deliveryMoneyGeoRow
	if err := s.db.Table("delivery_orders").
		Select(
			"delivery_orders.id",
			"bills.total_amount as bill_total_cents",
			"businesses.display_currency as list_display_currency",
			"businesses.default_currency as list_default_currency",
			"businesses.latitude as biz_latitude",
			"businesses.longitude as biz_longitude",
		).
		Joins("LEFT JOIN bills ON bills.id = delivery_orders.bill_id").
		Joins("LEFT JOIN businesses ON businesses.id = delivery_orders.business_id").
		Where("delivery_orders.id IN ?", ids).
		Find(&rows).Error; err != nil {
		return fmt.Errorf("failed to hydrate delivery list money/geo: %w", err)
	}
	byID := make(map[uint]deliveryMoneyGeoRow, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	for _, order := range orders {
		if order == nil {
			continue
		}
		row, ok := byID[order.ID]
		if !ok {
			continue
		}
		if row.BillTotalCents != nil {
			cents := *row.BillTotalCents
			order.DispatchTotalCents = &cents
		}
		order.DispatchCurrency = database.ResolveDispatchCurrency(row.DisplayCurrency, row.DefaultCurrency)
		if order.PickupLocation.Latitude == 0 && order.PickupLocation.Longitude == 0 &&
			row.BizLatitude != nil && row.BizLongitude != nil &&
			(*row.BizLatitude != 0 || *row.BizLongitude != 0) {
			order.PickupLocation.Latitude = *row.BizLatitude
			order.PickupLocation.Longitude = *row.BizLongitude
		}
	}
	return nil
}

// UpdateDeliveryStatus updates the status of a delivery order
// ErrInvalidDeliveryStatus is returned when a write supplies a status that is
// not one of the recognized DeliveryStatus values.
var ErrInvalidDeliveryStatus = errors.New("invalid delivery status")

// ErrDeliveryStatusTerminal is returned when a caller tries to transition a
// delivery out of a terminal state (delivered/cancelled/failed).
var ErrDeliveryStatusTerminal = errors.New("delivery is already in a final state and cannot change")

// ErrUnknownDeliveryStatusFilter is returned by GetBusinessDeliveries when the
// status filter is not a recognized DeliveryStatus. Without this guard an
// unknown value (e.g. ?status=shipped) silently matched zero rows.
var ErrUnknownDeliveryStatusFilter = errors.New("unknown delivery status filter")

// normalizeDeliveryStatusFilters resolves the legacy single Status pointer and
// the multi Statuses slice into a deduped list of valid statuses. Statuses wins
// when non-empty so the dispatch board can send active/terminal windows without
// breaking older single-status callers.
func normalizeDeliveryStatusFilters(
	single *database.DeliveryStatus,
	multi []database.DeliveryStatus,
) ([]database.DeliveryStatus, error) {
	raw := multi
	if len(raw) == 0 && single != nil {
		raw = []database.DeliveryStatus{*single}
	}
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[database.DeliveryStatus]struct{}, len(raw))
	out := make([]database.DeliveryStatus, 0, len(raw))
	for _, s := range raw {
		if !s.IsValid() {
			return nil, ErrUnknownDeliveryStatusFilter
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out, nil
}

// ErrDeliveryAlreadyAssigned is returned when AssignDriver is called on a
// delivery that already has a driver. Re-assigning is a conflict, not an
// idempotent overwrite — the operator must unassign first.
var ErrDeliveryAlreadyAssigned = errors.New("delivery already has an assigned driver")

// errNoopDeliveryStatus is an internal sentinel used to unwind the status-update
// transaction when the requested status equals the current one. It is never
// returned to callers — UpdateDeliveryStatus swallows it and returns nil so a
// redundant advance stays idempotent (no timestamp rewrite, no history row, no
// re-notify).
var errNoopDeliveryStatus = errors.New("delivery status unchanged")

func (s *DeliveryService) UpdateDeliveryStatus(id uint, status database.DeliveryStatus, location *database.Location, changedBy string) error {
	// Reject arbitrary status strings before they reach the row / history /
	// notifications. The handler only checks non-empty (binding:"required").
	if !status.IsValid() {
		return ErrInvalidDeliveryStatus
	}

	// Route both terminal-cleanup statuses through the single lifecycle path so
	// driver release, order cancellation, and unpaid-bill close all happen in
	// one transaction (R3). "failed" needs the SAME cleanup as "cancelled" — a
	// failed delivery never reached the customer and has no refund flow, so the
	// linked order must be cancelled (inventory restored if deducted) and the
	// open bill closed, not merely the driver released.
	// NOTE: location is intentionally not forwarded; cancelDeliveryLinked records no location on the terminal history entry.
	switch status {
	case database.DeliveryStatusCancelled:
		return s.cancelDeliveryLinked(id, "cancelled via status update", changedBy, "cancelled", database.DeliveryStatusCancelled)
	case database.DeliveryStatusFailed:
		return s.cancelDeliveryLinked(id, "delivery failed", changedBy, "failed", database.DeliveryStatusFailed)
	}

	// Read, guard, and write all happen under a FOR UPDATE row lock inside the
	// transaction (R-race): a plain non-locking pre-read would let a concurrent
	// cancel (expiry sweep / operator cancel / order rejection) commit in the
	// read→save window, and a full-row Save with no conditional WHERE would then
	// clobber it — resurrecting a cancelled delivery to delivered, rebinding the
	// released driver, wiping audit fields, and firing a delivered email.
	var deliveryOrder database.DeliveryOrder
	var oldStatus database.DeliveryStatus
	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deliveryOrder, id).Error; err != nil {
			return fmt.Errorf("delivery order not found: %w", err)
		}

		oldStatus = deliveryOrder.Status
		// A delivered/cancelled/failed delivery is final. Allow a redundant no-op
		// (same status) but reject any real transition out of a terminal state,
		// which would corrupt the lifecycle timeline and driver-performance KPIs.
		if oldStatus.IsTerminal() && status != oldStatus {
			return ErrDeliveryStatusTerminal
		}
		// Short-circuit a no-op (from == to) BEFORE the Save/history/notify block.
		// A double-fired advance (operator double-click, retried request) would
		// otherwise re-Save the row — rewriting ActualPickupTime/ActualDeliveryTime
		// to "now" — append a spurious "X → X" history row, and re-send the
		// status-change notification. errNoopDeliveryStatus is caught after the
		// transaction and treated as success by the caller.
		if oldStatus == status {
			return errNoopDeliveryStatus
		}
		// Forward-edge validation under the lock: reject backward moves, the
		// confirmed payment-bypass edge, and pending+online fulfillment advances
		// that skip Accept/payment (see validateOperatorFulfillmentTransition).
		if err := validateOperatorFulfillmentTransition(&deliveryOrder, status); err != nil {
			return err
		}

		deliveryOrder.Status = status
		now := time.Now()
		switch status {
		case database.DeliveryStatusPickedUp:
			deliveryOrder.ActualPickupTime = &now
		case database.DeliveryStatusDelivered:
			deliveryOrder.ActualDeliveryTime = &now
		}
		if location != nil {
			deliveryOrder.CurrentLocation = location
		}

		if err := tx.Omit(clause.Associations).Save(&deliveryOrder).Error; err != nil {
			return fmt.Errorf("failed to update delivery status: %w", err)
		}
		if status.IsTerminal() && deliveryOrder.DriverID != nil {
			if err := releaseDriverTx(tx, *deliveryOrder.DriverID, deliveryOrder.ID); err != nil {
				return err
			}
		}
		// History inside the transaction so it rolls back with the status write.
		history := database.DeliveryStatusHistory{
			DeliveryOrderID: id,
			Status:          status,
			Location:        location,
			Notes:           fmt.Sprintf("Status changed from %s to %s", oldStatus, status),
			ChangedBy:       changedBy,
		}
		return tx.Create(&history).Error
	})
	if err != nil {
		// A no-op status write is not an error: the delivery already sits at the
		// requested status. Return success without any side effects.
		if errors.Is(err, errNoopDeliveryStatus) {
			return nil
		}
		return err
	}

	// Propagate delivered → order: ready → delivered is the valid edge in the
	// order state machine. If the order is in an earlier state (e.g. still
	// pending/approved for prepay), the ValidateTransition guard will reject it;
	// we log and continue rather than failing the delivery update.
	// failed: driver already released in the tx above; no order propagation target exists.
	if status == database.DeliveryStatusDelivered && deliveryOrder.OrderID != nil {
		if err := database.UpdateOrderStatus(*deliveryOrder.OrderID, database.OrderStatusOrderDelivered, changedBy, ""); err != nil {
			log.Printf("delivery delivered: order propagation failed (delivery=%d order=%d): %v", id, *deliveryOrder.OrderID, err)
		} else if order, _, oerr := database.GetOrderByID(*deliveryOrder.OrderID); oerr == nil {
			// Kitchen listens to order.* — without this emit the delivered
			// order sits in the "Ready" column until a manual refresh.
			eventsPublishOrderUpdated(order)
		} else {
			log.Printf("delivery delivered: order reload for SSE failed (delivery=%d order=%d): %v", id, *deliveryOrder.OrderID, oerr)
		}
	}

	// Send notifications based on status
	s.notifyStatusChange(&deliveryOrder, status)

	return nil
}

// AssignDriver assigns a driver to a delivery order inside a single locked
// transaction. The delivery and driver rows are locked FOR UPDATE in a fixed
// order (delivery, then driver) to prevent concurrent double-assignment and
// deadlock. Notifications fire only after the transaction commits.
func (s *DeliveryService) AssignDriver(deliveryID, driverID uint) error {
	var deliveryOrder database.DeliveryOrder
	var driver database.DeliveryDriver

	err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&deliveryOrder, deliveryID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("delivery order not found")
			}
			return fmt.Errorf("failed to lock delivery order: %w", err)
		}

		// Reject re-assignment inside the locked section so a concurrent assign
		// cannot slip a second driver onto the same delivery.
		if deliveryOrder.DriverID != nil {
			return ErrDeliveryAlreadyAssigned
		}

		// A cancelled/failed/delivered delivery is final — assigning a driver
		// would flip it back to "assigned" and resurrect a closed delivery.
		if deliveryOrder.Status.IsTerminal() {
			return ErrDeliveryStatusTerminal
		}

		// DEL-SM-1 + online-pending gate: assignment IS a status change
		// (→ assigned) and must pass the same operator fulfillment machine as
		// UpdateDeliveryStatus. Blocks confirmed (unpaid prepay) and pending +
		// payment_mode_stored=online (skip-Accept free fulfillment).
		if err := validateOperatorFulfillmentTransition(&deliveryOrder, database.DeliveryStatusAssigned); err != nil {
			return err
		}

		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&driver, driverID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return fmt.Errorf("driver not found")
			}
			return fmt.Errorf("failed to lock driver: %w", err)
		}

		// Assignability mirrors GetAvailableDrivers: available + active + not
		// already carrying a delivery. The `status` column is server-managed
		// (set to busy below on assignment, back to online/offline on release)
		// and is NOT a precondition — no production path sets it to "online", so
		// gating on it here made every real driver unassignable.
		if !driver.IsAvailable || !driver.IsActive || driver.CurrentDeliveryID != nil {
			return fmt.Errorf("driver is not available")
		}

		now := time.Now()
		deliveryOrder.DriverID = &driverID
		deliveryOrder.AssignedAt = &now
		deliveryOrder.Status = database.DeliveryStatusAssigned
		if err := tx.Omit(clause.Associations).Save(&deliveryOrder).Error; err != nil {
			return fmt.Errorf("failed to assign driver: %w", err)
		}

		driver.Status = database.DriverStatusBusy
		driver.CurrentDeliveryID = &deliveryID
		if err := tx.Omit(clause.Associations).Save(&driver).Error; err != nil {
			return fmt.Errorf("failed to update driver status: %w", err)
		}

		// Status history is part of the atomic unit — it must roll back with the rest.
		history := database.DeliveryStatusHistory{
			DeliveryOrderID: deliveryID,
			Status:          database.DeliveryStatusAssigned,
			Notes:           fmt.Sprintf("Driver %s assigned", driver.Name),
			ChangedBy:       "system",
		}
		if err := tx.Create(&history).Error; err != nil {
			return fmt.Errorf("failed to record assignment history: %w", err)
		}

		return nil
	})
	if err != nil {
		return err
	}

	// Notify driver (operational) and customer (lifecycle) only after commit.
	s.notifyDriverAssignment(&deliveryOrder, &driver)
	s.notifyStatusChange(&deliveryOrder, database.DeliveryStatusAssigned)

	return nil
}

// UnassignDriver was removed (DEL-SM-9): it was dead code (no handler routed
// to it) doing unlocked full-row Saves that could resurrect a cancelled
// delivery if ever wired. Driver release happens through the locked lifecycle
// paths (releaseDriverTx / cancelDeliveryLinked).

// CancelDeliveryOrder cancels a delivery order through the linked lifecycle:
// terminal status written once, driver released in the same transaction,
// linked order cancelled and unpaid bill closed (R3). Idempotent — a second
// call on an already-cancelled delivery returns nil without re-emitting or
// overwriting the first cancellation data.
func (s *DeliveryService) CancelDeliveryOrder(id uint, reason string, cancelledBy string) error {
	return s.cancelDeliveryLinked(id, reason, cancelledBy, "cancelled", database.DeliveryStatusCancelled)
}

// DeliveryOrderContactPatch is the scoped operator edit surface for contact,
// address, instructions, and ETA on non-terminal deliveries. All fields are
// optional; only non-nil pointers are applied.
type DeliveryOrderContactPatch struct {
	CustomerName          *string
	CustomerPhone         *string
	CustomerEmail         *string
	Street                *string
	Apartment             *string
	City                  *string
	State                 *string
	PostalCode            *string
	Country               *string
	FormattedAddress      *string
	DeliveryInstructions  *string
	ContactlessDelivery   *bool
	LeaveAtDoor           *bool
	EstimatedDeliveryTime *time.Time
	ChangedBy             string
}

// ErrDeliveryOrderNotEditable is returned when a contact/address patch targets
// a terminal delivery (delivered/cancelled/failed). Operators must not mutate
// guest-visible terminal history without a cancel+recreate path.
var ErrDeliveryOrderNotEditable = errors.New("delivery is terminal and cannot be edited")

// UpdateDeliveryOrderContact applies a scoped operator edit to a non-terminal
// delivery. Writes go through a single Updates map (no full-row Save) and
// append a status-history audit row noting the fields changed.
func (s *DeliveryService) UpdateDeliveryOrderContact(businessID, deliveryID uint, patch DeliveryOrderContactPatch) (*database.DeliveryOrder, error) {
	var order database.DeliveryOrder
	if err := s.db.Where("business_id = ? AND id = ?", businessID, deliveryID).
		First(&order).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("delivery order not found")
		}
		return nil, fmt.Errorf("failed to load delivery for edit: %w", err)
	}
	if order.Status.IsTerminal() {
		return nil, ErrDeliveryOrderNotEditable
	}

	updates := map[string]interface{}{}
	notes := make([]string, 0, 8)

	setStr := func(col, label string, val *string) {
		if val == nil {
			return
		}
		updates[col] = *val
		notes = append(notes, label)
	}
	setStr("customer_name", "customer_name", patch.CustomerName)
	setStr("customer_phone", "customer_phone", patch.CustomerPhone)
	setStr("customer_email", "customer_email", patch.CustomerEmail)
	setStr("delivery_street", "street", patch.Street)
	setStr("delivery_apartment", "apartment", patch.Apartment)
	setStr("delivery_city", "city", patch.City)
	setStr("delivery_state", "state", patch.State)
	setStr("delivery_postal_code", "postal_code", patch.PostalCode)
	setStr("delivery_country", "country", patch.Country)
	setStr("delivery_formatted_address", "formatted_address", patch.FormattedAddress)
	setStr("delivery_instructions", "instructions", patch.DeliveryInstructions)

	if patch.ContactlessDelivery != nil {
		updates["contactless_delivery"] = *patch.ContactlessDelivery
		notes = append(notes, "contactless")
	}
	if patch.LeaveAtDoor != nil {
		updates["leave_at_door"] = *patch.LeaveAtDoor
		notes = append(notes, "leave_at_door")
	}
	if patch.EstimatedDeliveryTime != nil {
		updates["estimated_delivery_time"] = *patch.EstimatedDeliveryTime
		notes = append(notes, "estimated_delivery_time")
	}

	if len(updates) == 0 {
		return s.GetDeliveryOrderByBusiness(businessID, deliveryID)
	}

	changedBy := patch.ChangedBy
	if changedBy == "" {
		changedBy = "operator"
	}

	err := s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Model(&database.DeliveryOrder{}).
			Where("business_id = ? AND id = ? AND status NOT IN ?", businessID, deliveryID,
				[]database.DeliveryStatus{
					database.DeliveryStatusDelivered,
					database.DeliveryStatusCancelled,
					database.DeliveryStatusFailed,
				}).
			Updates(updates)
		if res.Error != nil {
			return fmt.Errorf("failed to patch delivery contact: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return ErrDeliveryOrderNotEditable
		}
		audit := database.DeliveryStatusHistory{
			DeliveryOrderID: deliveryID,
			Status:          order.Status,
			Notes:           "operator edit: " + strings.Join(notes, ", "),
			ChangedBy:       changedBy,
			CreatedAt:       time.Now(),
		}
		if err := tx.Create(&audit).Error; err != nil {
			return fmt.Errorf("failed to write delivery edit audit: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return s.GetDeliveryOrderByBusiness(businessID, deliveryID)
}

// GetAvailableDrivers retrieves all available drivers for a business.
//
// Availability is keyed off the operator-controlled toggles plus the absence of
// an in-flight delivery — NOT the `status` column. There is no production route
// that flips a driver to DriverStatusOnline (the demo generator was the only
// caller), so requiring status="online" left every real driver permanently
// unassignable. A driver is assignable when: they are marked available, their
// record is active, and they are not already on a delivery.
func (s *DeliveryService) GetAvailableDrivers(businessID uint) ([]database.DeliveryDriver, error) {
	var drivers []database.DeliveryDriver
	if err := s.db.Where("business_id = ? AND is_available = ? AND is_active = ? AND current_delivery_id IS NULL",
		businessID, true, true).
		Find(&drivers).Error; err != nil {
		return nil, fmt.Errorf("failed to get available drivers: %w", err)
	}
	return drivers, nil
}

// DEL-SM-9 follow-up: UpdateDriverLocation was deleted. Its handlers were
// removed with DEL-ROUTE-4 (never routed, cross-tenant IDOR risk) and nothing
// else called it — a future driver-app slice must rebuild it business-scoped,
// with column-scoped Updates and a non-terminal-status guard on any
// delivery-side location mirror.

// Helper functions

// GenerateDeliveryNumber returns a unique delivery number in the format DEL-XXXXXXXXXXXXXXXX
// (16 uppercase hex characters). It always uses crypto/rand for entropy; if the OS RNG
// returns an error on the first call it retries once before panicking, so that we never
// silently fall back to a time-seeded or predictable value.
func GenerateDeliveryNumber() string {
	token := make([]byte, 8)
	if _, err := rand.Read(token); err == nil {
		return "DEL-" + strings.ToUpper(hex.EncodeToString(token))
	}
	// Second attempt — the OS RNG transient error is extremely rare; two consecutive
	// failures indicate a systemic problem and we must not silently degrade to a
	// predictable value.
	if _, err := rand.Read(token); err == nil {
		return "DEL-" + strings.ToUpper(hex.EncodeToString(token))
	}
	// Final resort: use UUID v4 (which internally uses crypto/rand) so we still have
	// cryptographic entropy. The format differs from the happy path but remains
	// unique and does not expose timing or sequence information.
	id := uuid.New()
	raw := strings.ToUpper(strings.ReplaceAll(id.String(), "-", ""))
	if len(raw) < 16 {
		raw += strings.Repeat("0", 16-len(raw))
	}
	return "DEL-" + raw[:16]
}

func (s *DeliveryService) generateDeliveryNumber() string {
	return GenerateDeliveryNumber()
}

func (s *DeliveryService) addStatusHistory(deliveryOrderID uint, status database.DeliveryStatus, location *database.Location, notes string, changedBy string) {
	history := database.DeliveryStatusHistory{
		DeliveryOrderID: deliveryOrderID,
		Status:          status,
		Location:        location,
		Notes:           notes,
		ChangedBy:       changedBy,
	}
	s.db.Create(&history)
}

func (s *DeliveryService) notifyBusinessNewDelivery(delivery *database.DeliveryOrder) {
	business, err := database.GetBusinessByID(delivery.BusinessID)
	if err != nil {
		log.Printf("Failed to load business %d for delivery notification: %v", delivery.BusinessID, err)
		return
	}
	// The new row is already inserted, so the cap trips when the 24h count
	// is greater than maxVenueDeliveryNoticesPerDay (the 201st order).
	if venueDeliveryNoticeCapReached(s.noticeDB(), business.ID, time.Now()) {
		return
	}
	title, body := localizedNewDeliveryMessage(
		delivery.DeliveryNumber,
		delivery.CustomerName,
		delivery.DeliveryAddress.Street,
		businessNotificationLocale(business),
	)
	s.sendDeliveryNotification(business, delivery, title, body)
}

func (s *DeliveryService) notifyStatusChange(delivery *database.DeliveryOrder, status database.DeliveryStatus) {
	if delivery.CustomerEmail == "" {
		return
	}
	// Locale-independent gate keeps high-frequency operational updates off the DB.
	if !isGuestVisibleStatus(status) {
		return
	}
	business, err := database.GetBusinessByID(delivery.BusinessID)
	if err != nil {
		log.Printf("Failed to load business %d for status notification: %v", delivery.BusinessID, err)
	}
	locale := guestNotificationLocale(delivery, business)
	title, body, _ := localizedGuestMessage(
		status,
		delivery.CancellationReason,
		locale,
	)
	s.sendDeliveryNotificationToEmail(deliveryCustomerMailOrigin(delivery, ""), delivery.CustomerEmail, delivery.CustomerName, title, body, locale)
}

func (s *DeliveryService) notifyDriverAssignment(delivery *database.DeliveryOrder, driver *database.DeliveryDriver) {
	// Customer-facing assignment email is dispatched by notifyStatusChange(assigned)
	// at the call site. This function only notifies the operator side.
	business, err := database.GetBusinessByID(delivery.BusinessID)
	if err != nil {
		log.Printf("Failed to load business %d for driver assignment notification: %v", delivery.BusinessID, err)
		return
	}
	title, body := localizedOpsDriverAssignedMessage(
		delivery.DeliveryNumber,
		delivery.CustomerName,
		businessNotificationLocale(business),
	)
	s.sendDeliveryNotification(business, delivery, title, body)
}

func (s *DeliveryService) notifyCancellation(delivery *database.DeliveryOrder) {
	business, err := database.GetBusinessByID(delivery.BusinessID)
	if err != nil {
		log.Printf("Failed to load business %d for cancellation notification: %v", delivery.BusinessID, err)
	}

	// Notify customer in their locale.
	if delivery.CustomerEmail != "" {
		locale := guestNotificationLocale(delivery, business)
		title, body, _ := localizedGuestMessage(
			database.DeliveryStatusCancelled,
			delivery.CancellationReason,
			locale,
		)
		s.sendDeliveryNotificationToEmail(deliveryCustomerMailOrigin(delivery, ""), delivery.CustomerEmail, delivery.CustomerName, title, body, locale)
	}

	// Notify the verified owner in the business locale. business.Email is a
	// free-form contact field and is never a venue-notice recipient.
	if business != nil {
		title, body := localizedOpsCancellationMessage(
			delivery.DeliveryNumber,
			delivery.CustomerName,
			businessNotificationLocale(business),
		)
		s.sendDeliveryNotification(business, delivery, title, body)
	}
}

// maxVenueDeliveryNoticesPerDay caps anonymous public-checkout venue notices
// (the new-order email and the Telegram order-created enqueue) for one
// business. The 201st delivery order in a rolling 24h window gets neither.
const maxVenueDeliveryNoticesPerDay = 200

// countRecentDeliveryOrders counts delivery_orders for businessID with
// created_at in the 24 hours before now. It issues a single COUNT.
func countRecentDeliveryOrders(db *gorm.DB, businessID uint, now time.Time) (int64, error) {
	if db == nil {
		return 0, errors.New("delivery notice cap: no database")
	}
	var count int64
	err := db.Model(&database.DeliveryOrder{}).
		Where("business_id = ? AND created_at >= ?", businessID, now.Add(-24*time.Hour)).
		Count(&count).Error
	return count, err
}

// venueDeliveryNoticeCapReached reports whether a venue email for an
// already-inserted delivery order should be skipped. The new row is included
// in the count, so the cap trips when the count is greater than
// maxVenueDeliveryNoticesPerDay. A database error fails closed.
func venueDeliveryNoticeCapReached(db *gorm.DB, businessID uint, now time.Time) bool {
	count, err := countRecentDeliveryOrders(db, businessID, now)
	if err != nil {
		log.Printf("venue delivery notice cap: business_id=%d: %v", businessID, err)
		return true
	}
	return count > int64(maxVenueDeliveryNoticesPerDay)
}

// noticeDB is the handle venue-notice reads use. Unit tests construct a
// service with a nil db and rely on the package test database.
func (s *DeliveryService) noticeDB() *gorm.DB {
	if s != nil && s.db != nil {
		return s.db
	}
	return database.GetDB()
}

// verifiedVenueOwner loads the business owner's email. Venue notices go only
// to that address, and only when it is present and verified. UserID nil, a
// missing user, a blank email, EmailVerified false, or a database error all
// send nothing. The lookup selects only the columns the notice needs.
func verifiedVenueOwner(db *gorm.DB, business *database.Business) (database.User, bool) {
	if business == nil || business.UserID == nil || *business.UserID == 0 {
		return database.User{}, false
	}
	if db == nil {
		log.Printf("venue notice: no database for business %d", business.ID)
		return database.User{}, false
	}
	var owner database.User
	err := db.Select("id", "email", "email_verified", "name").First(&owner, *business.UserID).Error
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("venue notice: load owner user_id=%d business_id=%d: %v", *business.UserID, business.ID, err)
		}
		return database.User{}, false
	}
	owner.Email = strings.TrimSpace(owner.Email)
	if owner.Email == "" || !owner.EmailVerified {
		return database.User{}, false
	}
	return owner, true
}

// sendDeliveryNotification mails a venue notice (new order, driver assigned,
// cancelled) to the business owner's verified email and stamps it so the
// tenant mail budget counts it. business.Email is never the recipient.
func (s *DeliveryService) sendDeliveryNotification(business *database.Business, delivery *database.DeliveryOrder, title, description string) {
	if s.notificationManager == nil || business == nil || delivery == nil {
		return
	}
	owner, ok := verifiedVenueOwner(s.noticeDB(), business)
	if !ok {
		return
	}
	notification := structs.NewNotification(title, description, 0)
	notification.MailOrigin = structs.NotificationMailOrigin{
		BusinessID:      business.ID,
		DeliveryOrderID: delivery.ID,
	}
	s.notificationManager.SendNotification(
		notification,
		structs.User{
			Email: owner.Email,
			Name:  owner.Name,
			NotificationPreferences: structs.NotificationPreferences{
				EmailEnabled:         true,
				TransactionalEnabled: true,
			},
		},
	)
}

// deliveryCustomerMailOrigin stamps mail to the address on a delivery order
// so the tenant mail budget counts it against the venue. purpose is empty for mail a venue action
// sends (accepted, status change, cancelled, expired after an accept, payment
// received) and emails.MailPurposeGuestDeliveryRequest for the order-received
// confirmation an anonymous public checkout triggers, which keeps that mail in
// the guest lane so anonymous checkouts cannot spend the venue's own budget.
func deliveryCustomerMailOrigin(delivery *database.DeliveryOrder, purpose string) structs.NotificationMailOrigin {
	return structs.NotificationMailOrigin{
		BusinessID:      delivery.BusinessID,
		DeliveryOrderID: delivery.ID,
		Purpose:         purpose,
	}
}

func (s *DeliveryService) sendDeliveryNotificationToEmail(origin structs.NotificationMailOrigin, email, name, title, description string, locale NotificationLocale) {
	if s.notificationManager == nil || email == "" {
		return
	}
	notification := structs.NewNotification(title, description, 0)
	notification.MailOrigin = origin
	s.notificationManager.SendNotification(
		notification,
		structs.User{
			Email:            email,
			Name:             name,
			LanguageSelected: string(locale),
			NotificationPreferences: structs.NotificationPreferences{
				EmailEnabled:         true,
				TransactionalEnabled: true,
			},
		},
	)
}

// CreateDriver creates a new delivery driver
func (s *DeliveryService) CreateDriver(driver *database.DeliveryDriver) error {
	if err := s.db.Create(driver).Error; err != nil {
		return fmt.Errorf("failed to create driver: %w", err)
	}
	return nil
}

// GetDriverByBusiness retrieves a driver by ID scoped to a business.
func (s *DeliveryService) GetDriverByBusiness(businessID, driverID uint) (*database.DeliveryDriver, error) {
	var driver database.DeliveryDriver
	if err := s.db.Where("id = ? AND business_id = ?", driverID, businessID).
		Preload("Business").Preload("Staff").First(&driver).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("driver not found")
		}
		return nil, fmt.Errorf("failed to get driver: %w", err)
	}
	return &driver, nil
}

// GetBusinessDrivers retrieves all drivers for a business
func (s *DeliveryService) GetBusinessDrivers(businessID uint) ([]database.DeliveryDriver, error) {
	var drivers []database.DeliveryDriver
	if err := s.db.Where("business_id = ?", businessID).
		Order("name ASC").
		Find(&drivers).Error; err != nil {
		return nil, fmt.Errorf("failed to get business drivers: %w", err)
	}
	return drivers, nil
}

// UpdateDriverByBusiness updates a driver only when it belongs to the given business.
//
// When the update sets the `status` column (the availability toggle syncs it for
// roster-chip coherence), a driver currently on a delivery must NOT be demoted
// from "busy" to online/offline — that status is owned by the assign/release
// lifecycle. In that case the write is guarded so it applies only to a
// not-busy row, and RowsAffected==0 for a busy driver is treated as a
// no-op success (the other fields, if any, are applied on a separate pass).
func (s *DeliveryService) UpdateDriverByBusiness(businessID, driverID uint, updates interface{}) error {
	updateMap, hasStatus := statusBearingUpdate(updates)
	if hasStatus {
		return s.updateDriverGuardingBusyStatus(businessID, driverID, updateMap)
	}

	result := s.db.Model(&database.DeliveryDriver{}).
		Where("id = ? AND business_id = ?", driverID, businessID).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update driver: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("driver not found")
	}
	return nil
}

// statusBearingUpdate reports whether the update is a map that carries a
// "status" key, returning the map for guarded application.
func statusBearingUpdate(updates interface{}) (map[string]interface{}, bool) {
	m, ok := updates.(map[string]interface{})
	if !ok {
		return nil, false
	}
	if _, has := m["status"]; !has {
		return nil, false
	}
	return m, true
}

// updateDriverGuardingBusyStatus applies an availability-toggle update in two
// passes: the non-status fields unconditionally (so the toggle always persists),
// then the status field only when the driver is not "busy" (so a mid-delivery
// driver's busy status is preserved). Driver existence/ownership is verified up
// front so a genuinely missing driver still errors.
func (s *DeliveryService) updateDriverGuardingBusyStatus(businessID, driverID uint, updateMap map[string]interface{}) error {
	var count int64
	if err := s.db.Model(&database.DeliveryDriver{}).
		Where("id = ? AND business_id = ?", driverID, businessID).
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to update driver: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("driver not found")
	}

	status := updateMap["status"]
	nonStatus := make(map[string]interface{}, len(updateMap))
	for k, v := range updateMap {
		if k == "status" {
			continue
		}
		nonStatus[k] = v
	}

	if len(nonStatus) > 0 {
		if err := s.db.Model(&database.DeliveryDriver{}).
			Where("id = ? AND business_id = ?", driverID, businessID).
			Updates(nonStatus).Error; err != nil {
			return fmt.Errorf("failed to update driver: %w", err)
		}
	}

	// Status: skip busy drivers — assign/release owns that value.
	if err := s.db.Model(&database.DeliveryDriver{}).
		Where("id = ? AND business_id = ? AND status <> ?", driverID, businessID, database.DriverStatusBusy).
		Update("status", status).Error; err != nil {
		return fmt.Errorf("failed to update driver: %w", err)
	}
	return nil
}

// DeleteDriverByBusiness deletes a driver only when it belongs to the given business.
func (s *DeliveryService) DeleteDriverByBusiness(businessID, driverID uint) error {
	var driver database.DeliveryDriver
	if err := s.db.Where("id = ? AND business_id = ?", driverID, businessID).First(&driver).Error; err != nil {
		return fmt.Errorf("driver not found: %w", err)
	}

	if driver.CurrentDeliveryID != nil {
		return fmt.Errorf("cannot delete driver with active delivery")
	}

	if err := s.db.Delete(&database.DeliveryDriver{}, driver.ID).Error; err != nil {
		return fmt.Errorf("failed to delete driver: %w", err)
	}
	return nil
}

// defaultDeliverySettings is the in-memory row used when a business has never
// saved settings. Money fields are int64 CENTS: $5.00 fee, $30.00
// free-delivery threshold. (Untyped float literals like 5.0 silently truncate
// to 5 cents here — the DTO divides by 100 and would surface $0.05 in the
// settings form.) DeliveryHoursSameAsBusiness mirrors the column's
// `gorm:"default:true"`. That tag only fires on INSERT, so this in-memory
// default must set it explicitly: UpdateDeliverySettingsDTO patches this
// struct and only overwrites the flag when the caller sends it. Leaving the
// bool zero-value here makes a first-ever save that omits delivery hours (the
// zone editor does exactly that) fail the custom-hours fail-closed check with
// a misleading error.
func defaultDeliverySettings(businessID uint) *database.DeliverySettings {
	return &database.DeliverySettings{
		BusinessID:                  businessID,
		DeliveryEnabled:             false,
		DeliveryHoursSameAsBusiness: true,
		FlatDeliveryFee:             500,
		FreeDeliveryMinimum:         3000,
		// L3-39: no default radius. The field has no operator UI and is
		// no longer distance-gated, so seeding 10km implied a delivery
		// limit that nothing enforces.
		EstimatedPrepTime:    30,
		ExternalPartnerLinks: database.JSONRawMessage("[]"),
	}
}

// GetDeliverySettings retrieves delivery settings for a business
func (s *DeliveryService) GetDeliverySettings(businessID uint) (*database.DeliverySettings, error) {
	var settings database.DeliverySettings
	if err := s.db.Where("business_id = ?", businessID).First(&settings).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return defaultDeliverySettings(businessID), nil
		}
		return nil, fmt.Errorf("failed to get delivery settings: %w", err)
	}

	if len(settings.ExternalPartnerLinks) == 0 || string(settings.ExternalPartnerLinks) == "null" {
		settings.ExternalPartnerLinks = database.JSONRawMessage("[]")
	}

	return &settings, nil
}

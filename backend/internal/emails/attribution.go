package emails

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/resend/resend-go/v3"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/metrics"
)

// Bounce attribution (#562).
//
// A provider delivery webhook carries only the provider's message ID. Before
// this, that ID was thrown away at send time, so an ingested bounce could not
// be traced to the reservation, bill or order whose confirmation just failed to
// arrive — the operator saw a suppression row and nothing else.
//
// The outbox row closes the loop: it stores the origin references captured at
// enqueue time (#562) and the provider message ID captured at
// send time (#551). Attribution therefore has two tiers:
//
//  1. Always: the verdict is written back onto the outbox row, which is indexed
//     by reservation / bill / delivery order, and logged with those references.
//  2. When the recipient is a known customer of the business: a
//     customer_communications row records the outcome, giving
//     CommunicationStatusBounced its first production writer.
//
// Tier 2 is best-effort by design — most guest reservations are placed by an
// email address with no Customer account, and customer_communications requires
// a non-null customer_business_id.

// EmailOrigin identifies the entity whose lifecycle triggered a send.
type EmailOrigin struct {
	BusinessID      uint
	ReservationID   uint
	BillID          uint
	DeliveryOrderID uint
	// Purpose selects extra tenant-budget caps (see tenant_budget.go); it is
	// not persisted on the outbox row.
	Purpose string
	// ReportDuplicate makes a send swallowed by the tenant dedupe window return
	// ErrTenantMailDuplicate instead of nil (see ReportingDuplicates). Not
	// persisted.
	ReportDuplicate bool
}

func (o EmailOrigin) isZero() bool {
	return o.BusinessID == 0 && o.ReservationID == 0 && o.BillID == 0 && o.DeliveryOrderID == 0
}

// entityLabel is the coarse origin kind used for metrics and log lines.
func (o EmailOrigin) entityLabel() string {
	switch {
	case o.ReservationID != 0:
		return "reservation"
	case o.BillID != 0:
		return "bill"
	case o.DeliveryOrderID != 0:
		return "delivery_order"
	case o.BusinessID != 0:
		return "business"
	default:
		return "unattributed"
	}
}

func (o EmailOrigin) logFields() logrus.Fields {
	fields := logrus.Fields{"origin_entity": o.entityLabel()}
	if o.BusinessID != 0 {
		fields["business_id"] = o.BusinessID
	}
	if o.ReservationID != 0 {
		fields["reservation_id"] = o.ReservationID
	}
	if o.BillID != 0 {
		fields["bill_id"] = o.BillID
	}
	if o.DeliveryOrderID != 0 {
		fields["delivery_order_id"] = o.DeliveryOrderID
	}
	return fields
}

// WithOrigin returns a shallow copy of the server that stamps origin references
// onto everything it queues. Callers use it at the send site
// (emailServer.WithOrigin(...).SendReservationConfirmation(...)) so the ~40
// existing sender signatures stay untouched.
func (e *EmailServer) WithOrigin(origin EmailOrigin) *EmailServer {
	if e == nil {
		return nil
	}
	clone := *e
	clone.origin = origin
	return &clone
}

// ForReservation / ForBill are the call shapes that actually exist today;
// delivery mail is attributed through structs.Notification.MailOrigin.
func (e *EmailServer) ForReservation(businessID, reservationID uint) *EmailServer {
	return e.WithOrigin(EmailOrigin{BusinessID: businessID, ReservationID: reservationID})
}

func (e *EmailServer) ForBill(businessID, billID uint) *EmailServer {
	return e.WithOrigin(EmailOrigin{BusinessID: businessID, BillID: billID})
}

// ForReceipt is ForBill for a payment or fiscal receipt to the guest who
// paid: it also stamps MailPurposeReceipt so the tenant budget counts it
// against the receipt-only cross-tenant key.
func (e *EmailServer) ForReceipt(businessID, billID uint) *EmailServer {
	return e.WithOrigin(EmailOrigin{BusinessID: businessID, BillID: billID, Purpose: MailPurposeReceipt})
}

func idPtr(id uint) *uint {
	if id == 0 {
		return nil
	}
	value := id
	return &value
}

func derefID(id *uint) uint {
	if id == nil {
		return 0
	}
	return *id
}

// applyOutboxOrigin copies the server's origin onto a queued row.
func (e *EmailServer) applyOutboxOrigin(row *EmailOutbox) {
	if row == nil || e.origin.isZero() {
		return
	}
	row.BusinessID = idPtr(e.origin.BusinessID)
	row.ReservationID = idPtr(e.origin.ReservationID)
	row.BillID = idPtr(e.origin.BillID)
	row.DeliveryOrderID = idPtr(e.origin.DeliveryOrderID)
}

// deliveryOutcome is the provider verdict projected onto our own vocabulary.
type deliveryOutcome struct {
	// status is the CustomerCommunication status; empty means "informational
	// event, do not change the recorded status".
	status database.CommunicationStatus
	// failure marks an outcome the guest did NOT receive.
	failure bool
}

func classifyDeliveryEvent(eventType string) (deliveryOutcome, bool) {
	switch eventType {
	case resend.EventEmailDelivered:
		return deliveryOutcome{status: database.CommunicationStatusDelivered}, true
	case resend.EventEmailBounced:
		return deliveryOutcome{status: database.CommunicationStatusBounced, failure: true}, true
	case resend.EventEmailFailed, resend.EventEmailSuppressed:
		return deliveryOutcome{status: database.CommunicationStatusFailed, failure: true}, true
	case resend.EventEmailComplained:
		// A complaint means it arrived and the guest reported it as spam. The
		// send is not a failure, but it must be visible on the entity.
		return deliveryOutcome{status: database.CommunicationStatusDelivered}, true
	case resend.EventEmailDeliveryDelayed:
		return deliveryOutcome{}, true
	default:
		return deliveryOutcome{}, false
	}
}

// applyOutboxDeliveryEvent attributes one provider delivery event to the entity
// that triggered the send. An unknown provider message ID (mail sent before
// this shipped, or from another system on the same domain) is a no-op, not an
// error: the event is still recorded by the caller's own bookkeeping.
func applyOutboxDeliveryEvent(tx *gorm.DB, provider, providerMessageID, eventType, recipient, detail string, occurredAt time.Time) error {
	providerMessageID = strings.TrimSpace(providerMessageID)
	if tx == nil || providerMessageID == "" {
		return nil
	}
	outcome, known := classifyDeliveryEvent(eventType)
	if !known {
		return nil
	}

	var row EmailOutbox
	err := tx.Where("provider = ? AND provider_message_id = ?",
		strings.ToLower(strings.TrimSpace(provider)), providerMessageID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("email outbox attribution lookup failed: %w", err)
	}

	updates := map[string]interface{}{
		"delivery_event_at": occurredAt.UTC(),
		"updated_at":        occurredAt.UTC(),
	}
	if outcome.status != "" {
		updates["delivery_status"] = string(outcome.status)
		updates["delivery_detail"] = truncateDeliveryDetail(detail)
	}

	origin := row.Origin()
	communicationID, commErr := upsertCustomerCommunicationIsolated(tx, row, outcome, recipient, detail, occurredAt)
	if commErr != nil {
		// Never fail the webhook over the optional CRM projection: the outbox
		// row below is the durable attribution. The savepoint above keeps the
		// enclosing transaction usable after a failed CRM write.
		logrus.WithFields(origin.logFields()).WithError(commErr).
			Warn("email bounce attribution: customer communication write failed")
	} else if communicationID != 0 && row.CustomerCommunicationID == nil {
		updates["customer_communication_id"] = communicationID
	}

	if err := tx.Model(&EmailOutbox{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
		return fmt.Errorf("email outbox attribution update failed: %w", err)
	}

	metrics.EmailDeliveryAttributed.WithLabelValues(eventType, origin.entityLabel()).Inc()
	if outcome.failure && !origin.isZero() {
		fields := origin.logFields()
		fields["event_type"] = eventType
		fields["template_name"] = row.TemplateName
		fields["provider_message_id"] = providerMessageID
		logrus.WithFields(fields).Warn("transactional email did not reach the guest")
	}
	return nil
}

// upsertCustomerCommunicationIsolated wraps the CRM write in a savepoint. On
// Postgres a failed statement aborts the whole transaction, so the optional
// projection must be able to roll back on its own without taking the delivery
// event and suppression writes down with it.
func upsertCustomerCommunicationIsolated(
	tx *gorm.DB,
	row EmailOutbox,
	outcome deliveryOutcome,
	recipient, detail string,
	occurredAt time.Time,
) (uint, error) {
	const savepoint = "email_attribution_communication"
	if err := tx.SavePoint(savepoint).Error; err != nil {
		return 0, fmt.Errorf("email attribution savepoint failed: %w", err)
	}
	id, err := upsertCustomerCommunication(tx, row, outcome, recipient, detail, occurredAt)
	if err != nil {
		if rollbackErr := tx.RollbackTo(savepoint).Error; rollbackErr != nil {
			// The enclosing transaction is unusable; surface the failure so the
			// provider retries the webhook instead of half-writing it.
			return 0, fmt.Errorf("email attribution rollback failed: %w (original: %v)", rollbackErr, err)
		}
		return 0, err
	}
	return id, nil
}

// upsertCustomerCommunication records the outcome on the business CRM log when
// the recipient is a known customer of the originating business. It returns 0
// when the send cannot be tied to a customer_businesses row, which is the
// normal case for guest reservations placed by a bare email address.
func upsertCustomerCommunication(
	tx *gorm.DB,
	row EmailOutbox,
	outcome deliveryOutcome,
	recipient, detail string,
	occurredAt time.Time,
) (uint, error) {
	if outcome.status == "" {
		return 0, nil
	}
	now := occurredAt.UTC()

	if row.CustomerCommunicationID != nil && *row.CustomerCommunicationID != 0 {
		updates := map[string]interface{}{"status": string(outcome.status), "updated_at": now}
		if outcome.status == database.CommunicationStatusDelivered {
			updates["delivered_at"] = now
		}
		if outcome.failure {
			updates["error_message"] = truncateDeliveryDetail(detail)
		}
		if err := tx.Model(&database.CustomerCommunication{}).
			Where("id = ?", *row.CustomerCommunicationID).Updates(updates).Error; err != nil {
			return 0, err
		}
		return *row.CustomerCommunicationID, nil
	}

	businessID := derefID(row.BusinessID)
	if businessID == 0 {
		return 0, nil
	}
	customerBusinessID, err := resolveCustomerBusiness(tx, businessID, recipient)
	if err != nil {
		return 0, err
	}
	if customerBusinessID == 0 {
		return 0, nil
	}

	record := database.CustomerCommunication{
		BusinessID:         businessID,
		CustomerBusinessID: customerBusinessID,
		Type:               database.CommunicationTypeEmail,
		Status:             outcome.status,
		Subject:            row.Payload.Subject,
		// Content stays the logical template name, never the rendered body: the
		// CRM log must not become a second copy of the guest's message.
		Content:    row.TemplateName,
		SentAt:     row.SentAt,
		CampaignID: strings.TrimSpace(row.Tag),
	}
	if outcome.status == database.CommunicationStatusDelivered {
		record.DeliveredAt = &now
	}
	if outcome.failure {
		record.ErrorMessage = truncateDeliveryDetail(detail)
	}
	// Omit the association structs: this is a log write, not a place to upsert
	// a Business or CustomerBusiness row.
	if err := tx.Omit("Business", "CustomerBusiness").Create(&record).Error; err != nil {
		return 0, err
	}
	return record.ID, nil
}

// resolveCustomerBusiness maps a recipient address to this business's CRM row.
// Returns 0 when the address belongs to no registered customer.
func resolveCustomerBusiness(tx *gorm.DB, businessID uint, recipient string) (uint, error) {
	recipient = strings.ToLower(strings.TrimSpace(recipient))
	if businessID == 0 || recipient == "" {
		return 0, nil
	}
	var customer database.Customer
	err := tx.Select("id").Where("LOWER(email) = ?", recipient).First(&customer).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	var link database.CustomerBusiness
	err = tx.Select("id").Where("customer_id = ? AND business_id = ?", customer.ID, businessID).
		First(&link).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return link.ID, nil
}

func truncateDeliveryDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	if len(detail) > 255 {
		return detail[:255]
	}
	return detail
}

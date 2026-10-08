package database

import "time"

type OperationalAlertType string

const (
	OperationalAlertTypeOrderNew          OperationalAlertType = "order_new"
	OperationalAlertTypeKitchenOrderReady OperationalAlertType = "kitchen_order_ready"
	OperationalAlertTypeReservationNew    OperationalAlertType = "reservation_new"
	// OperationalAlertTypeReservationApproval marks a pending guest request
	// that needs manual approval before its window expires.
	OperationalAlertTypeReservationApproval OperationalAlertType = "reservation_approval"
	OperationalAlertTypeDeliveryNew         OperationalAlertType = "delivery_new"
	OperationalAlertTypeBillNew             OperationalAlertType = "bill_new"
	OperationalAlertTypePaymentRequested    OperationalAlertType = "payment_requested"
	OperationalAlertTypePaymentReceived     OperationalAlertType = "payment_received"
	// OperationalAlertTypePaymentRefundReview marks an out-of-band partial
	// provider refund (e.g. a merchant refunds $10 of a $50 PayPal capture from
	// the PSP dashboard) that we deliberately do NOT auto-reverse, because a full
	// reversal over-reverses the books and a partial mutation here would desync
	// the Payment row vs accounting. The operator reconciles it manually (manual
	// ledger entry / PSP dashboard) — Payverge's in-app refund tooling does full
	// refunds only.
	OperationalAlertTypePaymentRefundReview OperationalAlertType = "payment_refund_review"
	// OperationalAlertTypeServiceCall is a guest at a table asking for a human
	// (water / order / check). Urgent + repeating while the call is inside the
	// seating SLA; past that window it is resolved so the live list and guest
	// chrome go quiet.
	OperationalAlertTypeServiceCall OperationalAlertType = "service_call"
	// OperationalAlertTypeAITakeover fires when a guest writes into a paused,
	// unclaimed AI conversation — a message nobody (bot or human) will answer.
	OperationalAlertTypeAITakeover OperationalAlertType = "ai_takeover"
	// OperationalAlertTypePrintQueueStale fires when a browser-routed print job
	// sits unclaimed past the station SLA (Wave 4 PRINT).
	OperationalAlertTypePrintQueueStale OperationalAlertType = "print_queue_stale"
	// OperationalAlertTypePrintBrowserOffline fires when the business has
	// enabled browser printers and pending browser jobs but no active lease
	// holder (no open operator tab claiming jobs).
	OperationalAlertTypePrintBrowserOffline OperationalAlertType = "print_browser_offline"
	// OperationalAlertTypePaymentReorgSuspected fires when the reorg reconciler
	// (internal/services/reorgwatch) finds a previously-confirmed on-chain USDC
	// payment or crypto refund whose transaction is no longer in the canonical
	// chain — a suspected blockchain reorganization deeper than the confirmation
	// gate. High priority: it may mean a paid bill was never really paid, or a
	// reversed refund never really sent. The reconciler flags + alerts only; the
	// operator verifies on-chain and remediates via the audited void/refund path
	// (we do NOT auto-reverse the ledger on a single RPC read).
	OperationalAlertTypePaymentReorgSuspected OperationalAlertType = "payment_reorg_suspected"
	// OperationalAlertTypeFiscalIssueFailed flags a paid bill whose fiscal
	// receipt could not be queued or permanently failed to issue.
	OperationalAlertTypeFiscalIssueFailed OperationalAlertType = "fiscal_issue_failed"
)

type OperationalAlertResourceType string

const (
	OperationalAlertResourceTypeOrder       OperationalAlertResourceType = "order"
	OperationalAlertResourceTypeBill        OperationalAlertResourceType = "bill"
	OperationalAlertResourceTypeReservation OperationalAlertResourceType = "reservation"
	OperationalAlertResourceTypeDelivery    OperationalAlertResourceType = "delivery"
	OperationalAlertResourceTypePayment     OperationalAlertResourceType = "payment"
	OperationalAlertResourceTypeAltPayment  OperationalAlertResourceType = "alternative_payment"
	OperationalAlertResourceTypeTable       OperationalAlertResourceType = "table"
	// OperationalAlertResourceTypeAIConversation scopes ai_takeover alerts to
	// a single AI Waiter conversation.
	OperationalAlertResourceTypeAIConversation OperationalAlertResourceType = "ai_conversation"
	// OperationalAlertResourceTypePrinter scopes browser-print station health.
	OperationalAlertResourceTypePrinter OperationalAlertResourceType = "printer"
	// OperationalAlertResourceTypePrintJob scopes a single stale print job.
	OperationalAlertResourceTypePrintJob OperationalAlertResourceType = "print_job"
	// OperationalAlertResourceTypePaymentRefund scopes a suspected-reorg alert to
	// a payment_refunds row. It is distinct from ...TypePayment so a payment and a
	// refund that share the same numeric id in one business never collide on the
	// (business, alert_type, resource_type, resource_id) alert dedup key — the two
	// tables have independent id sequences.
	OperationalAlertResourceTypePaymentRefund OperationalAlertResourceType = "payment_refund"
	// OperationalAlertResourceTypeWebhookEvent scopes a payment-review alert to
	// a webhook_events row when the provider event names no bill (for example
	// a card dispute whose capture was never recorded).
	OperationalAlertResourceTypeWebhookEvent OperationalAlertResourceType = "webhook_event"
)

type OperationalAlertStatus string

const (
	OperationalAlertStatusOpen      OperationalAlertStatus = "open"
	OperationalAlertStatusClaimed   OperationalAlertStatus = "claimed"
	OperationalAlertStatusResolved  OperationalAlertStatus = "resolved"
	OperationalAlertStatusDismissed OperationalAlertStatus = "dismissed"
)

type OperationalAlertPriority string

const (
	OperationalAlertPriorityLow    OperationalAlertPriority = "low"
	OperationalAlertPriorityNormal OperationalAlertPriority = "normal"
	OperationalAlertPriorityHigh   OperationalAlertPriority = "high"
	OperationalAlertPriorityUrgent OperationalAlertPriority = "urgent"
)

type OperationalAlertEventType string

const (
	OperationalAlertEventTypeCreated       OperationalAlertEventType = "created"
	OperationalAlertEventTypeClaimed       OperationalAlertEventType = "claimed"
	OperationalAlertEventTypeResolved      OperationalAlertEventType = "resolved"
	OperationalAlertEventTypeDismissed     OperationalAlertEventType = "dismissed"
	OperationalAlertEventTypeReopened      OperationalAlertEventType = "reopened"
	OperationalAlertEventTypeStatusChanged OperationalAlertEventType = "status_changed"
	OperationalAlertEventTypeSnoozed       OperationalAlertEventType = "snoozed"
)

type OperationalAlert struct {
	ID               uint                         `gorm:"primaryKey" json:"id"`
	BusinessID       uint                         `gorm:"index;not null" json:"business_id"`
	AlertType        OperationalAlertType         `gorm:"type:text;not null" json:"alert_type"`
	ResourceType     OperationalAlertResourceType `gorm:"type:text;not null" json:"resource_type"`
	ResourceID       int64                        `gorm:"not null" json:"resource_id"`
	Status           OperationalAlertStatus       `gorm:"type:text;not null;default:'open';index" json:"status"`
	Priority         OperationalAlertPriority     `gorm:"type:text;not null;default:'normal'" json:"priority"`
	Title            string                       `gorm:"type:text;not null;default:''" json:"title"`
	Body             string                       `gorm:"type:text;not null;default:''" json:"body"`
	ClaimedByStaffID *uint                        `gorm:"index" json:"claimed_by_staff_id,omitempty"`
	ClaimedByUserID  *uint                        `gorm:"index" json:"claimed_by_user_id,omitempty"`
	ClaimedByName    string                       `gorm:"type:text;not null;default:''" json:"claimed_by_name"`
	ClaimedAt        *time.Time                   `json:"claimed_at,omitempty"`
	ResolvedAt       *time.Time                   `json:"resolved_at,omitempty"`
	SnoozedUntil     *time.Time                   `gorm:"index" json:"snoozed_until,omitempty"`
	LastEventAt      time.Time                    `gorm:"not null;default:CURRENT_TIMESTAMP" json:"last_event_at"`
	Metadata         JSONRawMessage               `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt        time.Time                    `json:"created_at"`
	UpdatedAt        time.Time                    `json:"updated_at"`

	// Business is GORM-only — zero embeds dumped ~3KB/row on alerts list (FIND-038).
	Business       Business                `gorm:"foreignKey:BusinessID;constraint:OnDelete:CASCADE" json:"-"`
	ClaimedByStaff *Staff                  `gorm:"foreignKey:ClaimedByStaffID;constraint:OnDelete:SET NULL" json:"claimed_by_staff,omitempty"`
	ClaimedByUser  *User                   `gorm:"foreignKey:ClaimedByUserID;constraint:OnDelete:SET NULL" json:"claimed_by_user,omitempty"`
	Events         []OperationalAlertEvent `gorm:"foreignKey:AlertID;constraint:OnDelete:CASCADE" json:"events,omitempty"`
}

func (OperationalAlert) TableName() string { return "operational_alerts" }

type OperationalAlertEvent struct {
	ID           uint                      `gorm:"primaryKey" json:"id"`
	AlertID      uint                      `gorm:"index;not null" json:"alert_id"`
	BusinessID   uint                      `gorm:"index;not null" json:"business_id"`
	EventType    OperationalAlertEventType `gorm:"type:text;not null" json:"event_type"`
	ActorStaffID *uint                     `gorm:"index" json:"actor_staff_id,omitempty"`
	ActorUserID  *uint                     `gorm:"index" json:"actor_user_id,omitempty"`
	ActorName    string                    `gorm:"type:text;not null;default:''" json:"actor_name"`
	Metadata     JSONRawMessage            `gorm:"type:jsonb;not null;default:'{}'" json:"metadata"`
	CreatedAt    time.Time                 `gorm:"not null;default:CURRENT_TIMESTAMP" json:"created_at"`

	Alert      OperationalAlert `gorm:"foreignKey:AlertID;constraint:OnDelete:CASCADE" json:"alert,omitempty"`
	Business   Business         `gorm:"foreignKey:BusinessID;constraint:OnDelete:CASCADE" json:"-"`
	ActorStaff *Staff           `gorm:"foreignKey:ActorStaffID;constraint:OnDelete:SET NULL" json:"actor_staff,omitempty"`
	ActorUser  *User            `gorm:"foreignKey:ActorUserID;constraint:OnDelete:SET NULL" json:"actor_user,omitempty"`
}

func (OperationalAlertEvent) TableName() string { return "operational_alert_events" }

type AlertTypeSetting struct {
	Enabled   bool                     `json:"enabled"`
	Repeating bool                     `json:"repeating"`
	Priority  OperationalAlertPriority `json:"priority"`
}

type OperationalAlertEventSettings struct {
	OrderNew            AlertTypeSetting `json:"order_new"`
	KitchenOrderReady   AlertTypeSetting `json:"kitchen_order_ready"`
	ReservationNew      AlertTypeSetting `json:"reservation_new"`
	ReservationApproval AlertTypeSetting `json:"reservation_approval"`
	DeliveryNew         AlertTypeSetting `json:"delivery_new"`
	BillNew             AlertTypeSetting `json:"bill_new"`
	PaymentRequested    AlertTypeSetting `json:"payment_requested"`
	PaymentReceived     AlertTypeSetting `json:"payment_received"`
	// PaymentRefundReview is the sound/notification preference for
	// payment_refund_review alerts (out-of-band partial provider refunds that
	// need manual reconciliation). Without this field the wire settings map
	// never carries the type, so the FE could neither enable alerts for it nor
	// render it in the settings UI.
	PaymentRefundReview AlertTypeSetting `json:"payment_refund_review"`
	ServiceCall         AlertTypeSetting `json:"service_call"`
	AITakeover          AlertTypeSetting `json:"ai_takeover"`
}

type BusinessAlertSettings struct {
	ID                          uint `gorm:"primaryKey" json:"id"`
	BusinessID                  uint `gorm:"uniqueIndex;not null" json:"business_id"`
	Enabled                     bool `gorm:"not null;default:true" json:"enabled"`
	BrowserNotificationsEnabled bool `gorm:"not null;default:true" json:"browser_notifications_enabled"`
	// Sound is opt-in: a factory-default 8s looping alarm was a dinner-service
	// nightmare (#267). Column default false; operators enable explicitly.
	SoundEnabled          bool                          `gorm:"not null;default:false" json:"sound_enabled"`
	Volume                float64                       `gorm:"not null;default:0.8" json:"volume"`
	RepeatIntervalSeconds int                           `gorm:"not null;default:30" json:"repeat_interval_seconds"`
	EventSettings         OperationalAlertEventSettings `gorm:"serializer:json;type:jsonb;not null;default:'{}'" json:"event_settings"`
	CreatedAt             time.Time                     `json:"created_at"`
	UpdatedAt             time.Time                     `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID;constraint:OnDelete:CASCADE" json:"-"`
}

func (BusinessAlertSettings) TableName() string { return "business_alert_settings" }

// DefaultBusinessAlertRepeatIntervalSeconds is the dinner-safe repeat cadence
// for new businesses (was 8s). Keep in sync with the genesis column
// defaults and the FE normalizeOperationalSettings fallback.
const DefaultBusinessAlertRepeatIntervalSeconds = 30

// MinBusinessAlertRepeatIntervalSeconds is the floor operators can set. Values
// below this (the old 3–8s factory range) turn the floor into a looping alarm.
const MinBusinessAlertRepeatIntervalSeconds = 30

// MaxBusinessAlertRepeatIntervalSeconds is the slider/API ceiling.
const MaxBusinessAlertRepeatIntervalSeconds = 60

func DefaultBusinessAlertSettings(businessID uint) BusinessAlertSettings {
	urgentRepeating := AlertTypeSetting{
		Enabled:   true,
		Repeating: true,
		Priority:  OperationalAlertPriorityUrgent,
	}
	normalSingle := AlertTypeSetting{
		Enabled:   true,
		Repeating: false,
		Priority:  OperationalAlertPriorityNormal,
	}
	highSingle := AlertTypeSetting{
		Enabled:   true,
		Repeating: false,
		Priority:  OperationalAlertPriorityHigh,
	}

	return BusinessAlertSettings{
		BusinessID:                  businessID,
		Enabled:                     true,
		BrowserNotificationsEnabled: true,
		// Opt-in alarm: silent until staff explicitly enables sound (#267).
		SoundEnabled:          false,
		Volume:                0.8,
		RepeatIntervalSeconds: DefaultBusinessAlertRepeatIntervalSeconds,
		EventSettings: OperationalAlertEventSettings{
			OrderNew:          urgentRepeating,
			KitchenOrderReady: urgentRepeating,
			// reservation_new is no longer a blanket alarm: the alert itself
			// carries time-aware priority (urgent only when the party arrives
			// within 2h), so the default stops repeating. reservation_approval
			// keeps the alarm semantics — it has a real deadline.
			ReservationNew:      normalSingle,
			ReservationApproval: urgentRepeating,
			DeliveryNew:         urgentRepeating,
			BillNew:             normalSingle,
			PaymentRequested:    normalSingle,
			PaymentReceived:     normalSingle,
			// Matches the FE fallback in NotificationPreferencesTab: a refund
			// needing manual reconciliation is important (high) but has no
			// realtime deadline, so it does not repeat.
			PaymentRefundReview: highSingle,
			ServiceCall:         urgentRepeating,
			AITakeover:          urgentRepeating,
		},
	}
}

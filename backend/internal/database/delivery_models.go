package database

import (
	"encoding/json"
	"strings"
	"time"
)

// MarshalJSON emits a slim driver shape when the row was preloaded with only
// dispatch/tracking summary columns (id, business_id?, name, phone, email?).
// A full default marshal invents is_active:false, zero KPIs, and empty vehicle
// fields that look authoritative on the operator dispatch list. (FIND-044)
func (d DeliveryDriver) MarshalJSON() ([]byte, error) {
	if d.ID != 0 && d.CreatedAt.IsZero() && d.Status == "" && d.VehicleType == "" {
		return json.Marshal(struct {
			ID         uint   `json:"id"`
			BusinessID uint   `json:"business_id,omitempty"`
			Name       string `json:"name"`
			Phone      string `json:"phone"`
			Email      string `json:"email,omitempty"`
		}{
			ID:         d.ID,
			BusinessID: d.BusinessID,
			Name:       d.Name,
			Phone:      d.Phone,
			Email:      d.Email,
		})
	}
	type alias DeliveryDriver
	return json.Marshal(alias(d))
}

// DeliveryType represents the type of delivery service
type DeliveryType string

const (
	DeliveryTypeInHouse   DeliveryType = "in_house"
	DeliveryTypeUberEats  DeliveryType = "uber_eats"
	DeliveryTypeDoorDash  DeliveryType = "doordash"
	DeliveryTypeGrubHub   DeliveryType = "grubhub"
	DeliveryTypePostmates DeliveryType = "postmates"
)

// DeliveryStatus represents the current status of a delivery
type DeliveryStatus string

const (
	DeliveryStatusPending   DeliveryStatus = "pending"
	DeliveryStatusConfirmed DeliveryStatus = "confirmed"
	DeliveryStatusPreparing DeliveryStatus = "preparing"
	DeliveryStatusReady     DeliveryStatus = "ready"
	DeliveryStatusAssigned  DeliveryStatus = "assigned"
	DeliveryStatusPickedUp  DeliveryStatus = "picked_up"
	DeliveryStatusInTransit DeliveryStatus = "in_transit"
	DeliveryStatusNearby    DeliveryStatus = "nearby"
	DeliveryStatusDelivered DeliveryStatus = "delivered"
	DeliveryStatusCancelled DeliveryStatus = "cancelled"
	DeliveryStatusFailed    DeliveryStatus = "failed"
)

// IsValid reports whether s is a recognized delivery status. Used to reject
// arbitrary status strings on write/filter paths so the lifecycle timeline,
// analytics, and notification branching cannot be corrupted.
func (s DeliveryStatus) IsValid() bool {
	switch s {
	case DeliveryStatusPending, DeliveryStatusConfirmed, DeliveryStatusPreparing,
		DeliveryStatusReady, DeliveryStatusAssigned, DeliveryStatusPickedUp,
		DeliveryStatusInTransit, DeliveryStatusNearby, DeliveryStatusDelivered,
		DeliveryStatusCancelled, DeliveryStatusFailed:
		return true
	default:
		return false
	}
}

// IsTerminal reports whether the status is a final lifecycle state. Transitions
// out of a terminal state (e.g. delivered → pending, cancelled → assigned) are
// invalid: they corrupt the lifecycle timeline and the driver-performance KPIs,
// which key off delivered/cancelled/failed.
func (s DeliveryStatus) IsTerminal() bool {
	switch s {
	case DeliveryStatusDelivered, DeliveryStatusCancelled, DeliveryStatusFailed:
		return true
	default:
		return false
	}
}

// DriverStatus represents the current status of a driver
type DriverStatus string

const (
	DriverStatusOffline DriverStatus = "offline"
	DriverStatusOnline  DriverStatus = "online"
	DriverStatusBusy    DriverStatus = "busy"
	DriverStatusOnBreak DriverStatus = "on_break"
)

// DeliveryPaymentMode controls when a guest delivery order is paid.
type DeliveryPaymentMode string

const (
	// DeliveryPaymentOnline — guest pays online after staff acceptance,
	// before the kitchen starts. No refunds exist: money moves only
	// post-acceptance.
	DeliveryPaymentOnline DeliveryPaymentMode = "online"
	// DeliveryPaymentCashOnDelivery — kitchen starts at acceptance; the
	// guest pays the driver in cash and staff settle the bill at handoff.
	DeliveryPaymentCashOnDelivery DeliveryPaymentMode = "cash_on_delivery"
)

// IsValid reports whether m is a recognized delivery payment mode. Used to
// reject arbitrary mode strings on write paths so the payment lifecycle cannot
// be set to an undefined state.
func (m DeliveryPaymentMode) IsValid() bool {
	return m == DeliveryPaymentOnline || m == DeliveryPaymentCashOnDelivery
}

// VehicleType represents the type of vehicle used for delivery
type VehicleType string

const (
	VehicleTypeBicycle    VehicleType = "bicycle"
	VehicleTypeScooter    VehicleType = "scooter"
	VehicleTypeMotorcycle VehicleType = "motorcycle"
	VehicleTypeCar        VehicleType = "car"
	VehicleTypeVan        VehicleType = "van"
)

// IsValid reports whether v is a recognized vehicle type. An empty value is
// treated as "unset" and is considered valid so callers may omit it.
func (v VehicleType) IsValid() bool {
	switch v {
	case "", VehicleTypeBicycle, VehicleTypeScooter, VehicleTypeMotorcycle,
		VehicleTypeCar, VehicleTypeVan:
		return true
	default:
		return false
	}
}

// DeliveryAddress represents a delivery address
type DeliveryAddress struct {
	Street           string `gorm:"column:street" json:"street"`
	Apartment        string `gorm:"column:apartment" json:"apartment"`
	City             string `gorm:"column:city" json:"city"`
	State            string `gorm:"column:state" json:"state"`
	PostalCode       string `gorm:"column:postal_code" json:"postal_code"`
	Country          string `gorm:"column:country" json:"country"`
	FormattedAddress string `gorm:"column:formatted_address" json:"formatted_address"`
}

// Location represents a geographic location
type Location struct {
	Latitude  float64    `gorm:"column:latitude" json:"latitude"`
	Longitude float64    `gorm:"column:longitude" json:"longitude"`
	Timestamp *time.Time `gorm:"column:timestamp" json:"timestamp,omitempty"`
}

// DeliveryOrder represents a delivery order
type DeliveryOrder struct {
	ID         uint  `gorm:"primaryKey" json:"id"`
	BusinessID uint  `gorm:"index;not null" json:"business_id"`
	BillID     uint  `gorm:"index;not null" json:"bill_id"`
	OrderID    *uint `gorm:"index" json:"order_id"`
	CustomerID *uint `gorm:"index" json:"customer_id"`
	ZoneID     *uint `gorm:"index" json:"zone_id"`

	// Delivery Details
	DeliveryNumber  string         `gorm:"uniqueIndex;not null" json:"delivery_number"`
	DeliveryType    DeliveryType   `gorm:"not null" json:"delivery_type"`
	Status          DeliveryStatus `gorm:"not null;default:'pending'" json:"status"`
	Priority        OrderPriority  `json:"priority"`
	FulfillmentMode string         `gorm:"default:'in_house'" json:"fulfillment_mode"`

	// Driver Assignment
	DriverID   *uint      `gorm:"index" json:"driver_id"`
	AssignedAt *time.Time `json:"assigned_at"`

	// Customer Information
	CustomerName   string `gorm:"not null" json:"customer_name"`
	CustomerPhone  string `gorm:"not null" json:"customer_phone"`
	CustomerEmail  string `json:"customer_email"`
	CustomerLocale string `gorm:"default:''" json:"customer_locale"`

	// Delivery Address
	DeliveryAddress DeliveryAddress `gorm:"embedded;embeddedPrefix:delivery_" json:"delivery_address"`

	// Location Tracking
	PickupLocation  Location  `gorm:"embedded;embeddedPrefix:pickup_" json:"pickup_location"`
	DropoffLocation Location  `gorm:"embedded;embeddedPrefix:dropoff_" json:"dropoff_location"`
	CurrentLocation *Location `gorm:"embedded;embeddedPrefix:current_" json:"current_location,omitempty"`

	// Timing
	EstimatedPickupTime   *time.Time `json:"estimated_pickup_time"`
	ActualPickupTime      *time.Time `json:"actual_pickup_time"`
	EstimatedDeliveryTime *time.Time `json:"estimated_delivery_time"`
	ActualDeliveryTime    *time.Time `json:"actual_delivery_time"`

	// Delivery Fees (stored as cents; emitted as dollars via MarshalJSON)
	DeliveryFee int64 `json:"delivery_fee"`
	DriverTip   int64 `gorm:"default:0" json:"driver_tip"`
	PlatformFee int64 `json:"platform_fee"`

	// Special Instructions
	DeliveryInstructions string `gorm:"type:text" json:"delivery_instructions"`
	ContactlessDelivery  bool   `gorm:"default:false" json:"contactless_delivery"`
	LeaveAtDoor          bool   `gorm:"default:false" json:"leave_at_door"`

	// Proof of Delivery
	SignatureURL  string         `json:"signature_url"`
	PhotoURL      string         `json:"photo_url"`
	DeliveryCode  string         `json:"delivery_code"`
	QuoteMetadata JSONRawMessage `gorm:"type:jsonb;default:'{}'" json:"quote_metadata"`
	CutoffAt      *time.Time     `json:"cutoff_at,omitempty"`
	// PaymentExpiresAt is set when a prepay-mode order is accepted; the
	// expiry job cancels the order if the bill is still unpaid past it.
	PaymentExpiresAt *time.Time `json:"payment_expires_at,omitempty"`
	// PaymentModeStored snapshots the EFFECTIVE payment mode
	// ("online"/"cash_on_delivery") that was in force when the order was
	// created, so operators can see which in-flight orders were placed under
	// a mode that differs from the current settings. Rows created before the
	// column exists stay "" = unknown (no backfill, no mismatch signal).
	PaymentModeStored string `gorm:"column:payment_mode_stored;default:''" json:"payment_mode_stored"`

	// Third-Party Integration
	ExternalOrderID     string `json:"external_order_id"`
	ExternalTrackingURL string `json:"external_tracking_url"`

	// Cancellation
	CancelledAt        *time.Time `json:"cancelled_at"`
	CancelledBy        string     `json:"cancelled_by"`
	CancellationReason string     `json:"cancellation_reason"`

	// Dispatcher claim (one operator at a time). Nil ClaimedByStaffID with empty
	// claimed_at = unclaimed. Mirrors AiWaiterConversation claim columns.
	ClaimedByStaffID *uint      `gorm:"index" json:"claimed_by_staff_id,omitempty"`
	ClaimedByName    string     `json:"claimed_by_name,omitempty"`
	ClaimedByRole    string     `json:"claimed_by_role,omitempty"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`

	// Ratings & Feedback
	CustomerRating   *int   `json:"customer_rating"`
	CustomerFeedback string `json:"customer_feedback"`
	DriverRating     *int   `json:"driver_rating"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Dispatch list/detail projection (not columns). Hydrated from the bill
	// total + business currency/coords so the board can render money and map
	// pins without a full Bill/Business preload.
	//
	// The total stays in CENTS here on purpose: the money wire contract has
	// exactly one conversion site per model, and for DeliveryOrder that site is
	// MarshalJSON. Hydrators must never pre-divide by 100.
	DispatchTotalCents *int64 `gorm:"-" json:"-"`
	DispatchCurrency   string `gorm:"-" json:"-"`

	// Relationships
	// Business is GORM-only on the model tag; DeliveryOrder.MarshalJSON optionally
	// emits a scrubbed billPublicBusiness when the relation is actually loaded.
	// Zero-value embeds used to dump ~3KB empty business per nested driver row.
	Business      Business                `gorm:"foreignKey:BusinessID" json:"-"`
	Bill          Bill                    `gorm:"foreignKey:BillID" json:"bill,omitempty"`
	Order         *Order                  `gorm:"foreignKey:OrderID" json:"order,omitempty"`
	Driver        *DeliveryDriver         `gorm:"foreignKey:DriverID" json:"driver,omitempty"`
	Customer      *Customer               `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	Zone          *DeliveryZone           `gorm:"foreignKey:ZoneID" json:"zone,omitempty"`
	StatusHistory []DeliveryStatusHistory `gorm:"foreignKey:DeliveryOrderID" json:"status_history,omitempty"`
}

// DeliveryDriver represents a delivery driver
type DeliveryDriver struct {
	ID         uint  `gorm:"primaryKey" json:"id"`
	BusinessID uint  `gorm:"index;not null" json:"business_id"`
	StaffID    *uint `gorm:"index" json:"staff_id"`

	// Driver Details
	Name          string      `gorm:"not null" json:"name"`
	Phone         string      `gorm:"not null" json:"phone"`
	Email         string      `json:"email"`
	LicenseNumber string      `json:"license_number"`
	VehicleType   VehicleType `json:"vehicle_type"`
	VehiclePlate  string      `json:"vehicle_plate"`

	// Status & Availability
	Status          DriverStatus `gorm:"default:'offline'" json:"status"`
	IsAvailable     bool         `gorm:"default:true" json:"is_available"`
	CurrentLocation *Location    `gorm:"embedded;embeddedPrefix:current_" json:"current_location,omitempty"`

	// Performance Metrics
	TotalDeliveries     int     `gorm:"default:0" json:"total_deliveries"`
	CompletedDeliveries int     `gorm:"default:0" json:"completed_deliveries"`
	AverageRating       float64 `gorm:"default:0" json:"average_rating"`
	TotalEarnings       float64 `gorm:"default:0" json:"total_earnings"`

	// Active Delivery
	CurrentDeliveryID *uint `json:"current_delivery_id"`

	// Account Status
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	LastActiveAt *time.Time `json:"last_active_at"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships — Business never on the wire (FIND-038 nested bloat on dispatch list).
	Business   Business        `gorm:"foreignKey:BusinessID" json:"-"`
	Staff      *Staff          `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	Deliveries []DeliveryOrder `gorm:"foreignKey:DriverID" json:"deliveries,omitempty"`
}

// DeliveryZone represents a delivery coverage area
type DeliveryZone struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	BusinessID  uint   `gorm:"index;not null" json:"business_id"`
	Name        string `gorm:"not null" json:"name"`
	Description string `json:"description"`

	// Geographic Boundaries (GeoJSON polygon)
	Boundaries string `gorm:"type:text" json:"boundaries"`

	// Delivery Settings (money fields stored as cents; emitted as dollars via MarshalJSON)
	DeliveryFee         int64 `json:"delivery_fee"`
	MinimumOrderAmount  int64 `json:"minimum_order_amount"`
	EstimatedTime       int   `json:"estimated_time"`
	Priority            int   `gorm:"default:0" json:"priority"`
	CutoffBufferMinutes int   `gorm:"default:0" json:"cutoff_buffer_minutes"`
	IsActive            bool  `gorm:"default:true" json:"is_active"`

	// Operating Hours
	OperatingHours string `gorm:"type:text" json:"operating_hours"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// DeliveryStatusHistory represents the audit trail of delivery status changes
type DeliveryStatusHistory struct {
	ID              uint           `gorm:"primaryKey" json:"id"`
	DeliveryOrderID uint           `gorm:"index;not null" json:"delivery_order_id"`
	Status          DeliveryStatus `gorm:"not null" json:"status"`
	Location        *Location      `gorm:"embedded" json:"location,omitempty"`
	Notes           string         `json:"notes"`
	ChangedBy       string         `json:"changed_by"`
	CreatedAt       time.Time      `json:"created_at"`

	DeliveryOrder DeliveryOrder `gorm:"foreignKey:DeliveryOrderID" json:"delivery_order,omitempty"`
}

// DeliverySettings represents business-level delivery configuration
type DeliverySettings struct {
	ID         uint `gorm:"primaryKey" json:"id"`
	BusinessID uint `gorm:"unique;not null" json:"business_id"`

	// Feature Toggles
	DeliveryEnabled        bool `gorm:"default:false" json:"delivery_enabled"`
	InHouseDeliveryEnabled bool `gorm:"default:false" json:"in_house_delivery_enabled"`
	ThirdPartyEnabled      bool `gorm:"default:false" json:"third_party_enabled"`

	// Third-Party Integrations
	UberEatsEnabled bool `gorm:"default:false" json:"uber_eats_enabled"`
	DoordashEnabled bool `gorm:"column:doordash_enabled;default:false" json:"doordash_enabled"`
	GrubhubEnabled  bool `gorm:"column:grubhub_enabled;default:false" json:"grubhub_enabled"`

	// API Credentials (should be encrypted in production). json:"-" so they can
	// never serialize into an API response: GetDeliverySettings already returns a
	// curated DTO that omits them, but hiding them on the model defuses the
	// landmine if any future handler returns the raw model. GORM keys off the
	// gorm column tag (not json), and these are set via UpdateDeliverySettingsInput,
	// never JSON-unmarshalled into the model, so hiding them changes nothing else.
	UberEatsAPIKey string `gorm:"column:uber_eats_api_key" json:"-"`
	DoordashAPIKey string `gorm:"column:doordash_api_key" json:"-"`
	GrubhubAPIKey  string `gorm:"column:grubhub_api_key" json:"-"`

	// Global Settings
	// PaymentMode: "online" | "cash_on_delivery" | "" (unset → derived:
	// online when the business has an online payment method, else COD).
	PaymentMode string `gorm:"column:payment_mode;default:''" json:"payment_mode"`
	// Money fields stored as cents; emitted as dollars via MarshalJSON.
	FlatDeliveryFee             int64          `gorm:"column:default_delivery_fee" json:"flat_delivery_fee"`
	FreeDeliveryMinimum         int64          `gorm:"column:free_delivery_threshold" json:"free_delivery_minimum"`
	MinimumOrderAmount          int64          `json:"minimum_order_amount"`
	DeliveryRadius              float64        `gorm:"column:max_delivery_radius" json:"delivery_radius"`
	EstimatedPrepTime           int            `json:"estimated_prep_time"`
	MaxConcurrentDeliveries     int            `gorm:"default:5" json:"max_concurrent_deliveries"`
	DeliveryHoursSameAsBusiness bool           `gorm:"default:true" json:"delivery_hours_same_as_business"`
	DeliveryStartTime           string         `json:"delivery_start_time"`
	DeliveryEndTime             string         `json:"delivery_end_time"`
	DeliveryInstructions        string         `gorm:"type:text" json:"delivery_instructions"`
	DeliveryZones               JSONRawMessage `gorm:"type:jsonb" json:"delivery_zones"` // JSON array of zones
	ExternalPartnerLinks        JSONRawMessage `gorm:"type:jsonb;default:'[]'" json:"external_partner_links"`

	// Auto-Assignment
	AutoAssignDrivers bool `gorm:"default:false" json:"auto_assign_drivers"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// TableName methods
func (DeliveryOrder) TableName() string {
	return "delivery_orders"
}

func (DeliveryDriver) TableName() string {
	return "delivery_drivers"
}

func (DeliveryZone) TableName() string {
	return "delivery_zones"
}

func (DeliveryStatusHistory) TableName() string {
	return "delivery_status_history"
}

func (DeliverySettings) TableName() string {
	return "delivery_settings"
}

// ResolveDispatchCurrency resolves the currency for the dispatch list/detail
// projection, which reads the currency columns off a JOIN instead of a fully
// preloaded Business row.
//
// It delegates to the same resolver the rest of the app uses so the dispatch
// board can never disagree with bills/orders: an explicitly set display
// currency wins, default_currency is the fallback, and USD is the floor.
// businesses.default_currency has no DB default, so without the floor every
// business that never picked a currency would still render a blank chip.
func ResolveDispatchCurrency(displayCurrency, defaultCurrency string) string {
	return resolveBusinessCurrency("", Business{
		DisplayCurrency: strings.TrimSpace(displayCurrency),
		DefaultCurrency: strings.TrimSpace(defaultCurrency),
	})
}

package database

import (
	"encoding/json"
	"math"
	"time"

	"github.com/stdevmac/payverge/backend/internal/structs"
)

// User represents the users table
type User struct {
	ID uint `gorm:"primaryKey" json:"id"`

	// Primary identifier - Email is now the main identifier for new users
	Email string `gorm:"uniqueIndex" json:"email"` // Primary identifier for email/OAuth users

	// Wallet address - Optional, can be linked later
	Address string `gorm:"index" json:"address"` // Optional wallet address (linked later)

	// User profile
	Name     string `json:"name"`     // User's display name
	Username string `json:"username"` // Optional username
	Picture  string `json:"picture"`  // Profile picture URL (from OAuth provider)
	Role     string `gorm:"default:user" json:"role"`

	// Authentication method tracking
	AuthMethod string `json:"auth_method"` // "email", "google", "wallet" (primary auth method)

	// OAuth provider IDs (for linking multiple providers)
	GoogleID string `gorm:"index" json:"google_id"` // Google OAuth user ID

	// Email verification
	EmailVerified bool `gorm:"default:false" json:"email_verified"`

	// Notification preferences
	NotificationPreferences structs.NotificationPreferences `gorm:"embedded" json:"notification_preferences"`
	LanguageSelected        string                          `json:"language_selected"`

	// Account deletion (IMP-20: GDPR / UAE PDPL self-service)
	// DeletedAt is set immediately on a delete request — DB stays the source of truth.
	// DeletionScheduledAt is now() + 30d. After it, the account erasure janitor
	// (services.RunAccountErasure) anonymizes identifiers and login secrets in
	// place; rows and business records stay for the retention exceptions.
	DeletedAt           *time.Time `gorm:"index" json:"deleted_at,omitempty"`
	DeletionScheduledAt *time.Time `gorm:"index" json:"deletion_scheduled_at,omitempty"`
	DeletionReason      string     `json:"deletion_reason,omitempty"`

	// Activation attribution.
	// SignupSource is how the account arrived (organic, concierge, utm_*, …).
	// ActivatedAt is stamped when registration completes successfully.
	SignupSource string     `gorm:"size:64" json:"signup_source,omitempty"`
	ActivatedAt  *time.Time `json:"activated_at,omitempty"`

	// Timestamps
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ErrorLog represents the error_logs table
type ErrorLog struct {
	ID             uint                   `gorm:"primaryKey" json:"id"`
	CreatedAt      time.Time              `json:"created_at"`
	Timestamp      time.Time              `gorm:"index" json:"timestamp"`
	Source         string                 `json:"source" gorm:"index"`
	Component      string                 `json:"component"`
	Function       string                 `json:"function"`
	Error          string                 `json:"error"`
	Message        string                 `json:"message"`
	Stack          string                 `json:"stack,omitempty"`
	UserID         string                 `json:"user_id,omitempty" gorm:"index"`
	RequestID      string                 `json:"request_id,omitempty" gorm:"index"`
	Metadata       string                 `json:"metadata,omitempty" gorm:"type:jsonb"`
	AdditionalInfo map[string]interface{} `gorm:"serializer:json" json:"additional_info,omitempty"`
}

type BusinessMilestoneType string

const (
	BusinessMilestoneTypeFirstOrder BusinessMilestoneType = "first_order"
	BusinessMilestoneTypeRevenue    BusinessMilestoneType = "revenue"
)

type BusinessMilestoneStatus string

const (
	BusinessMilestoneStatusPending    BusinessMilestoneStatus = "pending"
	BusinessMilestoneStatusProcessing BusinessMilestoneStatus = "processing"
	BusinessMilestoneStatusSent       BusinessMilestoneStatus = "sent"
	BusinessMilestoneStatusFailed     BusinessMilestoneStatus = "failed"
)

// BusinessMilestoneEvent is a durable, idempotent outbox for business milestone
// emails. Payment transactions insert these rows under a unique key, and email
// delivery marks them sent after the transaction commits.
type BusinessMilestoneEvent struct {
	ID              uint                    `gorm:"primaryKey" json:"id"`
	BusinessID      uint                    `gorm:"not null;index;index:idx_business_milestone_pending,priority:1;uniqueIndex:idx_business_milestone_unique,priority:1" json:"business_id"`
	MilestoneType   BusinessMilestoneType   `gorm:"type:varchar(64);not null;uniqueIndex:idx_business_milestone_unique,priority:2" json:"milestone_type"`
	ThresholdCents  int64                   `gorm:"not null;default:0;uniqueIndex:idx_business_milestone_unique,priority:3" json:"threshold_cents"`
	BillID          *uint                   `gorm:"index" json:"bill_id,omitempty"`
	Status          BusinessMilestoneStatus `gorm:"type:varchar(32);not null;default:'pending';index;index:idx_business_milestone_pending,priority:2" json:"status"`
	ProcessingToken string                  `gorm:"type:varchar(64);index" json:"processing_token,omitempty"`
	Payload         map[string]interface{}  `gorm:"serializer:json" json:"payload,omitempty"`
	LastError       string                  `gorm:"type:text" json:"last_error,omitempty"`
	SentAt          *time.Time              `json:"sent_at,omitempty"`
	CreatedAt       time.Time               `gorm:"index:idx_business_milestone_pending,priority:3" json:"created_at"`
	UpdatedAt       time.Time               `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	Bill     Bill     `gorm:"foreignKey:BillID" json:"bill,omitempty"`
}

// BusinessRevenueAggregate stores the current recognized revenue totals used
// for O(1) milestone threshold checks during payment transactions.
type BusinessRevenueAggregate struct {
	ID                  uint      `gorm:"primaryKey" json:"id"`
	BusinessID          uint      `gorm:"not null;uniqueIndex" json:"business_id"`
	NetRevenueCents     int64     `gorm:"not null;default:0" json:"net_revenue_cents"`
	NetTipCents         int64     `gorm:"not null;default:0" json:"net_tip_cents"`
	GrossRevenueCents   int64     `gorm:"not null;default:0" json:"gross_revenue_cents"`
	GrossTipCents       int64     `gorm:"not null;default:0" json:"gross_tip_cents"`
	PositiveEventCount  int64     `gorm:"not null;default:0" json:"positive_event_count"`
	RecognizedBillCount int64     `gorm:"not null;default:0" json:"recognized_bill_count"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`

	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// Payverge-specific models

// Business represents a restaurant/venue in the system
type Business struct {
	ID               uint            `gorm:"primaryKey" json:"id"`
	BusinessId       string          `gorm:"uniqueIndex;not null" json:"business_id"`
	OwnerAddress     string          `gorm:"index;not null" json:"owner_address"`
	UserID           *uint           `gorm:"index" json:"user_id"` // OAuth user ID (nullable for Web3-only businesses)
	OwnerName        string          `json:"owner_name"`           // Business owner's name
	Name             string          `gorm:"not null" json:"name"`
	Logo             string          `json:"logo"`
	Address          BusinessAddress `gorm:"embedded" json:"address"`
	SettlementAddr   string          `gorm:"not null" json:"settlement_address"`
	TippingAddr      string          `gorm:"not null" json:"tipping_address"`
	TaxRate          float64         `json:"tax_rate"`
	ServiceFeeRate   float64         `json:"service_fee_rate"`
	TaxInclusive     bool            `json:"tax_inclusive"`
	ServiceInclusive bool            `json:"service_inclusive"`
	IsActive         bool            `gorm:"default:true" json:"is_active"`
	// New fields for enhanced business features
	Description          string `json:"description"`
	CustomURL            string `json:"custom_url"`
	Phone                string `json:"phone"`
	Email                string `json:"email"` // Business contact email
	Website              string `json:"website"`
	SocialMedia          string `json:"social_media"`  // JSON string for social media links
	BannerImages         string `json:"banner_images"` // JSON array of banner image URLs
	BusinessPageEnabled  bool   `json:"business_page_enabled"`
	ShowReviews          bool   `json:"show_reviews"`
	GoogleReviewsEnabled bool   `json:"google_reviews_enabled"`
	// Google Business Integration
	GooglePlaceID      string `json:"google_place_id"`               // Google Places API Place ID
	GoogleBusinessName string `json:"google_business_name"`          // Google business name for verification
	GoogleReviewLink   string `json:"google_review_link"`            // Generated review link
	GoogleBusinessURL  string `json:"google_business_url"`           // Google Maps business URL
	Timezone           string `gorm:"default:'UTC'" json:"timezone"` // IANA timezone (e.g., "Asia/Dubai", "America/New_York")
	// ServiceDayStartMinute is minutes after local midnight when the venue's
	// service day begins (0–1439). 0 = calendar midnight; 240 = 04:00 cutoff so
	// a 01:00 close still belongs to the previous service day.
	ServiceDayStartMinute int `gorm:"not null;default:0" json:"service_day_start_minute"`
	// Segmentation: business type captured at registration (restaurant, cafe,
	// bar, quick_service, food_truck, bakery, fine_dining, other).
	BusinessType string `gorm:"type:varchar(32);index" json:"business_type"`
	// Counter support for takeaway/quick service
	CounterEnabled bool   `json:"counter_enabled"`
	CounterCount   int    `json:"counter_count"`
	CounterPrefix  string `json:"counter_prefix"`
	// Kitchen and Orders feature toggle (for paid tier)
	KitchenEnabled bool `gorm:"default:true" json:"kitchen_enabled"` // Enable kitchen order management
	OrdersEnabled  bool `gorm:"default:true" json:"orders_enabled"`  // Enable order creation and approval
	// CRM feature toggle (for paid tier)
	CRMEnabled bool `gorm:"default:false" json:"crm_enabled"` // Enable customer relationship management
	// Multi-currency and multilingual support
	DefaultCurrency string `json:"default_currency"` // Currency for setting prices (internal)
	DisplayCurrency string `json:"display_currency"` // Currency shown to customers
	DefaultLanguage string `json:"default_language"` // Default display language for customers
	SourceLanguage  string `json:"source_language"`  // Language content was originally written in

	// Design Customization Settings (embedded struct)
	DesignSettings BusinessDesignSettings `gorm:"embedded;embeddedPrefix:design_" json:"design_settings"`

	// AI Waiter Settings (embedded struct)
	AiSettings BusinessAiSettings `gorm:"embedded;embeddedPrefix:ai_" json:"ai_settings"`

	// Default QR Code Customization (applied to new tables)
	DefaultQRLogoURL          string `json:"default_qr_logo_url"`
	DefaultQRForegroundColor  string `gorm:"default:'#000000'" json:"default_qr_foreground_color"`
	DefaultQRBackgroundColor  string `gorm:"default:'#FFFFFF'" json:"default_qr_background_color"`
	DefaultQRLogoSize         int    `gorm:"default:20" json:"default_qr_logo_size"`
	DefaultQRShowBusinessName bool   `gorm:"default:false" json:"default_qr_show_business_name"`
	DefaultQRShowTableName    bool   `gorm:"default:false" json:"default_qr_show_table_name"`
	DefaultQRTextFont         string `gorm:"default:'Verdana'" json:"default_qr_text_font"`
	// QRPreviewedAt is the durable first-value activation milestone. It is written idempotently by the authenticated
	// QR-preview endpoint; client state and analytics events are not authoritative.
	QRPreviewedAt *time.Time `json:"qr_previewed_at,omitempty"`

	// Location Coordinates (for delivery radius calculations)
	Latitude  *float64 `gorm:"type:decimal(10,8)" json:"latitude"`
	Longitude *float64 `gorm:"type:decimal(11,8)" json:"longitude"`

	// Hospitality Features
	WelcomeMessage      string `json:"welcome_message"`
	AboutStory          string `json:"about_story"`
	ShowWelcomeMessage  bool   `json:"show_welcome_message"`
	ShowAboutStory      bool   `json:"show_about_story"`
	ShowGallery         bool   `json:"show_gallery"`
	ShowOperatingHours  bool   `json:"show_operating_hours"`
	ShowSpecialFeatures bool   `json:"show_special_features"`
	// Admin closure (account closed by the instance admin).
	ClosedAt     *time.Time `json:"closed_at"`     // When the business was closed by admin
	ClosedReason string     `json:"closed_reason"` // Reason for admin closure

	// IMP-06 — first-run setup wizard. OnboardingState is an opaque JSON blob
	// that the frontend wizard reads and writes verbatim; the backend never
	// inspects the shape so the schema can evolve without a migration. Once
	// the operator clicks "Done" (or "Skip for now"), OnboardingCompletedAt
	// is set and the dashboard stops rendering the overlay.
	OnboardingState       JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"onboarding_state"`
	OnboardingCompletedAt *time.Time     `json:"onboarding_completed_at"`

	// Marketing automation settings (Slice 4). Opaque JSON blob written and read
	// verbatim by the marketing service layer: {"enabled":bool,"disabled_plays":[]string}.
	// Default keeps the suggestion engine on with no plays disabled. The GORM tag
	// mirrors the genesis column so AutoMigrate test fixtures match it.
	MarketingSettings JSONRawMessage `gorm:"type:jsonb;not null;default:'{\"enabled\": true, \"disabled_plays\": []}'" json:"marketing_settings"`

	// Kind classifies the business for the admin registry:
	// real (default), demo (Demo Center seeder), test (CI/local fixtures).
	// Admin lists and dashboard counts default to kind=real.
	Kind BusinessKind `gorm:"type:text;not null;default:'real';index:idx_businesses_kind" json:"kind"`

	// IsDemo flags internal demo businesses (boolean).
	// Prefer Kind for new admin scoping; IsDemo remains true for Kind=demo so
	// Demo Center paths keep working. Index name pinned to idx_businesses_demo.
	IsDemo bool `gorm:"not null;default:false;index:idx_businesses_demo" json:"is_demo"`

	// DemoOwnerUserID scopes generated demo businesses to the admin account they
	// belong to. Reset/delete paths must filter on both is_demo and this column.
	DemoOwnerUserID *uint `gorm:"index:idx_businesses_demo_owner" json:"demo_owner_user_id,omitempty"`

	// DirectorDigestLastSentAt records when the most recent daily digest email
	// was successfully sent for this business (UTC). The DirectorDigestScheduler
	// uses an atomic UPDATE on this column — conditioned on the value being NULL
	// or from a previous UTC day — to claim the send slot and prevent double-
	// sends on restart or multi-replica deployments.
	DirectorDigestLastSentAt *time.Time `json:"director_digest_last_sent_at,omitempty"`

	// GettingStartedEmailSentAt is a lifecycle claim column. The
	// LifecycleScheduler atomically stamps it from NULL → now() before sending
	// the onboarding email, so a restart's immediate-on-boot sweep cannot
	// re-emit the same email to every business in the created_at window.
	GettingStartedEmailSentAt *time.Time `json:"getting_started_email_sent_at,omitempty"`

	// SetupNudgeEmailSentAt, FirstOrderMilestoneSentAt and
	// RevenueMilestoneSentAt are one-shot lifecycle claim columns. The
	// schedulers atomically stamp each from NULL → now() before sending the
	// corresponding email (day-3 setup nudge, first-order and $10k revenue
	// milestones), so a restart's immediate-on-boot sweep cannot re-emit it.
	SetupNudgeEmailSentAt     *time.Time `json:"setup_nudge_email_sent_at,omitempty"`
	FirstOrderMilestoneSentAt *time.Time `json:"first_order_milestone_sent_at,omitempty"`
	RevenueMilestoneSentAt    *time.Time `json:"revenue_milestone_sent_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Counter represents a service counter for takeaway/quick service
type Counter struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	BusinessID    uint      `gorm:"not null;index" json:"business_id"`
	CounterNumber int       `gorm:"not null" json:"counter_number"`
	Name          string    `gorm:"not null" json:"name"`
	IsActive      bool      `gorm:"default:true" json:"is_active"`
	CurrentBillID *uint     `json:"current_bill_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`

	// Relationships
	Business    Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	CurrentBill *Bill    `gorm:"foreignKey:CurrentBillID" json:"current_bill,omitempty"`
}

// BusinessAddress represents the physical address of a business
type BusinessAddress struct {
	Street     string `gorm:"column:street" json:"street"`
	City       string `gorm:"column:city" json:"city"`
	State      string `gorm:"column:state" json:"state"`
	PostalCode string `gorm:"column:postal_code" json:"postal_code"`
	Country    string `gorm:"column:country" json:"country"`
}

// BusinessDesignSettings represents design customization settings for a business
type BusinessDesignSettings struct {
	PrimaryColor      string  `gorm:"default:'#1f2937'" json:"primary_color"`
	SecondaryColor    string  `gorm:"default:'#3b82f6'" json:"secondary_color"`
	FontFamily        string  `gorm:"default:'Inter'" json:"font_family"`
	Theme             string  `gorm:"default:'light'" json:"theme"`
	MenuLayout        string  `gorm:"default:'grid'" json:"menu_layout"`
	ShowImages        bool    `gorm:"default:true" json:"show_images"`
	ShowDescriptions  bool    `gorm:"default:true" json:"show_descriptions"`
	HeaderStyle       string  `gorm:"default:'banner'" json:"header_style"`
	CornerRadius      string  `gorm:"default:'medium'" json:"corner_radius"`
	ShadowIntensity   string  `gorm:"default:'subtle'" json:"shadow_intensity"`
	BackgroundPattern string  `gorm:"default:'none'" json:"background_pattern"`
	PatternOpacity    float64 `gorm:"default:0.1" json:"pattern_opacity"`
	HeroLayout        string  `gorm:"default:'centered'" json:"hero_layout"`
	SectionDensity    string  `gorm:"default:'comfortable'" json:"section_density"`
}

// BusinessAiSettings represents AI Waiter configuration
type BusinessAiSettings struct {
	AiEnabled             bool   `gorm:"default:true" json:"ai_enabled"`
	AiName                string `gorm:"default:'Sage'" json:"ai_name"`
	AiPriority            string `gorm:"default:'balanced'" json:"ai_priority"` // upselling, balanced, service
	SpecialInstructions   string `gorm:"type:text" json:"special_instructions"`
	BusinessPageAiEnabled bool   `gorm:"default:false" json:"business_page_ai_enabled"`
}

// BusinessGalleryImage represents a gallery image for a business
type BusinessGalleryImage struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	BusinessID   uint      `gorm:"index;not null" json:"business_id"`
	ImageURL     string    `gorm:"not null" json:"image_url"`
	Caption      string    `json:"caption"`
	DisplayOrder int       `gorm:"default:0" json:"display_order"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Business relation: see Bundle/Offer for rationale — never serialized
	// to JSON. The public storefront handler returns these slices raw on
	// an unauthenticated path (GetBusinessByCustomURL, ~L1177 of
	// business_settings_handlers.go). The frontend Business type carries
	// no gallery-relative `business` field. (X-2)
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// BusinessOperatingHours represents operating hours for a business
type BusinessOperatingHours struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	BusinessID uint   `gorm:"index;not null" json:"business_id"`
	DayOfWeek  int    `gorm:"not null" json:"day_of_week"` // 0=Sunday, 1=Monday, etc.
	OpenTime   string `json:"open_time"`                   // Format: "09:00"
	CloseTime  string `json:"close_time"`                  // Format: "17:00"
	// KitchenCloseTime is optional last-seating / kitchen-close clock ("22:00").
	// When set, guest dining availability (OPEN NOW + reservation slots) ends
	// here even if CloseTime runs later for the bar/door.
	KitchenCloseTime *string   `json:"kitchen_close_time,omitempty"`
	IsClosed         bool      `gorm:"default:false" json:"is_closed"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	// Business relation: see BusinessGalleryImage for rationale. Returned
	// raw on the public storefront handler. (X-2)
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// BusinessOperatingException is a one-off holiday / private-event override for
// a single calendar date. When IsClosed, the venue takes no bookings that day.
type BusinessOperatingException struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	BusinessID       uint      `gorm:"index;not null;uniqueIndex:uq_business_operating_exception_date" json:"business_id"`
	ExceptionDate    time.Time `gorm:"type:date;not null;uniqueIndex:uq_business_operating_exception_date" json:"exception_date"`
	OpenTime         *string   `json:"open_time,omitempty"`
	CloseTime        *string   `json:"close_time,omitempty"`
	KitchenCloseTime *string   `json:"kitchen_close_time,omitempty"`
	IsClosed         bool      `gorm:"not null;default:true" json:"is_closed"`
	Label            string    `gorm:"not null;default:''" json:"label"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Business         Business  `gorm:"foreignKey:BusinessID" json:"-"`
}

// BusinessSpecialFeature represents a special feature/amenity for a business
type BusinessSpecialFeature struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	BusinessID   uint      `gorm:"index;not null" json:"business_id"`
	Title        string    `gorm:"not null" json:"title"`
	Description  string    `json:"description"`
	Icon         string    `json:"icon"` // Icon name for frontend
	DisplayOrder int       `gorm:"default:0" json:"display_order"`
	IsActive     bool      `gorm:"default:true" json:"is_active"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Business relation: see BusinessGalleryImage for rationale. Returned
	// raw on the public storefront handler. (X-2)
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// Menu represents a business's menu
type Menu struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	Categories string    `gorm:"type:text" json:"categories"` // JSON string for database storage
	IsActive   bool      `gorm:"default:true" json:"is_active"`
	Version    uint      `gorm:"default:1" json:"version"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
	Business   Business  `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// MenuCategory represents a category within a menu
type MenuCategory struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Items       []MenuItem `json:"items"`
	SortOrder   int        `json:"sort_order"`
}

// MenuItem represents an individual menu item
type MenuItem struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Price       float64 `json:"price"`
	// Cogs is an optional per-plate food cost in dollars (same wire unit as Price).
	// Used by the food-cost calculator when the item has no inventory recipe —
	// operators can enter a plate cost without building a full recipe first.
	Cogs             float64          `json:"cogs,omitempty"`
	Currency         string           `json:"currency"`
	Image            string           `json:"image"`             // Cover photo
	Images           []string         `json:"images"`            // Gallery photos
	CompositionImage string           `json:"composition_image"` // AI-generated composition/ingredients image
	Options          []MenuItemOption `json:"options"`
	Allergens        []string         `json:"allergens"`
	DietaryTags      []string         `json:"dietary_tags"`
	IsAvailable      bool             `json:"is_available"`
	SortOrder        int              `json:"sort_order"`
	// InventoryStatus is a serve-time stamp, never stored in the menu blob.
	// Guest ResolveOrderability overwrites inventory_out with business_closed;
	// this field survives that remap so diners can still omit an 86'd dish
	// after hours. Only out_of_stock is stamped (omitempty otherwise).
	InventoryStatus string `json:"inventory_status,omitempty"`
	// ManualAvailable mirrors the STORED is_available flag on operator reads.
	// Operator payloads report is_available as the effective answer — the same
	// one guests get — so the surfaces that EDIT the manual 86 (Menu Builder's
	// edit form and one-tap 86, Kitchen's 86 control) must read this instead,
	// or saving an inventory-blocked dish would pin a manual 86 that outlives
	// the restock (#727). Serve-time only: no write path accepts it, and the
	// pointer keeps it out of the stored menu blob.
	ManualAvailable *bool `json:"manual_available,omitempty"`
}

// MenuItemOption represents options/modifications for menu items
type MenuItemOption struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	PriceChange float64 `json:"price_change"`
	IsRequired  bool    `json:"is_required"`
}

// Table represents a physical table in a business
type Table struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	BusinessID uint   `gorm:"index;not null" json:"business_id"`
	TableCode  string `gorm:"uniqueIndex;not null" json:"table_code"`
	Name       string `gorm:"not null" json:"name"`
	Capacity   int    `gorm:"default:4" json:"capacity"` // Number of seats at this table
	QRCode     string `json:"qr_code"`
	IsActive   bool   `gorm:"default:true" json:"is_active"`
	// QR Code Customization
	QRLogoURL          string `json:"qr_logo_url"`                                  // Custom logo in center of QR
	QRForegroundColor  string `gorm:"default:'#000000'" json:"qr_foreground_color"` // QR code color
	QRBackgroundColor  string `gorm:"default:'#FFFFFF'" json:"qr_background_color"` // Background color
	QRLogoSize         int    `gorm:"default:20" json:"qr_logo_size"`               // Logo size percentage (10-30)
	QRShowBusinessName bool   `gorm:"default:false" json:"qr_show_business_name"`   // Show business name on QR
	QRShowTableName    bool   `gorm:"default:false" json:"qr_show_table_name"`      // Show table name on QR
	QRTextFont         string `gorm:"default:'Verdana'" json:"qr_text_font"`        // Font for QR text (Verdana, Arial, Helvetica, Times New Roman, Courier, Georgia)

	// Spaces & layout placement (nullable: a table need not sit on a floor plan).
	// Coordinates are millimeters; origin top-left of the space, X right, Y down.
	SpaceID          *uint   `gorm:"index" json:"space_id,omitempty"`
	RegionID         *uint   `json:"region_id,omitempty"`
	PosXMm           *int    `json:"pos_x_mm,omitempty"`
	PosYMm           *int    `json:"pos_y_mm,omitempty"`
	WidthMm          *int    `json:"width_mm,omitempty"`
	HeightMm         *int    `json:"height_mm,omitempty"`
	RotationDeg      float64 `gorm:"default:0" json:"rotation_deg"`
	Shape            string  `gorm:"default:'rectangle'" json:"shape"` // round|square|rectangle|oval|bar|custom
	MinCapacity      *int    `json:"min_capacity,omitempty"`
	MaxCapacity      *int    `json:"max_capacity,omitempty"`
	VisibleSeatCount *int    `json:"visible_seat_count,omitempty"`
	IsReservable     bool    `gorm:"default:true" json:"is_reservable"`
	IsCombinable     bool    `gorm:"default:false" json:"is_combinable"`
	IsAccessible     bool    `gorm:"default:false" json:"is_accessible"`
	LayoutInDraft    bool    `gorm:"default:false" json:"layout_in_draft"`
	LayoutPublished  bool    `gorm:"default:false" json:"layout_published"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Business  Business  `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// ReservationSettings represents business-level reservation configuration
type ReservationSettings struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	BusinessID            uint           `gorm:"unique;not null" json:"business_id"`
	Enabled               bool           `gorm:"default:false" json:"enabled"`
	MaxAdvanceDays        int            `gorm:"default:30" json:"max_advance_days"` // Max days in advance to book
	MinAdvanceMinutes     int            `gorm:"default:30" json:"min_advance_minutes"`
	MinPartySize          int            `gorm:"default:1" json:"min_party_size"`
	MaxPartySize          int            `gorm:"default:20" json:"max_party_size"`
	DefaultDuration       int            `gorm:"default:120" json:"default_duration"` // Default duration in minutes
	SlotIntervalMinutes   int            `gorm:"default:30" json:"slot_interval_minutes"`
	ServiceBufferMinutes  int            `gorm:"default:15" json:"service_buffer_minutes"`
	MaxCoversPerSlot      int            `gorm:"default:0" json:"max_covers_per_slot"`
	AutoAssignTables      bool           `gorm:"default:true" json:"auto_assign_tables"`
	ApprovalMode          string         `gorm:"size:16;default:'auto'" json:"approval_mode"` // auto | manual (business approves each guest booking)
	AllowWaitlist         bool           `gorm:"default:true" json:"allow_waitlist"`
	HoldDurationMinutes   int            `gorm:"default:15" json:"hold_duration_minutes"`
	AllowCancellation     bool           `gorm:"default:true" json:"allow_cancellation"`  // Allow customer cancellation
	CancellationDeadline  int            `gorm:"default:24" json:"cancellation_deadline"` // Hours before reservation
	NoShowGraceMinutes    int            `gorm:"default:15" json:"no_show_grace_minutes"`
	SendConfirmationEmail bool           `gorm:"default:true" json:"send_confirmation_email"`
	SendReminderEmail     bool           `gorm:"default:true" json:"send_reminder_email"`
	ReminderHoursBefore   int            `gorm:"default:24" json:"reminder_hours_before"`
	ExternalPartnerLinks  JSONRawMessage `gorm:"type:jsonb;default:'[]'" json:"external_partner_links"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	Business              Business       `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// TableReservation represents a reservation for a table
type TableReservation struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	BusinessID       uint      `gorm:"index;index:idx_reservations_biz_status_time,priority:1;not null" json:"business_id"`
	TableID          *uint     `gorm:"index" json:"table_id"` // Nullable - assigned by business later for guest reservations
	CustomerName     string    `gorm:"not null" json:"customer_name"`
	CustomerPhone    string    `json:"customer_phone"`
	CustomerEmail    string    `json:"customer_email"`
	PartySize        int       `gorm:"not null" json:"party_size"`
	ReservationTime  time.Time `gorm:"index;index:idx_reservations_biz_status_time,priority:3;not null" json:"reservation_time"`
	Duration         int       `gorm:"default:120" json:"duration"`                                                       // Duration in minutes
	Status           string    `gorm:"default:'pending';index:idx_reservations_biz_status_time,priority:2" json:"status"` // pending, confirmed, waitlist, seated, completed, cancelled, no_show
	Source           string    `gorm:"default:'customer'" json:"source"`
	ConfirmationCode string    `gorm:"size:32;index" json:"confirmation_code"`
	// Language is the guest locale captured at booking time (e.g. "en", "es-AR",
	// "ja"). Used for email template family selection, deep-link ?lang=, and
	// confirmation-page chrome.
	Language        string `gorm:"size:16;not null;default:en" json:"language"`
	SpecialRequests string `gorm:"type:text" json:"special_requests"`
	Notes           string `gorm:"type:text" json:"notes"`             // Internal staff notes
	ReminderSent    bool   `gorm:"default:false" json:"reminder_sent"` // Track if reminder email has been sent
	// Stamped by the approval sweeper when the operator "still unactioned"
	// nudge email is sent (manual-approval requests only). One email per
	// request, ever.
	ApprovalReminderSentAt *time.Time `json:"approval_reminder_sent_at,omitempty"`
	CreatedBy              string     `json:"created_by"` // staff or customer
	ConfirmedAt            *time.Time `json:"confirmed_at,omitempty"`
	AssignedAt             *time.Time `json:"assigned_at,omitempty"`
	SeatedAt               *time.Time `json:"seated_at,omitempty"`
	CompletedAt            *time.Time `json:"completed_at,omitempty"`
	CancelledAt            *time.Time `json:"cancelled_at,omitempty"`
	CancelledBy            string     `json:"cancelled_by"`
	CancellationReason     string     `gorm:"type:text" json:"cancellation_reason"`
	WaitlistPosition       *int       `json:"waitlist_position,omitempty"`
	// L4-8: multi-operator claim lock (mirrors delivery_orders / AI waiter).
	// Nil ClaimedByStaffID + empty name = unclaimed.
	ClaimedByStaffID *uint      `gorm:"index" json:"claimed_by_staff_id,omitempty"`
	ClaimedByName    string     `json:"claimed_by_name,omitempty"`
	ClaimedByRole    string     `json:"claimed_by_role,omitempty"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	// Business is GORM-only; list/detail already scope by business_id and must
	// not embed a zero-value Business (~3KB empty fields per reservation).
	Business      Business                   `gorm:"foreignKey:BusinessID" json:"-"`
	Table         *Table                     `gorm:"foreignKey:TableID" json:"table,omitempty"`
	StatusHistory []ReservationStatusHistory `gorm:"foreignKey:ReservationID" json:"status_history,omitempty"`
}

type ReservationStatusHistory struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ReservationID uint      `gorm:"index;not null" json:"reservation_id"`
	Status        string    `gorm:"not null" json:"status"`
	TableID       *uint     `gorm:"index" json:"table_id,omitempty"`
	Notes         string    `gorm:"type:text" json:"notes"`
	ChangedBy     string    `json:"changed_by"`
	CreatedAt     time.Time `json:"created_at"`

	Reservation TableReservation `gorm:"foreignKey:ReservationID" json:"reservation,omitempty"`
	Table       *Table           `gorm:"foreignKey:TableID" json:"table,omitempty"`
}

// Bill represents a bill/check for a table
type Bill struct {
	ID         uint   `gorm:"primaryKey" json:"id"`
	BusinessID uint   `gorm:"index;not null" json:"business_id"`
	TableID    uint   `gorm:"index" json:"table_id"`
	CounterID  *uint  `gorm:"index" json:"counter_id"`
	BillNumber string `gorm:"uniqueIndex;not null" json:"bill_number"`
	// PublicToken is the unguessable guest capability for /guest/bill routes.
	// Generated by Bill.BeforeCreate (crypto/rand, 16 bytes hex). BillNumber
	// stays the operator-facing display number.
	PublicToken string `gorm:"column:public_token;uniqueIndex:idx_bills_public_token" json:"public_token"`
	Notes       string `gorm:"type:text" json:"notes"` // Order notes for kitchen/staff
	Items       string `gorm:"type:text" json:"items"` // JSON snapshot; MarshalJSON emits an array or omits (#771)
	// loadedItemsRaw is the items column as read by GetBillByID, before the
	// read path rewrites Items for display. UpdateBillWithHistory compares it
	// under lock to refuse writes computed from a stale snapshot.
	loadedItemsRaw       *string
	Subtotal             int64 `gorm:"default:0" json:"subtotal"`
	TaxAmount            int64 `gorm:"default:0" json:"tax_amount"`
	ServiceFeeAmount     int64 `gorm:"default:0" json:"service_fee_amount"`
	TotalAmount          int64 `gorm:"default:0" json:"total_amount"`
	PaidAmount           int64 `gorm:"default:0" json:"paid_amount"`
	TipAmount            int64 `gorm:"default:0" json:"tip_amount"`
	LoyaltyDiscountCents int64 `gorm:"default:0" json:"loyalty_discount_cents"`
	// LoyaltyPointsRedeemed / LoyaltyRedeemedByCustomerID record who spent how many
	// points on this bill, so the points can be returned if the bill is voided
	// (the redemption never converted to a payment). Set by loyalty.RedeemPoints,
	// cleared by loyalty.UndoRedemption.
	LoyaltyPointsRedeemed       int        `gorm:"default:0" json:"loyalty_points_redeemed"`
	LoyaltyRedeemedByCustomerID *uint      `gorm:"index" json:"loyalty_redeemed_by_customer_id,omitempty"`
	Currency                    string     `gorm:"-" json:"currency"`
	Status                      BillStatus `gorm:"default:'open'" json:"status"`
	SettlementAddr              string     `gorm:"not null" json:"settlement_address"`
	TippingAddr                 string     `gorm:"not null" json:"tipping_address"`
	CreatedByStaffID            *uint      `gorm:"index" json:"created_by_staff_id,omitempty"`
	ClosedByStaffID             *uint      `gorm:"index" json:"closed_by_staff_id,omitempty"`
	CRMCustomerID               *uint      `gorm:"index" json:"crm_customer_id,omitempty"`
	// Fiscal customer identity — populated at checkout for AFIP/ARCA e-invoice emission.
	FiscalCustomerDocType      *string `gorm:"column:fiscal_customer_doc_type" json:"fiscal_customer_doc_type"`
	FiscalCustomerDocNumber    *string `gorm:"column:fiscal_customer_doc_number" json:"fiscal_customer_doc_number"`
	FiscalCustomerTaxCondition *string `gorm:"column:fiscal_customer_tax_condition" json:"fiscal_customer_tax_condition"`
	FiscalCustomerName         *string `gorm:"column:fiscal_customer_name" json:"fiscal_customer_name"`
	// FiscalCustomerEmail is where the guest wants their factura sent (migration
	// 000202). Feeds the fiscal delivery email channel; never printed on tickets.
	FiscalCustomerEmail *string `gorm:"column:fiscal_customer_email" json:"fiscal_customer_email"`
	// FiscalCustomerGuestSession is the guestsession.Fingerprint of the guest
	// session that set the fiscal identity through the public guest route.
	// NULL when an operator set it, which locks the identity against guest
	// writes. Never serialized.
	FiscalCustomerGuestSession *string    `gorm:"column:fiscal_customer_guest_session;size:64" json:"-"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
	ClosedAt                   *time.Time `json:"closed_at"`
	// AbandonedAt is set by the bill-lifecycle sweeper when a long-open bill is
	// marked abandoned (status=abandoned). NULL for every other status.
	AbandonedAt         *time.Time `gorm:"index" json:"abandoned_at,omitempty"`
	SettledAt           *time.Time `gorm:"column:settled_at" json:"settled_at,omitempty"`
	FeedbackEmailSentAt *time.Time `gorm:"index" json:"feedback_email_sent_at,omitempty"`
	Business            Business   `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	// -:migration keeps GORM AutoMigrate (test fixtures) from synthesizing
	// fk_bills_table: table_id uses 0 as the "no table" sentinel for
	// delivery/counter bills, which a FK to tables(id) rejects. Genesis has no
	// such key either. Preload("Table") still works.
	Table Table `gorm:"foreignKey:TableID;-:migration" json:"table,omitempty"`
	// Counter          *Counter   `gorm:"foreignKey:CounterID" json:"counter,omitempty"` // Temporarily disabled
	Payments            []Payment            `gorm:"foreignKey:BillID" json:"payments,omitempty"`
	AlternativePayments []AlternativePayment `gorm:"foreignKey:BillID" json:"alternative_payments,omitempty"`
	ItemsRelation       []BillItem           `gorm:"foreignKey:BillID" json:"items_relation,omitempty"`
	CreatedByStaff      *Staff               `gorm:"foreignKey:CreatedByStaffID" json:"created_by_staff,omitempty"`
	ClosedByStaff       *Staff               `gorm:"foreignKey:ClosedByStaffID" json:"closed_by_staff,omitempty"`
	CRMCustomer         *Customer            `gorm:"foreignKey:CRMCustomerID" json:"crm_customer,omitempty"`
}

// BillItem represents an item on a bill
type BillItem struct {
	ID             string           `gorm:"primaryKey;type:uuid;default:gen_random_uuid()" json:"id"`
	BillID         uint             `gorm:"index;not null" json:"bill_id"`
	MenuItemID     string           `gorm:"default:''" json:"menu_item_id"`
	Name           string           `gorm:"not null" json:"name"`
	Price          float64          `gorm:"not null" json:"price"`
	Quantity       int              `gorm:"not null" json:"quantity"`
	Options        []MenuItemOption `gorm:"serializer:json" json:"options"`
	ItemType       string           `gorm:"default:'menu_item'" json:"item_type"`
	BundleID       *uint            `json:"bundle_id,omitempty"`
	ParentBundleID *uint            `json:"parent_bundle_id,omitempty"`
	// BundleOccurrenceID links one expanded bundle parent to only its own
	// component rows. Snapshot-only: bill reads merge it back into relational
	// rows by item ID before a normal mutation rebuilds the JSON snapshot.
	BundleOccurrenceID string `gorm:"-" json:"bundle_occurrence_id,omitempty"`
	SourceOfferID      *uint  `json:"source_offer_id,omitempty"`
	// OrderID links a bill item back to the approved order that added it, so that
	// cancelling that order can remove exactly its items and recompute the bill.
	// NULL for items added directly (not via an order). See removeCancelledOrderBillItemsTx.
	OrderID   *uint     `gorm:"index" json:"order_id,omitempty"`
	Subtotal  float64   `gorm:"not null" json:"subtotal"`
	CreatedAt time.Time `json:"created_at"`
}

// BillHistoryEvent represents an append-only audit trail for bill changes.
type BillHistoryEvent struct {
	ID          uint                   `gorm:"primaryKey" json:"id"`
	BillID      uint                   `gorm:"index;index:idx_bill_events_bill_created,priority:1;not null" json:"bill_id"`
	BusinessID  uint                   `gorm:"index;not null" json:"business_id"`
	EventType   string                 `gorm:"index;not null" json:"event_type"`
	Actor       string                 `json:"actor"`
	Reason      string                 `gorm:"type:text" json:"reason"`
	OrderID     *uint                  `gorm:"index" json:"order_id,omitempty"`
	OrderNumber string                 `json:"order_number,omitempty"`
	BillItemID  string                 `gorm:"index" json:"bill_item_id,omitempty"`
	ItemName    string                 `json:"item_name,omitempty"`
	Details     map[string]interface{} `gorm:"serializer:json" json:"details,omitempty"`
	CreatedAt   time.Time              `gorm:"index:idx_bill_events_bill_created,priority:2" json:"created_at"`

	Bill Bill `gorm:"foreignKey:BillID" json:"bill,omitempty"`
}

const (
	BillHistoryEventBillCreated    = "bill.created"
	BillHistoryEventBillUpdated    = "bill.updated"
	BillHistoryEventBillClosed     = "bill.closed"
	BillHistoryEventBillAbandoned  = "bill.abandoned"
	BillHistoryEventItemAdded      = "bill_item.added"
	BillHistoryEventItemRemoved    = "bill_item.removed"
	BillHistoryEventItemVoided     = "bill_item.voided"
	BillHistoryEventItemQtyUpdated = "bill_item.quantity_updated"
	BillHistoryEventOrderApproved  = "order.approved"
	BillHistoryEventOrderCanceled  = "order.cancelled"
)

// BillStatus represents the status of a bill
type BillStatus string

const (
	BillStatusOpen    BillStatus = "open"
	BillStatusPartial BillStatus = "partial"
	BillStatusPaid    BillStatus = "paid"
	BillStatusClosed  BillStatus = "closed"
	// BillStatusVoided marks a bill as cancelled before any payment landed.
	// Set by IMP-15's POST /bills/:id/void; the row stays in place for audit.
	BillStatusVoided BillStatus = "voided"
	// BillStatusAbandoned marks a long-open bill closed by the lifecycle sweeper
	// (default 24h). Leaves the active set so tables free up; not a void or paid.
	BillStatusAbandoned BillStatus = "abandoned"
)

// Payment represents a payment made towards a bill
type Payment struct {
	ID        uint          `gorm:"primaryKey" json:"id"`
	BillID    uint          `gorm:"not null" json:"bill_id"`
	PayerAddr string        `gorm:"not null" json:"payer_address"`
	Amount    int64         `gorm:"not null" json:"amount"`
	TipAmount int64         `gorm:"default:0" json:"tip_amount"`
	Currency  string        `gorm:"-" json:"currency"`
	TxHash    string        `gorm:"uniqueIndex" json:"tx_hash"`
	Status    PaymentStatus `gorm:"default:'pending'" json:"status"`
	// PaymentMethod raw storage: crypto, cross-chain, plugin, plugin names
	// (stripe/paypal/…), or legacy currency codes. Canonical operator vocabulary
	// is reporting.Method (crypto|cross_chain|card|cash|wallet|other) via
	// reporting.Canonicalize — never read this field as the UI filter key.
	PaymentMethod   string     `gorm:"default:'crypto'" json:"payment_method"`
	SourceChain     string     `json:"source_chain,omitempty"`                 // Source chain for cross-chain payments
	SourceToken     string     `json:"source_token,omitempty"`                 // Source token symbol for cross-chain payments
	SettlementChain string     `gorm:"default:'base'" json:"settlement_chain"` // Settlement chain (always Base for cross-chain)
	LifiRouteId     string     `json:"lifi_route_id,omitempty"`                // LI.FI route ID for tracking
	ConfirmedAt     *time.Time `json:"confirmed_at,omitempty"`
	ReversedAt      *time.Time `json:"reversed_at,omitempty"`
	// Reorg reconciliation evidence: the block a confirmed crypto
	// payment settled in, plus reconciler bookkeeping. Nil for non-crypto /
	// pre-000143 rows. block_hash IS NOT NULL is the sweep's candidate predicate.
	// See internal/services/reorgwatch.
	BlockNumber      *int64     `json:"block_number,omitempty"`
	BlockHash        *string    `gorm:"size:66" json:"block_hash,omitempty"`
	ReorgCheckedAt   *time.Time `json:"reorg_checked_at,omitempty"`
	ReorgSuspectedAt *time.Time `json:"reorg_suspected_at,omitempty"`
	// PayerGuestSession is the guestsession.Fingerprint of the guest browser
	// session that initiated this payment; NULL for staff and webhook rows.
	// It is payment proof for the guest fiscal identity binding. Never
	// serialized.
	PayerGuestSession *string `gorm:"column:payer_guest_session;size:64" json:"-"`
	// Provider-reported cumulative refund and current dispute withdrawal for
	// plugin (card) payments. Webhooks carry cumulative totals; the ledger
	// applies only the delta against these so a replay or a later cumulative
	// event never double-reverses. ProviderDisputedTipCents is the part of the
	// disputed amount that came out of TipAmount, so reinstatement restores it.
	ProviderRefundedCents    int64 `gorm:"column:provider_refunded_cents;not null;default:0" json:"-"`
	ProviderDisputedCents    int64 `gorm:"column:provider_disputed_cents;not null;default:0" json:"-"`
	ProviderDisputedTipCents int64 `gorm:"column:provider_disputed_tip_cents;not null;default:0" json:"-"`
	// SettlementAddr is the bill's settlement wallet when this crypto payment
	// confirmed: the wallet the funds landed in, and so the wallet a refund
	// must leave from. The bill's own settlement_addr follows wallet rotation
	// on open checks, so it cannot stand in for it. Nil for non-crypto rows.
	SettlementAddr *string   `gorm:"column:settlement_addr" json:"-"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	Bill           Bill      `gorm:"foreignKey:BillID" json:"bill,omitempty"`
}

// ---------------------------------------------------------------------------
// Wave 4 REFUND track — verified on-chain refund destinations + durable
// noncustodial crypto refund state machine. Money fields are integer base
// units (USDC micro-units). RefundAddress is never marshaled on public guest
// payloads; operator APIs return a masked projection only.
// ---------------------------------------------------------------------------

// RefundEvidenceType classifies how a refund destination was verified.
type RefundEvidenceType string

const (
	RefundEvidenceTransferLog     RefundEvidenceType = "transfer_log"
	RefundEvidenceWalletSignature RefundEvidenceType = "wallet_signature"
)

// PaymentRefundDestination is append-once evidence of a verified refund
// destination captured at payment settlement time. Public bill reads must
// never preload or expose RefundAddress.
type PaymentRefundDestination struct {
	ID              uint               `gorm:"primaryKey" json:"id"`
	PaymentID       uint               `gorm:"uniqueIndex;not null" json:"payment_id"`
	ChainID         int                `gorm:"not null" json:"chain_id"`
	Token           string             `gorm:"size:32;not null;default:USDC" json:"token"`
	AmountBaseUnits int64              `gorm:"not null" json:"amount_base_units"`
	RefundAddress   string             `gorm:"size:64;not null" json:"-"` // never public
	EvidenceType    RefundEvidenceType `gorm:"size:32;not null" json:"evidence_type"`
	SignatureRef    *string            `gorm:"type:text" json:"-"`
	LogRef          *string            `gorm:"size:128" json:"log_ref,omitempty"`
	VerifiedAt      time.Time          `gorm:"not null" json:"verified_at"`
	CreatedAt       time.Time          `json:"created_at"`
	Payment         Payment            `gorm:"foreignKey:PaymentID" json:"-"`
}

// PaymentRefundStatus is the durable crypto refund lifecycle. Only "confirmed"
// means money was returned on-chain and the local ledger was reversed. Never
// present requested/submitted/confirming as "refunded" in operator UI.
type PaymentRefundStatus string

const (
	PaymentRefundStatusRequested         PaymentRefundStatus = "requested"
	PaymentRefundStatusApproved          PaymentRefundStatus = "approved"
	PaymentRefundStatusAwaitingSignature PaymentRefundStatus = "awaiting_signature"
	PaymentRefundStatusSubmitted         PaymentRefundStatus = "submitted"
	PaymentRefundStatusConfirming        PaymentRefundStatus = "confirming"
	PaymentRefundStatusConfirmed         PaymentRefundStatus = "confirmed"
	PaymentRefundStatusFailed            PaymentRefundStatus = "failed"
	PaymentRefundStatusRejected          PaymentRefundStatus = "rejected"
	PaymentRefundStatusCancelled         PaymentRefundStatus = "cancelled"
)

// PaymentRefund is a durable noncustodial on-chain refund request. The backend
// never custodies a treasury key; owners sign/submit externally and the worker
// verifies the outbound transfer before marking confirmed and applying ledger.
type PaymentRefund struct {
	ID                uint                `gorm:"primaryKey" json:"id"`
	BusinessID        uint                `gorm:"index;not null" json:"business_id"`
	BillID            uint                `gorm:"index;not null" json:"bill_id"`
	PaymentID         uint                `gorm:"index;not null" json:"payment_id"`
	ChainID           int                 `gorm:"not null" json:"chain_id"`
	Token             string              `gorm:"size:32;not null;default:USDC" json:"token"`
	AmountBaseUnits   int64               `gorm:"not null" json:"amount_base_units"`
	VerifiedRecipient string              `gorm:"size:64;not null" json:"verified_recipient"`
	RecipientOverride bool                `gorm:"not null;default:false" json:"recipient_override"`
	OverrideReason    *string             `gorm:"type:text" json:"override_reason,omitempty"`
	Reason            string              `gorm:"type:text;not null" json:"reason"`
	RequestedBy       string              `gorm:"size:255;not null" json:"requested_by"`
	ApprovedBy        *string             `gorm:"size:255" json:"approved_by,omitempty"`
	RejectedBy        *string             `gorm:"size:255" json:"rejected_by,omitempty"`
	Status            PaymentRefundStatus `gorm:"size:32;not null;default:requested;index" json:"status"`
	IdempotencyKey    string              `gorm:"size:128;not null" json:"idempotency_key"`
	SubmittedTxHash   *string             `gorm:"size:128" json:"submitted_tx_hash,omitempty"`
	Confirmations     int                 `gorm:"not null;default:0" json:"confirmations"`
	LastError         *string             `gorm:"type:text" json:"last_error,omitempty"`
	LockedAt          *time.Time          `json:"-"`
	LockedBy          *string             `gorm:"size:128" json:"-"`
	LedgerAppliedAt   *time.Time          `json:"ledger_applied_at,omitempty"`
	// Reorg reconciliation evidence: the block the outbound refund
	// tx confirmed in + reconciler bookkeeping. Nil until confirmed / pre-000143.
	// See internal/services/reorgwatch.
	BlockNumber      *int64     `json:"block_number,omitempty"`
	BlockHash        *string    `gorm:"size:66" json:"block_hash,omitempty"`
	ReorgCheckedAt   *time.Time `json:"reorg_checked_at,omitempty"`
	ReorgSuspectedAt *time.Time `json:"reorg_suspected_at,omitempty"`
	RequestedAt      time.Time  `json:"requested_at"`
	ApprovedAt       *time.Time `json:"approved_at,omitempty"`
	SubmittedAt      *time.Time `json:"submitted_at,omitempty"`
	ConfirmingAt     *time.Time `json:"confirming_at,omitempty"`
	ConfirmedAt      *time.Time `json:"confirmed_at,omitempty"`
	FailedAt         *time.Time `json:"failed_at,omitempty"`
	RejectedAt       *time.Time `json:"rejected_at,omitempty"`
	CancelledAt      *time.Time `json:"cancelled_at,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusConfirmed PaymentStatus = "confirmed"
	PaymentStatusFailed    PaymentStatus = "failed"
	PaymentStatusReversed  PaymentStatus = "reversed"
	// PaymentStatusRefundPending is a short-lived guard for provider-backed
	// refunds. It is set before the PSP refund call so duplicate requests cannot
	// issue a second provider refund while the first call is in flight.
	PaymentStatusRefundPending PaymentStatus = "refund_pending"
	// PaymentStatusRefunded marks a payment that has been operator-refunded via
	// IMP-15. Unlike `reversed` (auto-triggered by a chain reorg detector) this
	// is an intentional, audited refund. The bill row reflects the new paid
	// total after the refund lands.
	PaymentStatusRefunded PaymentStatus = "refunded"
)

// WithdrawalHistory represents a business withdrawal/claim transaction
type WithdrawalHistory struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	BusinessID        uint       `gorm:"index;not null" json:"business_id"`
	TransactionHash   string     `gorm:"uniqueIndex;not null" json:"transaction_hash"`
	PaymentAmount     int64      `gorm:"default:0" json:"payment_amount"`    // Amount from payment earnings (cents)
	TipAmount         int64      `gorm:"default:0" json:"tip_amount"`        // Amount from tip earnings (cents)
	TotalAmount       int64      `gorm:"not null" json:"total_amount"`       // Total withdrawn (cents)
	WithdrawalAddress string     `gorm:"not null" json:"withdrawal_address"` // Address that received the funds
	BlockchainNetwork string     `gorm:"not null" json:"blockchain_network"` // e.g., "base-sepolia", "ethereum"
	Status            string     `gorm:"default:'pending'" json:"status"`    // pending, confirmed, failed
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	Business          Business   `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// AlternativePayment represents a non-crypto payment (cash, card, etc.)
type AlternativePayment struct {
	ID              uint                     `gorm:"primaryKey" json:"id"`
	BillID          uint                     `gorm:"not null;index:idx_alternative_payments_pending_expiry,where:status = 'pending',priority:1;uniqueIndex:idx_alternative_payments_bill_idempotency_hash,where:idempotency_key_hash IS NOT NULL AND idempotency_key_hash <> '',priority:1" json:"bill_id"`
	ParticipantAddr string                   `gorm:"not null" json:"participant_address"`
	ParticipantName string                   `json:"participant_name"` // Optional name for identification
	Amount          int64                    `gorm:"not null" json:"amount"`
	BillAmountCents int64                    `gorm:"not null;default:0" json:"bill_amount_cents"`
	TipAmountCents  int64                    `gorm:"not null;default:0" json:"tip_amount_cents"`
	PaymentMethod   AlternativePaymentMethod `gorm:"not null" json:"payment_method"`
	Status          AlternativePaymentStatus `gorm:"default:'pending'" json:"status"`
	// IdempotencyKey is the per-attempt key (from the Idempotency-Key header)
	// that lets a retried request dedupe while two genuinely separate tenders of
	// the same amount remain distinct. Enforced by a partial unique index
	// (bill_id, idempotency_key) WHERE idempotency_key <> ''.
	IdempotencyKey string `gorm:"index" json:"-"`
	// Guest request identities are stored only as fixed-length SHA-256 digests.
	// PayloadHash binds a key to its canonical request so an exact retry can
	// replay while key reuse with different money or tender details conflicts.
	IdempotencyKeyHash string     `gorm:"size:64;index;uniqueIndex:idx_alternative_payments_bill_idempotency_hash,where:idempotency_key_hash IS NOT NULL AND idempotency_key_hash <> '',priority:2;check:alternative_payments_request_hashes_check,(COALESCE(idempotency_key_hash, '') = '' AND COALESCE(payload_hash, '') = '') OR (length(idempotency_key_hash) = 64 AND length(payload_hash) = 64)" json:"-"`
	PayloadHash        string     `gorm:"size:64" json:"-"`
	ExpiresAt          *time.Time `gorm:"index;index:idx_alternative_payments_pending_expiry,where:status = 'pending',priority:2" json:"expires_at,omitempty"`
	ConfirmedBy        string     `json:"confirmed_by"` // Business owner who confirmed
	ResolvedBy         string     `gorm:"size:255;not null;default:''" json:"resolved_by,omitempty"`
	ResolutionReason   string     `gorm:"size:500;not null;default:''" json:"resolution_reason,omitempty"`
	// PayerGuestSession is the guestsession.Fingerprint of the guest browser
	// session that requested this tender or opened this provider checkout;
	// NULL for staff-recorded rows. Payment proof for the guest fiscal identity
	// binding. Never serialized.
	PayerGuestSession *string    `gorm:"column:payer_guest_session;size:64" json:"-"`
	CreatedAt         time.Time  `json:"created_at"`
	UpdatedAt         time.Time  `json:"updated_at"`
	ConfirmedAt       *time.Time `json:"confirmed_at"`
	ResolvedAt        *time.Time `json:"resolved_at,omitempty"`
	Bill              Bill       `gorm:"foreignKey:BillID" json:"bill,omitempty"`
}

// AlternativePaymentMethod represents the method used for alternative payment.
// Canonical operator vocabulary (reporting.Method) maps:
//
//	cash → cash, card → card, venmo → wallet, other → other.
//
// Plugin names (stripe, paypal, mercadopago) are also written here
// by plugin settlement paths and canonicalize by settlement type.
type AlternativePaymentMethod string

const (
	PaymentMethodCash  AlternativePaymentMethod = "cash"
	PaymentMethodCard  AlternativePaymentMethod = "card"
	PaymentMethodVenmo AlternativePaymentMethod = "venmo" // canonicalizes to wallet
	PaymentMethodOther AlternativePaymentMethod = "other"
)

// AlternativePaymentStatus represents the status of an alternative payment
type AlternativePaymentStatus string

const (
	AltPaymentStatusPending   AlternativePaymentStatus = "pending"
	AltPaymentStatusConfirmed AlternativePaymentStatus = "confirmed"
	AltPaymentStatusFailed    AlternativePaymentStatus = "failed"
	AltPaymentStatusRefunded  AlternativePaymentStatus = "refunded"
	AltPaymentStatusExpired   AlternativePaymentStatus = "expired"
	AltPaymentStatusCancelled AlternativePaymentStatus = "cancelled"
	AltPaymentStatusRejected  AlternativePaymentStatus = "rejected"
)

type CashRegisterSessionStatus string

const (
	CashRegisterSessionStatusOpen   CashRegisterSessionStatus = "open"
	CashRegisterSessionStatusClosed CashRegisterSessionStatus = "closed"
)

type CashRegisterMovementType string

const (
	CashRegisterMovementTypeCashSale   CashRegisterMovementType = "cash_sale"
	CashRegisterMovementTypeCashRefund CashRegisterMovementType = "cash_refund"
	CashRegisterMovementTypeCashIn     CashRegisterMovementType = "cash_in"
	CashRegisterMovementTypeCashOut    CashRegisterMovementType = "cash_out"
)

type CashRegisterSession struct {
	ID                uint                      `gorm:"primaryKey" json:"id"`
	BusinessID        uint                      `gorm:"index;uniqueIndex:idx_cash_register_sessions_one_open_per_business,where:status = 'open';index:idx_cash_register_sessions_business_opened_at,priority:1;index:idx_cash_register_sessions_business_closed_at,priority:1;not null" json:"business_id"`
	Status            CashRegisterSessionStatus `gorm:"size:16;not null;default:'open';check:cash_register_sessions_status_check,status IN ('open','closed')" json:"status"`
	OpeningFloatCents int64                     `gorm:"not null;default:0;check:cash_register_sessions_opening_float_check,opening_float_cents >= 0" json:"opening_float_cents"`
	OpeningNote       string                    `gorm:"type:text;not null;default:''" json:"opening_note"`
	OpenedByUserID    *uint                     `gorm:"index" json:"opened_by_user_id,omitempty"`
	OpenedByStaffID   *uint                     `gorm:"index" json:"opened_by_staff_id,omitempty"`
	OpenedByLabel     string                    `gorm:"size:255;not null;default:''" json:"opened_by_label"`
	OpenedAt          time.Time                 `gorm:"index:idx_cash_register_sessions_business_opened_at,priority:2,sort:desc;not null" json:"opened_at"`
	CashSalesCents    int64                     `gorm:"not null;default:0" json:"cash_sales_cents"`
	CashRefundsCents  int64                     `gorm:"not null;default:0" json:"cash_refunds_cents"`
	CashInCents       int64                     `gorm:"not null;default:0" json:"cash_in_cents"`
	CashOutCents      int64                     `gorm:"not null;default:0" json:"cash_out_cents"`
	ExpectedCashCents int64                     `gorm:"not null;default:0" json:"expected_cash_cents"`
	CountedCashCents  int64                     `gorm:"not null;default:0;check:cash_register_sessions_counted_cash_check,counted_cash_cents >= 0" json:"counted_cash_cents"`
	VarianceCents     int64                     `gorm:"not null;default:0" json:"variance_cents"`
	ClosingNote       string                    `gorm:"type:text;not null;default:''" json:"closing_note"`
	ClosedByUserID    *uint                     `gorm:"index" json:"closed_by_user_id,omitempty"`
	ClosedByStaffID   *uint                     `gorm:"index" json:"closed_by_staff_id,omitempty"`
	ClosedByLabel     string                    `gorm:"size:255;not null;default:''" json:"closed_by_label"`
	ClosedAt          *time.Time                `gorm:"index:idx_cash_register_sessions_business_closed_at,priority:2,sort:desc" json:"closed_at"`
	CreatedAt         time.Time                 `json:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at"`

	Business      Business               `gorm:"foreignKey:BusinessID;constraint:OnDelete:CASCADE" json:"-"`
	Movements     []CashRegisterMovement `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
	OpenedBy      *User                  `gorm:"foreignKey:OpenedByUserID;constraint:OnDelete:SET NULL" json:"-"`
	OpenedByStaff *Staff                 `gorm:"foreignKey:OpenedByStaffID;constraint:OnDelete:SET NULL" json:"-"`
	ClosedBy      *User                  `gorm:"foreignKey:ClosedByUserID;constraint:OnDelete:SET NULL" json:"-"`
	ClosedByStaff *Staff                 `gorm:"foreignKey:ClosedByStaffID;constraint:OnDelete:SET NULL" json:"-"`
}

func (CashRegisterSession) TableName() string { return "cash_register_sessions" }

type CashRegisterMovement struct {
	ID                   uint                     `gorm:"primaryKey" json:"id"`
	BusinessID           uint                     `gorm:"index;index:idx_cash_register_movements_business_occurred,priority:1;not null" json:"business_id"`
	SessionID            uint                     `gorm:"index;index:idx_cash_register_movements_session_occurred,priority:1;not null" json:"session_id"`
	MovementType         CashRegisterMovementType `gorm:"size:32;not null;check:cash_register_movements_type_check,movement_type IN ('cash_sale','cash_refund','cash_in','cash_out');uniqueIndex:idx_cash_register_movements_alt_payment_type_unique,where:alternative_payment_id IS NOT NULL,priority:1" json:"movement_type"`
	AmountCents          int64                    `gorm:"not null;check:cash_register_movements_amount_check,amount_cents > 0" json:"amount_cents"`
	Reason               string                   `gorm:"size:80;not null;default:'';check:cash_register_movements_manual_reason_check,movement_type NOT IN ('cash_in','cash_out') OR length(trim(reason)) > 0" json:"reason"`
	Note                 string                   `gorm:"type:text;not null;default:''" json:"note"`
	AlternativePaymentID *uint                    `gorm:"index:idx_cash_register_movements_alt_payment,where:alternative_payment_id IS NOT NULL;uniqueIndex:idx_cash_register_movements_alt_payment_type_unique,where:alternative_payment_id IS NOT NULL,priority:2" json:"alternative_payment_id,omitempty"`
	BillID               *uint                    `gorm:"index" json:"bill_id,omitempty"`
	ActorUserID          *uint                    `gorm:"index" json:"actor_user_id,omitempty"`
	ActorStaffID         *uint                    `gorm:"index" json:"actor_staff_id,omitempty"`
	ActorLabel           string                   `gorm:"size:255;not null;default:''" json:"actor_label"`
	OccurredAt           time.Time                `gorm:"index:idx_cash_register_movements_session_occurred,priority:2,sort:desc;index:idx_cash_register_movements_business_occurred,priority:2,sort:desc;not null" json:"occurred_at"`
	CreatedAt            time.Time                `json:"created_at"`

	Session            CashRegisterSession `gorm:"foreignKey:SessionID;constraint:OnDelete:CASCADE" json:"-"`
	AlternativePayment *AlternativePayment `gorm:"foreignKey:AlternativePaymentID;constraint:OnDelete:SET NULL" json:"-"`
	Bill               *Bill               `gorm:"foreignKey:BillID;constraint:OnDelete:SET NULL" json:"-"`
	ActorUser          *User               `gorm:"foreignKey:ActorUserID;constraint:OnDelete:SET NULL" json:"-"`
	ActorStaff         *Staff              `gorm:"foreignKey:ActorStaffID;constraint:OnDelete:SET NULL" json:"-"`
}

func (CashRegisterMovement) TableName() string { return "cash_register_movements" }

// PaymentBreakdown represents the breakdown of crypto vs alternative payments
type PaymentBreakdown struct {
	TotalAmount     int64 `json:"total_amount"`
	CryptoPaid      int64 `json:"crypto_paid"`
	AlternativePaid int64 `json:"alternative_paid"`
	Remaining       int64 `json:"remaining"`
	IsComplete      bool  `json:"is_complete"`
}

// AccountingEntryType represents the type of a manual accounting entry.
type AccountingEntryType string

const (
	AccountingEntryTypeIncome  AccountingEntryType = "income"
	AccountingEntryTypeExpense AccountingEntryType = "expense"
)

// ManualLedgerEntry stores append-only business income and expense records.
type ManualLedgerEntry struct {
	ID               uint                `gorm:"primaryKey" json:"id"`
	BusinessID       uint                `gorm:"index;not null" json:"business_id"`
	EntryType        AccountingEntryType `gorm:"index;not null" json:"entry_type"`
	Category         string              `gorm:"index;not null" json:"category"`
	Amount           int64               `gorm:"not null" json:"amount"`
	Currency         string              `gorm:"not null" json:"currency"`
	OccurredAt       time.Time           `gorm:"index;not null" json:"occurred_at"`
	Description      string              `gorm:"not null" json:"description"`
	Notes            string              `gorm:"type:text" json:"notes"`
	Reference        string              `json:"reference"`
	CreatedByUserID  *uint               `gorm:"index" json:"created_by_user_id,omitempty"`
	CreatedByStaffID *uint               `gorm:"index" json:"created_by_staff_id,omitempty"`
	VoidedAt         *time.Time          `gorm:"index" json:"voided_at,omitempty"`
	VoidedByUserID   *uint               `gorm:"index" json:"voided_by_user_id,omitempty"`
	VoidedByStaffID  *uint               `gorm:"index" json:"voided_by_staff_id,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`

	// Business is GORM-only; ManualLedgerEntry.MarshalJSON already emits amount
	// as dollars and list rows must not embed empty business (FIND-038).
	Business       Business `gorm:"foreignKey:BusinessID" json:"-"`
	CreatedByStaff *Staff   `gorm:"foreignKey:CreatedByStaffID" json:"created_by_staff,omitempty"`
	// Owner-created entries (CreatedByUserID) resolve via this association —
	// L6-15: list/detail preload with selected columns only.
	CreatedByUser *User  `gorm:"foreignKey:CreatedByUserID" json:"created_by_user,omitempty"`
	VoidedByStaff *Staff `gorm:"foreignKey:VoidedByStaffID" json:"voided_by_staff,omitempty"`
	// AttachmentCount is computed on list (not a DB column). GORM "->" = read-only.
	AttachmentCount int `gorm:"->" json:"attachment_count"`
}

// RecurringEntryTemplate drives the hourly recurring-entries scheduler.
// Amount is int64 cents.
type RecurringEntryTemplate struct {
	ID               uint                `gorm:"primaryKey" json:"id"`
	BusinessID       uint                `gorm:"index;not null" json:"business_id"`
	EntryType        AccountingEntryType `gorm:"size:16;not null" json:"entry_type"`
	Category         string              `gorm:"size:64;not null" json:"category"`
	AmountCents      int64               `gorm:"column:amount_cents;not null" json:"amount_cents"`
	Currency         string              `gorm:"size:8;not null;default:USD" json:"currency"`
	Description      string              `gorm:"type:text;not null" json:"description"`
	Notes            string              `gorm:"type:text;not null;default:''" json:"notes"`
	Reference        string              `gorm:"size:160;not null;default:''" json:"reference"`
	Cadence          string              `gorm:"size:16;not null" json:"cadence"` // monthly|weekly
	AnchorDay        int                 `gorm:"not null" json:"anchor_day"`      // 1-28 (monthly) or weekday 0-6
	NextRunOn        time.Time           `gorm:"type:date;not null;index" json:"next_run_on"`
	Active           bool                `gorm:"not null;default:true" json:"active"`
	NeedsAttention   bool                `gorm:"not null;default:false" json:"needs_attention"`
	CreatedByUserID  *uint               `json:"created_by_user_id,omitempty"`
	CreatedByStaffID *uint               `json:"created_by_staff_id,omitempty"`
	LastGeneratedAt  *time.Time          `json:"last_generated_at,omitempty"`
	CreatedAt        time.Time           `json:"created_at"`
	UpdatedAt        time.Time           `json:"updated_at"`
}

func (RecurringEntryTemplate) TableName() string { return "recurring_entry_templates" }

// LedgerEntryAttachment is a protected-S3 file linked to a manual ledger entry.
type LedgerEntryAttachment struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	EntryID           uint      `gorm:"index;not null" json:"entry_id"`
	BusinessID        uint      `gorm:"index;not null" json:"business_id"`
	S3Key             string    `gorm:"type:text;not null" json:"-"`
	FileName          string    `gorm:"size:255;not null" json:"file_name"`
	ContentType       string    `gorm:"size:128;not null;default:''" json:"content_type"`
	SizeBytes         int64     `gorm:"not null;default:0" json:"size_bytes"`
	UploadedByUserID  *uint     `json:"uploaded_by_user_id,omitempty"`
	UploadedByStaffID *uint     `json:"uploaded_by_staff_id,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func (LedgerEntryAttachment) TableName() string { return "ledger_entry_attachments" }

// AccountingPeriodLock is an append-only close-the-books event.
// Authoritative locked_through is the latest row for the business (NULL = fully open).
type AccountingPeriodLock struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	BusinessID      uint       `gorm:"index;not null" json:"business_id"`
	LockedThrough   *time.Time `gorm:"type:date" json:"locked_through"`
	LockedByUserID  *uint      `json:"locked_by_user_id,omitempty"`
	LockedByStaffID *uint      `json:"locked_by_staff_id,omitempty"`
	Note            string     `gorm:"type:text;not null;default:''" json:"note"`
	CreatedAt       time.Time  `json:"created_at"`
}

func (AccountingPeriodLock) TableName() string { return "accounting_period_locks" }

// AccountingCategory is a business-defined EXTRA category.
// Hardcoded defaults stay in FE/BE consts; this table never replaces them.
type AccountingCategory struct {
	ID         uint                `gorm:"primaryKey" json:"id"`
	BusinessID uint                `gorm:"not null;uniqueIndex:idx_acct_cat_biz_key_type" json:"business_id"`
	Key        string              `gorm:"size:64;not null;uniqueIndex:idx_acct_cat_biz_key_type" json:"key"`
	Label      string              `gorm:"size:128;not null" json:"label"`
	EntryType  AccountingEntryType `gorm:"size:16;not null;uniqueIndex:idx_acct_cat_biz_key_type" json:"entry_type"`
	Active     bool                `gorm:"not null;default:true" json:"active"`
	Position   int                 `gorm:"not null;default:0" json:"position"`
	CreatedAt  time.Time           `json:"created_at"`
	UpdatedAt  time.Time           `json:"updated_at"`
}

func (AccountingCategory) TableName() string { return "accounting_categories" }

// PayrollRunStatus represents the lifecycle status of a payroll run.
type PayrollRunStatus string

const (
	PayrollRunStatusDraft PayrollRunStatus = "draft"
	PayrollRunStatusPaid  PayrollRunStatus = "paid"
	// PayrollRunStatusVoid marks a previously-paid run that was reversed. Voided
	// runs are excluded from P&L but retained for the audit trail.
	PayrollRunStatusVoid PayrollRunStatus = "void"
)

// PayrollPayeeType identifies whether a payroll line is tied to staff or a contractor.
type PayrollPayeeType string

const (
	PayrollPayeeTypeStaff      PayrollPayeeType = "staff"
	PayrollPayeeTypeContractor PayrollPayeeType = "contractor"
)

// PayrollRun stores a register of payroll runs for reporting.
type PayrollRun struct {
	ID          uint             `gorm:"primaryKey" json:"id"`
	BusinessID  uint             `gorm:"index;not null" json:"business_id"`
	PeriodStart time.Time        `gorm:"index;not null" json:"period_start"`
	PeriodEnd   time.Time        `gorm:"index;not null" json:"period_end"`
	Status      PayrollRunStatus `gorm:"index;default:'draft'" json:"status"`
	// Currency is the reporting currency stamped at creation (business default).
	// P&L converts each run's totals from this currency, mirroring manual ledger
	// entries — so a later change to the business currency cannot silently
	// re-value historical payroll.
	Currency         string     `gorm:"type:varchar(8)" json:"currency"`
	PaidAt           *time.Time `gorm:"index" json:"paid_at,omitempty"`
	PaidByUserID     *uint      `gorm:"index" json:"paid_by_user_id,omitempty"`
	PaidByStaffID    *uint      `gorm:"index" json:"paid_by_staff_id,omitempty"`
	VoidedAt         *time.Time `gorm:"index" json:"voided_at,omitempty"`
	VoidedByUserID   *uint      `gorm:"index" json:"voided_by_user_id,omitempty"`
	VoidedByStaffID  *uint      `gorm:"index" json:"voided_by_staff_id,omitempty"`
	Notes            string     `gorm:"type:text" json:"notes"`
	GrossTotal       int64      `gorm:"default:0" json:"gross_total"`
	BonusTotal       int64      `gorm:"default:0" json:"bonus_total"`
	DeductionTotal   int64      `gorm:"default:0" json:"deduction_total"`
	NetTotal         int64      `gorm:"default:0" json:"net_total"`
	CreatedByUserID  *uint      `gorm:"index" json:"created_by_user_id,omitempty"`
	CreatedByStaffID *uint      `gorm:"index" json:"created_by_staff_id,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`

	// Business is GORM-only. Non-pointer zero structs never omit under
	// encoding/json omitempty and dumped ~2.6KB empty business on every run
	// (L6-16 / T-1-adjacent). Clients already have business_id.
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
	// Actor staff for the detail drawer (created / paid / voided by). GetPayrollRun
	// preloads a narrow id+name projection so MarshalJSON emits the slim shape.
	CreatedByStaff *Staff            `gorm:"foreignKey:CreatedByStaffID" json:"created_by_staff,omitempty"`
	PaidByStaff    *Staff            `gorm:"foreignKey:PaidByStaffID" json:"paid_by_staff,omitempty"`
	VoidedByStaff  *Staff            `gorm:"foreignKey:VoidedByStaffID" json:"voided_by_staff,omitempty"`
	LineItems      []PayrollLineItem `gorm:"foreignKey:PayrollRunID" json:"line_items,omitempty"`
}

// PayrollLineItem stores a single payee in a payroll run.
type PayrollLineItem struct {
	ID              uint             `gorm:"primaryKey" json:"id"`
	PayrollRunID    uint             `gorm:"index;not null" json:"payroll_run_id"`
	BusinessID      uint             `gorm:"index;not null" json:"business_id"`
	PayeeType       PayrollPayeeType `gorm:"index;not null" json:"payee_type"`
	StaffID         *uint            `gorm:"index" json:"staff_id,omitempty"`
	PayeeName       string           `gorm:"not null" json:"payee_name"`
	GrossAmount     int64            `gorm:"default:0" json:"gross_amount"`
	BonusAmount     int64            `gorm:"default:0" json:"bonus_amount"`
	DeductionAmount int64            `gorm:"default:0" json:"deduction_amount"`
	NetAmount       int64            `gorm:"default:0" json:"net_amount"`
	Notes           string           `gorm:"type:text" json:"notes"`
	CreatedAt       time.Time        `json:"created_at"`
	UpdatedAt       time.Time        `json:"updated_at"`

	// GORM-only relations. Zero-value embeds never omit under encoding/json
	// omitempty: each line used to re-serialize an empty PayrollRun (itself
	// carrying another empty Business) plus an empty Business — ~5.8KB/line of
	// recursive bloat on GET payroll-run detail (L6-16). FE reads payee_name
	// and money fields only; staff identity is denormalized into payee_name
	// at create time.
	PayrollRun PayrollRun `gorm:"foreignKey:PayrollRunID" json:"-"`
	Business   Business   `gorm:"foreignKey:BusinessID" json:"-"`
	Staff      *Staff     `gorm:"foreignKey:StaffID" json:"-"`
}

// Staff represents employees/workers of a business
type Staff struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	BusinessID  uint       `gorm:"index;not null" json:"business_id"`
	Email       string     `gorm:"not null" json:"email"`
	Name        string     `gorm:"not null" json:"name"`
	Role        StaffRole  `gorm:"not null" json:"role"`
	IsActive    bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt *time.Time `json:"last_login_at"`
	InvitedBy   string     `gorm:"not null" json:"invited_by"` // Owner wallet address

	// RBAC Extensions
	CustomPermissions    string     `json:"custom_permissions"`          // JSON array of additional permissions beyond role defaults
	RoleLevel            int        `gorm:"default:0" json:"role_level"` // Hierarchy level for role management
	PermissionsUpdatedAt *time.Time `json:"permissions_updated_at"`      // When permissions were last modified
	PermissionsUpdatedBy string     `json:"permissions_updated_by"`      // Who last updated permissions (wallet address or email)

	// AuthzVersion is a monotonic access-revocation counter. Staff JWTs embed
	// the value at issuance; hydrateLiveStaffContext and SSE heartbeats reject
	// any token whose claim no longer matches the live row. Default 1.
	AuthzVersion int `gorm:"not null;default:1" json:"authz_version"`

	// Compensation (owner-only; exposed only via the dedicated compensation
	// sub-resource, never inline on the staff payload). Money stored in cents.
	EmploymentType    string `gorm:"type:varchar(16);default:''" json:"-"`
	HourlyRateCents   int64  `gorm:"default:0" json:"-"`
	AnnualSalaryCents int64  `gorm:"default:0" json:"-"`

	// Manager PIN (IMP-14). `PinHash` is a bcrypt hash; nil means the staff
	// has not enrolled a PIN yet. `PinSetAt` records the most recent rotation.
	// Both columns are stripped from JSON serialization to keep hashes out of
	// the wire — never expose them to the client.
	PinHash  string     `gorm:"column:pin_hash" json:"-"`
	PinSetAt *time.Time `gorm:"column:pin_set_at" json:"-"`

	// Email login-code brute-force protection. Failed verifications increment
	// LoginCodeFailedAttempts; once the threshold is hit, LoginCodeLockedUntil
	// blocks further attempts for this staff member regardless of source IP.
	// Both reset on a successful login. Stripped from JSON.
	LoginCodeFailedAttempts int        `gorm:"default:0" json:"-"`
	LoginCodeLockedUntil    *time.Time `json:"-"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	// Business is GORM-only. Non-pointer zero structs never omit under
	// encoding/json omitempty and used to dump ~3KB empty business fields on
	// every staff list/invite row. Clients already have business_id; staff
	// session payloads expose business_name explicitly.
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// StaffIdentity is the canonical, business-independent identity for staff
// authentication. During the expand/dual-write phase Staff remains the scoped
// operational row (and foreign-key anchor); memberships link that legacy row to
// one normalized identity without changing existing consumers.
type StaffIdentity struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	NormalizedEmail string    `gorm:"size:320;not null;uniqueIndex" json:"normalized_email"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (StaffIdentity) TableName() string { return "staff_identities" }

// StaffMembership carries authorization and revocation state for exactly one
// identity in one business. LegacyStaffID is deliberately retained through the
// expand phase so existing JWTs, sessions, and domain foreign keys stay scoped
// and valid while reads and writes are migrated incrementally.
type StaffMembership struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	IdentityID           uint       `gorm:"not null;uniqueIndex:idx_staff_memberships_identity_business,priority:1" json:"identity_id"`
	BusinessID           uint       `gorm:"not null;index;uniqueIndex:idx_staff_memberships_identity_business,priority:2" json:"business_id"`
	LegacyStaffID        uint       `gorm:"not null;uniqueIndex:idx_staff_memberships_legacy_staff" json:"legacy_staff_id"`
	Name                 string     `gorm:"not null" json:"name"`
	Role                 StaffRole  `gorm:"not null" json:"role"`
	CustomPermissions    string     `json:"custom_permissions"`
	RoleLevel            int        `gorm:"default:0" json:"role_level"`
	IsActive             bool       `gorm:"not null;default:true" json:"is_active"`
	AuthzVersion         int        `gorm:"not null;default:1" json:"authz_version"`
	InvitedBy            string     `gorm:"not null" json:"invited_by"`
	PermissionsUpdatedAt *time.Time `json:"permissions_updated_at"`
	PermissionsUpdatedBy string     `json:"permissions_updated_by"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

func (StaffMembership) TableName() string { return "staff_memberships" }

// StaffPermissionDeny is an explicit permission deny override for a staff member.
// Source of truth for denies (grants remain on Staff.CustomPermissions JSON).
// Effective permissions = (role grants ∪ custom grants) − denies, then owner-only invariant.
type StaffPermissionDeny struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"not null;uniqueIndex:idx_staff_permission_denies_unique,priority:1" json:"business_id"`
	StaffID    uint      `gorm:"not null;index:idx_staff_permission_denies_staff_id;uniqueIndex:idx_staff_permission_denies_unique,priority:2" json:"staff_id"`
	Permission string    `gorm:"size:128;not null;uniqueIndex:idx_staff_permission_denies_unique,priority:3" json:"permission"`
	CreatedBy  string    `gorm:"size:255;not null;default:''" json:"created_by"`
	Reason     string    `gorm:"type:text" json:"reason"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`

	Staff    Staff    `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// TableName for StaffPermissionDeny.
func (StaffPermissionDeny) TableName() string {
	return "staff_permission_denies"
}

// StaffInvitation represents pending staff invitations
type StaffInvitation struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	BusinessID uint      `gorm:"index;not null" json:"business_id"`
	Email      string    `gorm:"not null" json:"email"`
	Name       string    `gorm:"not null" json:"name"`
	Role       StaffRole `gorm:"not null" json:"role"`
	// Token is the secret capability for accept-invitation. Never serialize it
	// on list/API responses (staff:read is held by kitchen/server/host too).
	// Deliver only via email/invite URL or staff:invite-gated link endpoint.
	Token     string           `gorm:"uniqueIndex;not null" json:"-"`
	Status    InvitationStatus `gorm:"default:'pending'" json:"status"`
	InvitedBy string           `gorm:"not null" json:"invited_by"` // Owner wallet address
	ExpiresAt time.Time        `json:"expires_at"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
	Business  Business         `gorm:"foreignKey:BusinessID" json:"-"`
}

// StaffLoginCode represents temporary login codes for staff
type StaffLoginCode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	StaffID   uint      `gorm:"index:idx_staff_login_codes_staff_code,priority:1;not null" json:"staff_id"`
	Code      string    `gorm:"index:idx_staff_login_codes_code;index:idx_staff_login_codes_staff_code,priority:2;not null" json:"code"`
	ExpiresAt time.Time `gorm:"index:idx_staff_login_codes_expires_at" json:"expires_at"`
	Used      bool      `gorm:"default:false" json:"used"`
	CreatedAt time.Time `json:"created_at"`
	Staff     Staff     `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
}

// StaffRole represents different staff permission levels
type StaffRole string

const (
	StaffRoleManager StaffRole = "manager" // Can manage menu, tables, bills (no financial settings)
	StaffRoleServer  StaffRole = "server"  // Can view tables, create/close bills
	StaffRoleHost    StaffRole = "host"    // Can view tables, seat guests
	StaffRoleKitchen StaffRole = "kitchen" // Can view orders, mark items ready
)

// InvitationStatus represents the status of a staff invitation
type InvitationStatus string

const (
	InvitationStatusPending  InvitationStatus = "pending"
	InvitationStatusAccepted InvitationStatus = "accepted"
	InvitationStatusExpired  InvitationStatus = "expired"
	InvitationStatusRevoked  InvitationStatus = "revoked"
)

// TableName methods to specify custom table names if needed
func (User) TableName() string {
	return "users"
}

func (ErrorLog) TableName() string {
	return "error_logs"
}

func (Business) TableName() string {
	return "businesses"
}

func (Menu) TableName() string {
	return "menus"
}

func (Table) TableName() string {
	return "tables"
}

func (Bill) TableName() string {
	return "bills"
}

func (Payment) TableName() string {
	return "payments"
}

func (AlternativePayment) TableName() string {
	return "alternative_payments"
}

func (ManualLedgerEntry) TableName() string {
	return "manual_ledger_entries"
}

func (PayrollRun) TableName() string {
	return "payroll_runs"
}

func (PayrollLineItem) TableName() string {
	return "payroll_line_items"
}

func (Staff) TableName() string {
	return "staff"
}

func (StaffInvitation) TableName() string {
	return "staff_invitations"
}

func (StaffLoginCode) TableName() string {
	return "staff_login_codes"
}

// OrderPriority represents the priority level of an order
type OrderPriority string

const (
	PriorityLow    OrderPriority = "low"
	PriorityNormal OrderPriority = "normal"
	PriorityHigh   OrderPriority = "high"
	PriorityUrgent OrderPriority = "urgent"
)

// OrderStats represents order performance statistics
type OrderStats struct {
	BusinessID      uint    `json:"business_id"`
	Date            string  `json:"date"`
	TotalOrders     int     `json:"total_orders"`
	CompletedOrders int     `json:"completed_orders"`
	CancelledOrders int     `json:"cancelled_orders"`
	AverageTime     float64 `json:"average_time"` // Average preparation time in minutes
	PeakHour        string  `json:"peak_hour"`
	TotalRevenue    float64 `json:"total_revenue"`
}

// Order represents a small order within a bill (guest requests)
type Order struct {
	ID              uint        `gorm:"primaryKey" json:"id"`
	BillID          uint        `gorm:"index;not null;uniqueIndex:idx_orders_request_identity" json:"bill_id"`
	BusinessID      uint        `gorm:"index;not null" json:"business_id"`
	OrderNumber     string      `gorm:"not null" json:"order_number"` // Generated order number for display
	Status          OrderStatus `gorm:"not null;default:'pending'" json:"status"`
	Currency        string      `gorm:"-" json:"currency"`
	CreatedBy       string      `gorm:"uniqueIndex:idx_orders_request_identity" json:"created_by"` // "guest" or staff member address
	ClientRequestID *string     `gorm:"size:64;uniqueIndex:idx_orders_request_identity" json:"-"`
	ApprovedBy      string      `json:"approved_by"`            // staff member who approved
	CancelledBy     string      `json:"cancelled_by"`           // staff member who cancelled
	Notes           string      `json:"notes"`                  // Special instructions
	Items           string      `gorm:"type:text" json:"items"` // JSON snapshot; MarshalJSON emits an array (#771)
	// QuoteSnapshot freezes the cent-exact checkout quote for idempotent replay.
	// Legacy rows remain NULL and are reconstructed by the checkout service.
	QuoteSnapshot JSONRawMessage `gorm:"type:jsonb" json:"-"`
	CancelReason  string         `gorm:"type:text" json:"cancel_reason"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	ApprovedAt    *time.Time     `json:"approved_at"`
	CancelledAt   *time.Time     `json:"cancelled_at"`
	// KitchenAckedAt records the first time the Kitchen view rendered the
	// ticket for this order (IMP-04). Powers the
	// `payverge_order_to_kitchen_seconds` Prometheus metric and the
	// self-healing sweep that re-enqueues orphan orders.
	KitchenAckedAt *time.Time `gorm:"index" json:"kitchen_acked_at,omitempty"`

	// Relationships
	Bill     Bill     `gorm:"foreignKey:BillID" json:"bill,omitempty"`
	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`

	// Delivery is populated only by the business orders list for
	// guest-delivery orders (gorm:"-": never a column).
	Delivery *OrderDeliveryMeta `gorm:"-" json:"delivery,omitempty"`
}

// OrderDeliveryMeta is the pending-queue projection of a linked delivery order.
// Populated by GetOrdersByBusinessIDPaginated via a single batched query.
type OrderDeliveryMeta struct {
	DeliveryID       uint       `json:"delivery_id"`
	DeliveryNumber   string     `json:"delivery_number"`
	DeliveryStatus   string     `json:"delivery_status"`
	CustomerName     string     `json:"customer_name"`
	CustomerPhone    string     `json:"customer_phone"`
	Street           string     `json:"street"`
	City             string     `json:"city"`
	PaymentExpiresAt *time.Time `json:"payment_expires_at,omitempty"`
}

// OrderItem represents an item within an order
type OrderItem struct {
	ID                 string           `json:"id"`
	ItemType           string           `json:"item_type,omitempty"` // menu_item, bundle, bundle_item, discount
	MenuItemID         string           `json:"menu_item_id,omitempty"`
	BundleID           *uint            `json:"bundle_id,omitempty"`
	ParentBundleID     *uint            `json:"parent_bundle_id,omitempty"`
	BundleOccurrenceID string           `json:"bundle_occurrence_id,omitempty"`
	SourceOfferID      *uint            `json:"source_offer_id,omitempty"`
	MenuItemName       string           `json:"menu_item_name"`
	Quantity           int              `json:"quantity"`
	Price              float64          `json:"price"`
	Options            []MenuItemOption `json:"options"` // Add-ons/modifiers
	SpecialRequests    string           `json:"special_requests"`
	Subtotal           float64          `json:"subtotal"`
	// CreatedAt is optional on the wire. Leftover Date Night snapshots stored
	// year-1 / omitted it; read hydration stamps order.CreatedAt when needed.
	CreatedAt time.Time `json:"created_at,omitempty"`
}

// OrderStatus represents the status of an order
type OrderStatus string

const (
	OrderStatusPending        OrderStatus = "pending"    // Waiting for staff approval
	OrderStatusApproved       OrderStatus = "approved"   // Staff approved, ready for kitchen
	OrderStatusInKitchen      OrderStatus = "in_kitchen" // Sent to kitchen
	OrderStatusOrderReady     OrderStatus = "ready"      // Kitchen finished
	OrderStatusOrderDelivered OrderStatus = "delivered"  // Served to table
	OrderStatusOrderCancelled OrderStatus = "cancelled"  // Cancelled by staff
)

// IsValid returns true if the OrderStatus is one of the defined enum values.
func (s OrderStatus) IsValid() bool {
	switch s {
	case OrderStatusPending, OrderStatusApproved, OrderStatusInKitchen,
		OrderStatusOrderReady, OrderStatusOrderDelivered, OrderStatusOrderCancelled:
		return true
	}
	return false
}

// TableName method for Order model
func (Order) TableName() string {
	return "orders"
}

// Offer represents a special deal or discount
type Offer struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	BusinessID    uint       `gorm:"index;not null" json:"business_id"`
	Name          string     `gorm:"not null" json:"name"`
	Description   string     `json:"description"`
	Image         string     `json:"image"`
	DiscountType  string     `gorm:"not null" json:"discount_type"` // "percentage" or "fixed"
	DiscountValue float64    `gorm:"not null" json:"discount_value"`
	StartDate     *time.Time `json:"start_date"`
	EndDate       *time.Time `json:"end_date"`
	WeekdayMask   int16      `gorm:"column:weekday_mask;not null;default:127" json:"weekday_mask"`
	StartMinute   *int       `gorm:"column:start_minute" json:"start_minute,omitempty"`
	EndMinute     *int       `gorm:"column:end_minute" json:"end_minute,omitempty"`
	// No `gorm:"default:true"` on purpose. GORM omits a zero-value bool from
	// INSERT when the field has a default tag, so an explicit IsActive:false
	// would be silently flipped to true by the schema default — operators
	// could never create a draft/inactive offer. The "default active" policy
	// lives in the CreateOffer handler (isActive := true), which is the single
	// source of truth; the DB layer persists exactly what it is given.
	IsActive     bool      `json:"is_active"`
	ApplicableTo string    `gorm:"default:'all'" json:"applicable_to"` // "all", "category", "item", "bundle"
	TargetID     *string   `json:"target_id"`                          // ID of category/item/bundle
	Code         *string   `gorm:"size:50" json:"code,omitempty"`      // optional promo code (NULL = auto-apply)
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	// Business relation is reachable to Go via gorm (server-side code can
	// still read offer.Business.*), but never serialized to JSON. The
	// guest menu endpoint serves these on an unauthenticated path and the
	// full Business struct carries Stripe IDs, owner PII, and payout
	// addresses; `omitempty` is a no-op for non-pointer struct fields so
	// the empty object was always emitted, advertising the schema. A
	// future Preload would turn it into a real leak. The frontend
	// `Offer` type in `frontend/src/api/business.ts` has no `business`
	// field and no JS/TS consumer reads offer.business. (SEC-2 cont.)
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// Bundle represents a group of items sold together
type Bundle struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	BusinessID  uint      `gorm:"index;not null" json:"business_id"`
	Name        string    `gorm:"not null" json:"name"`
	Description string    `json:"description"`
	Price       float64   `gorm:"not null" json:"price"`
	Currency    string    `json:"currency"`
	Image       string    `json:"image"`
	Items       string    `gorm:"type:text" json:"items"` // JSON array of item IDs
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	// Business relation: see Offer.Business for rationale — never
	// serialized to JSON, the guest menu endpoint has no consumer of
	// bundle.business, and the embedded struct would otherwise advertise
	// stripe_* / owner_* / settlement_address / tipping_address fields.
	Business Business `gorm:"foreignKey:BusinessID" json:"-"`
}

// BundleItemRef represents a menu item reference inside a bundle definition.
type BundleItemRef struct {
	MenuItemID string `json:"menu_item_id"`
	Name       string `json:"name"`
	Quantity   int    `json:"quantity"`
}

// RBACAction represents the type of RBAC action performed
type RBACAction string

const (
	RBACActionRoleChanged           RBACAction = "role_changed"
	RBACActionPermissionGranted     RBACAction = "permission_granted"
	RBACActionPermissionRevoked     RBACAction = "permission_revoked"
	RBACActionPermissionDenied      RBACAction = "permission_denied"
	RBACActionPermissionDenyRemoved RBACAction = "permission_deny_removed"
	RBACActionStaffCreated          RBACAction = "staff_created"
	RBACActionStaffDeactivated      RBACAction = "staff_deactivated"
	RBACActionStaffReactivated      RBACAction = "staff_reactivated"
	// RBACActionClaimStolen records an explicit claim steal (AI waiter or
	// delivery dispatch). Same audit table as coverage/time-entry domain
	// actions — do not invent a parallel audit store.
	RBACActionClaimStolen RBACAction = "claim_stolen"
	// RBACActionClaimForceReleased records a manager/owner force-release of
	// another actor's live claim (L4-8). Self-release is not audited.
	RBACActionClaimForceReleased RBACAction = "claim_force_released"
)

// RBACAuditLog represents audit trail for all RBAC changes
type RBACAuditLog struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	StaffID        uint       `gorm:"index;not null" json:"staff_id"`
	BusinessID     uint       `gorm:"index;not null" json:"business_id"`
	Action         RBACAction `gorm:"not null" json:"action"`
	OldRole        string     `json:"old_role"`                   // Previous role (for role changes)
	NewRole        string     `json:"new_role"`                   // New role (for role changes)
	OldPermissions string     `json:"old_permissions"`            // Previous custom permissions (JSON array)
	NewPermissions string     `json:"new_permissions"`            // New custom permissions (JSON array)
	ChangedBy      string     `gorm:"not null" json:"changed_by"` // Wallet address or staff email who made the change
	Reason         string     `json:"reason"`                     // Optional reason for the change
	IPAddress      string     `json:"ip_address"`                 // IP address of the change initiator
	UserAgent      string     `json:"user_agent"`                 // User agent of the change initiator
	CreatedAt      time.Time  `json:"created_at"`

	// Relationships
	Staff    Staff    `gorm:"foreignKey:StaffID" json:"staff,omitempty"`
	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// TableName method for RBACAuditLog model
func (RBACAuditLog) TableName() string {
	return "rbac_audit_logs"
}

// CompVoidAudit captures every comp / void / refund attempt for tamper-evident
// review (IMP-14). Rows are append-only; we write one regardless of whether
// the approving staff had a PIN configured at the time (PinPresent records
// that fact so auditors can distinguish unenrolled approvers from verified
// PIN-protected ones).
type CompVoidAudit struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	BusinessID  uint      `gorm:"index;not null" json:"business_id"`
	StaffID     *uint     `gorm:"index" json:"staff_id,omitempty"`
	TargetType  string    `gorm:"size:32;not null" json:"target_type"` // e.g. "bill_item", "bill", "payment"
	TargetID    string    `gorm:"size:64;not null" json:"target_id"`
	Action      string    `gorm:"size:16;not null" json:"action"` // "comp" | "void" | "refund"
	Reason      string    `json:"reason,omitempty"`
	AmountCents *int64    `gorm:"column:amount_cents" json:"amount_cents,omitempty"`
	PinPresent  bool      `gorm:"default:false" json:"pin_present"`
	IPAddress   string    `gorm:"size:64" json:"ip_address,omitempty"`
	UserAgent   string    `json:"user_agent,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TableName pins the table name (Postgres uses snake_case, plural-less).
func (CompVoidAudit) TableName() string {
	return "comp_void_audit"
}

// AdminAction represents an admin action taken on a user account
type AdminAction struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	AdminUserID  uint           `gorm:"index;not null" json:"admin_user_id"`
	TargetUserID uint           `gorm:"index;not null" json:"target_user_id"`
	ActionType   string         `gorm:"not null" json:"action_type"`
	Details      JSONRawMessage `gorm:"type:jsonb;default:'{}'" json:"details"`
	CreatedAt    time.Time      `json:"created_at"`
}

// TableName method for AdminAction model
func (AdminAction) TableName() string {
	return "admin_actions"
}

// PlatformSettings represents platform-wide configuration settings
// managed by platform admins.
type PlatformSettings struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Key       string    `gorm:"uniqueIndex;not null" json:"key"`
	Value     string    `gorm:"type:text" json:"value"` // Value is AES-GCM encrypted (v1: prefix) when IsSecret; legacy plaintext rows re-encrypt on next save.
	Category  string    `gorm:"index" json:"category"`  // e.g., "stripe", "email", "general"
	IsSecret  bool      `gorm:"default:false" json:"is_secret"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName method for PlatformSettings model
func (PlatformSettings) TableName() string {
	return "platform_settings"
}

// WebhookEvent stores payment-provider webhook events for idempotent processing.
type WebhookEvent struct {
	ID          uint       `gorm:"primaryKey" json:"id"`
	Provider    string     `gorm:"not null;uniqueIndex:idx_provider_event,priority:1" json:"provider"`
	WebhookID   string     `gorm:"not null;uniqueIndex:idx_provider_event,priority:2" json:"webhook_id"`
	EventType   string     `gorm:"not null;index" json:"event_type"`
	Status      string     `gorm:"default:'processing'" json:"status"`
	Payload     string     `gorm:"type:text" json:"payload"`
	Error       string     `json:"error"`
	ReceivedAt  time.Time  `gorm:"not null" json:"received_at"`
	ProcessedAt *time.Time `json:"processed_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// TableName method for WebhookEvent model
func (WebhookEvent) TableName() string {
	return "webhook_events"
}

// ReportFrequency represents how often a report should be sent
type ReportFrequency string

const (
	ReportFrequencyDaily  ReportFrequency = "daily"
	ReportFrequencyWeekly ReportFrequency = "weekly"
)

// ReportSchedule represents a scheduled email report for a business
type ReportSchedule struct {
	ID         uint            `gorm:"primaryKey" json:"id"`
	BusinessID uint            `gorm:"index;not null" json:"business_id"`
	Frequency  ReportFrequency `gorm:"not null" json:"frequency"` // daily or weekly
	DayOfWeek  int             `json:"day_of_week"`               // 0=Sunday, 1=Monday, etc. (only for weekly)
	Hour       int             `gorm:"not null" json:"hour"`      // Hour of day (0-23) in business timezone
	Minute     int             `gorm:"default:0" json:"minute"`   // Minute of hour (0-59)
	Timezone   string          `gorm:"default:'UTC'" json:"timezone"`
	IsActive   bool            `gorm:"default:true" json:"is_active"`
	LastSentAt *time.Time      `json:"last_sent_at"`
	NextSendAt time.Time       `gorm:"index" json:"next_send_at"` // Calculated next send time
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`

	// Relationships
	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// TableName method for ReportSchedule model
func (ReportSchedule) TableName() string {
	return "report_schedules"
}

func (Offer) TableName() string {
	return "offers"
}

func (Bundle) TableName() string {
	return "bundles"
}

// CRM Models

// Customer represents a customer in the CRM system
// Customers have a universal profile shared across all businesses
type Customer struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	Email               string     `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash        string     `json:"-"` // Bcrypt hash, not exposed in JSON
	Name                string     `json:"name"`
	Phone               string     `json:"phone"`
	WalletAddress       string     `gorm:"index" json:"wallet_address"` // Optional wallet connection
	Birthday            *time.Time `json:"birthday"`
	ProfileImageURL     string     `json:"profile_image_url"`
	IsActive            bool       `gorm:"default:true" json:"is_active"`
	EmailVerified       bool       `gorm:"default:false" json:"email_verified"`
	VerificationToken   string     `json:"-"`
	PasswordResetToken  string     `json:"-"`
	PasswordResetExpiry *time.Time `json:"-"`
	LastLoginAt         *time.Time `json:"last_login_at"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`

	// Relationships - use pointers to avoid circular dependency
	BusinessConnections []CustomerBusiness   `gorm:"foreignKey:CustomerID" json:"business_connections,omitempty"`
	Preferences         *CustomerPreferences `gorm:"foreignKey:CustomerID" json:"preferences,omitempty"`
}

// TableName method for Customer model
func (Customer) TableName() string {
	return "customers"
}

// CustomerBusiness represents the relationship between a customer and a business
// Each customer can be connected to multiple businesses with separate loyalty data
type CustomerBusiness struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	CustomerID         uint       `gorm:"index;uniqueIndex:idx_customer_business_unique;not null" json:"customer_id"`
	BusinessID         uint       `gorm:"index;uniqueIndex:idx_customer_business_unique;not null" json:"business_id"`
	LoyaltyPoints      int        `gorm:"default:0" json:"loyalty_points"`
	LoyaltyTier        string     `json:"loyalty_tier"`                 // Bronze, Silver, Gold, etc.
	TotalSpent         float64    `gorm:"default:0" json:"total_spent"` // Lifetime spending in USD
	VisitCount         int        `gorm:"default:0" json:"visit_count"`
	LastVisitAt        *time.Time `json:"last_visit_at"`
	FirstVisitAt       time.Time  `json:"first_visit_at"`
	OptInMarketing     bool       `gorm:"default:false" json:"opt_in_marketing"`
	OptInSMS           bool       `gorm:"default:false" json:"opt_in_sms"`
	OptInEmail         bool       `gorm:"default:true" json:"opt_in_email"`
	FavoriteItems      string     `json:"favorite_items"`      // JSON array of menu item IDs
	DietaryPreferences string     `json:"dietary_preferences"` // JSON array of preferences
	Allergies          string     `json:"allergies"`           // JSON array of allergies
	Notes              string     `json:"notes"`               // Business-specific notes about customer
	Tags               string     `json:"tags"`                // JSON array of custom tags
	IsActive           bool       `gorm:"default:true" json:"is_active"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`

	// Relationships
	Customer Customer `gorm:"foreignKey:CustomerID" json:"customer,omitempty"`
	// Business is GORM-only — omitempty never drops non-pointer zero structs
	// and list endpoints were dumping ~3KB empty business per CRM row.
	Business Business        `gorm:"foreignKey:BusinessID" json:"-"`
	Visits   []CustomerVisit `gorm:"foreignKey:CustomerBusinessID" json:"visits,omitempty"`
}

// TableName method for CustomerBusiness model
func (CustomerBusiness) TableName() string {
	return "customer_businesses"
}

// CustomerPreferences represents customer's global preferences
type CustomerPreferences struct {
	ID                      uint      `gorm:"primaryKey" json:"id"`
	CustomerID              uint      `gorm:"uniqueIndex;not null" json:"customer_id"`
	PreferredLanguage       string    `gorm:"default:'en'" json:"preferred_language"`
	PreferredCurrency       string    `gorm:"default:'USD'" json:"preferred_currency"`
	ReceivePromotions       bool      `gorm:"default:true" json:"receive_promotions"`
	ReceiveNewsletters      bool      `gorm:"default:true" json:"receive_newsletters"`
	ReceiveBirthdayOffers   bool      `gorm:"default:true" json:"receive_birthday_offers"`
	ShareDataWithBusinesses bool      `gorm:"default:true" json:"share_data_with_businesses"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

// TableName method for CustomerPreferences model
func (CustomerPreferences) TableName() string {
	return "customer_preferences"
}

// CustomerVisit represents a single visit/transaction by a customer at a business
type CustomerVisit struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	CustomerBusinessID uint      `gorm:"index;not null" json:"customer_business_id"`
	BillID             *uint     `gorm:"index" json:"bill_id"` // Link to actual bill if available
	TableID            *uint     `gorm:"index" json:"table_id"`
	AmountSpent        float64   `json:"amount_spent"` // In USD
	PointsEarned       int       `json:"points_earned"`
	ItemsPurchased     string    `json:"items_purchased"` // JSON array of item details
	VisitDate          time.Time `gorm:"index" json:"visit_date"`
	VisitDuration      int       `json:"visit_duration"` // In minutes
	Rating             *int      `json:"rating"`         // 1-5 star rating
	Feedback           string    `json:"feedback"`
	CreatedAt          time.Time `json:"created_at"`

	// Relationships
	CustomerBusiness CustomerBusiness `gorm:"foreignKey:CustomerBusinessID" json:"customer_business,omitempty"`
	Bill             *Bill            `gorm:"foreignKey:BillID" json:"bill,omitempty"`
	Table            *Table           `gorm:"foreignKey:TableID" json:"table,omitempty"`
}

// TableName method for CustomerVisit model
func (CustomerVisit) TableName() string {
	return "customer_visits"
}

// CommunicationType represents the type of communication
type CommunicationType string

const (
	CommunicationTypeEmail CommunicationType = "email"
	CommunicationTypeSMS   CommunicationType = "sms"
	CommunicationTypePush  CommunicationType = "push"
)

// CommunicationStatus represents the status of a communication
type CommunicationStatus string

const (
	CommunicationStatusPending   CommunicationStatus = "pending"
	CommunicationStatusSent      CommunicationStatus = "sent"
	CommunicationStatusDelivered CommunicationStatus = "delivered"
	CommunicationStatusFailed    CommunicationStatus = "failed"
	CommunicationStatusBounced   CommunicationStatus = "bounced"
)

// CustomerCommunication represents a communication sent to a customer
type CustomerCommunication struct {
	ID                 uint                `gorm:"primaryKey" json:"id"`
	BusinessID         uint                `gorm:"index;not null" json:"business_id"`
	CustomerBusinessID uint                `gorm:"index;not null" json:"customer_business_id"`
	Type               CommunicationType   `gorm:"not null" json:"type"`
	Status             CommunicationStatus `gorm:"default:'pending'" json:"status"`
	Subject            string              `json:"subject"`
	Content            string              `json:"content"` // Email body or SMS text
	ScheduledFor       *time.Time          `json:"scheduled_for"`
	SentAt             *time.Time          `json:"sent_at"`
	DeliveredAt        *time.Time          `json:"delivered_at"`
	OpenedAt           *time.Time          `json:"opened_at"`
	ClickedAt          *time.Time          `json:"clicked_at"`
	ErrorMessage       string              `json:"error_message"`
	CampaignID         string              `json:"campaign_id"` // For grouping related communications
	CreatedAt          time.Time           `json:"created_at"`
	UpdatedAt          time.Time           `json:"updated_at"`

	// Relationships
	Business         Business         `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	CustomerBusiness CustomerBusiness `gorm:"foreignKey:CustomerBusinessID" json:"customer_business,omitempty"`
}

// AI Waiter models

// AiWaiterConversation tracks a unique session between a guest and the AI
type AiWaiterConversation struct {
	ID             uint   `gorm:"primaryKey" json:"id"`
	SessionID      string `gorm:"uniqueIndex:idx_ai_waiter_session_scope;not null" json:"session_id"`
	BusinessID     uint   `gorm:"uniqueIndex:idx_ai_waiter_session_scope;index;not null" json:"business_id"`
	TableCode      string `gorm:"uniqueIndex:idx_ai_waiter_session_scope" json:"table_code"`
	Language       string `json:"language"`
	Mode           string `gorm:"uniqueIndex:idx_ai_waiter_session_scope;default:'ordering'" json:"mode"` // ordering, concierge
	Status         string `gorm:"default:'active'" json:"status"`                                         // active, closed
	IsPaused       bool   `gorm:"default:false" json:"is_paused"`
	CartItemsAdded int    `gorm:"default:0" json:"cart_items_added"`

	// Staff takeover claim (one handler at a time). Nil ClaimedByStaffID = unclaimed.
	ClaimedByStaffID *uint      `gorm:"index" json:"claimed_by_staff_id,omitempty"`
	ClaimedByName    string     `json:"claimed_by_name,omitempty"`
	ClaimedByRole    string     `json:"claimed_by_role,omitempty"`
	ClaimedAt        *time.Time `json:"claimed_at,omitempty"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Relationships. A pointer (not an embedded value) so `omitempty` actually
	// works: a value struct always serialized a full ~200-field zero-value
	// Business on every conversation row even though nothing preloads it. Nil
	// (the default — no code preloads this) is now correctly omitted from the
	// list payload.
	Business *Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// AiWaiterMessage stores individual messages within a conversation
type AiWaiterMessage struct {
	ID                 uint   `gorm:"primaryKey" json:"id"`
	ConversationID     uint   `gorm:"index;index:idx_ai_waiter_messages_conversation_created_at,priority:1;not null" json:"conversation_id"`
	Role               string `gorm:"not null" json:"role"` // "user", "assistant", "system"
	Content            string `gorm:"type:text" json:"content"`
	ToolCalls          string `gorm:"type:text" json:"tool_calls,omitempty"` // JSON string of tools used
	StructuredResponse string `gorm:"type:jsonb;not null;default:'{}'" json:"structured_response"`

	// Authorship for human staff takeover replies (nil for AI/guest/system).
	AuthorStaffID *uint  `json:"author_staff_id,omitempty"`
	AuthorName    string `json:"author_name,omitempty"`
	AuthorRole    string `json:"author_role,omitempty"`

	CreatedAt time.Time `gorm:"index:idx_ai_waiter_messages_conversation_created_at,priority:2" json:"created_at"`
}

// DirectorConsoleThread stores a persistent owner-only AI copilot thread.
type DirectorConsoleThread struct {
	ID            uint       `gorm:"primaryKey" json:"id"`
	BusinessID    uint       `gorm:"index;not null" json:"business_id"`
	Title         string     `gorm:"not null" json:"title"`
	Locale        string     `gorm:"size:8;default:'en'" json:"locale"`
	LastMessageAt time.Time  `gorm:"index" json:"last_message_at"`
	ArchivedAt    *time.Time `gorm:"index" json:"archived_at,omitempty"`
	Pinned        bool       `gorm:"not null;default:false" json:"pinned"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`

	// Relationships
	Business Business `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// TableName method for DirectorConsoleThread model.
func (DirectorConsoleThread) TableName() string {
	return "director_console_threads"
}

// DirectorMessageRole represents the role of a message within the director console.
type DirectorMessageRole string

const (
	DirectorMessageRoleUser      DirectorMessageRole = "user"
	DirectorMessageRoleAssistant DirectorMessageRole = "assistant"
	DirectorMessageRoleSystem    DirectorMessageRole = "system"
)

// DirectorFeedbackVote captures owner feedback used for satisfaction KPI.
type DirectorFeedbackVote string

const (
	DirectorFeedbackPositive DirectorFeedbackVote = "up"
	DirectorFeedbackNegative DirectorFeedbackVote = "down"
)

// DirectorConsoleMessage stores individual chat messages and structured AI output.
type DirectorConsoleMessage struct {
	ID                 uint                  `gorm:"primaryKey" json:"id"`
	ThreadID           uint                  `gorm:"index;index:idx_director_console_messages_thread_history,priority:2;not null" json:"thread_id"`
	BusinessID         uint                  `gorm:"index;index:idx_director_console_messages_thread_history,priority:1;not null" json:"business_id"`
	Role               DirectorMessageRole   `gorm:"type:varchar(16);not null" json:"role"`
	Locale             string                `gorm:"size:8;default:'en'" json:"locale"`
	Content            string                `gorm:"type:text" json:"content"`
	StructuredResponse string                `gorm:"type:text" json:"structured_response,omitempty"`
	ModelName          string                `gorm:"size:64" json:"model_name,omitempty"`
	LatencyMs          int64                 `json:"latency_ms,omitempty"`
	FeedbackVote       *DirectorFeedbackVote `gorm:"type:varchar(8)" json:"feedback_vote,omitempty"`
	FeedbackAt         *time.Time            `json:"feedback_at,omitempty"`
	CreatedAt          time.Time             `gorm:"index:idx_director_console_messages_thread_history,priority:3,sort:desc" json:"created_at"`
	UpdatedAt          time.Time             `json:"updated_at"`

	// Relationships
	Thread   DirectorConsoleThread `gorm:"foreignKey:ThreadID" json:"thread,omitempty"`
	Business Business              `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// TableName method for DirectorConsoleMessage model.
func (DirectorConsoleMessage) TableName() string {
	return "director_console_messages"
}

// LoyaltyProgram represents a per-business loyalty program configuration.
// Earn and redeem rates are independent on purpose: using one field for both
// made 1.25 pts/$ earn + 1.25 pts = $1 redeem into ~100% cashback.
type LoyaltyProgram struct {
	ID                        uint          `gorm:"primaryKey" json:"id"`
	BusinessID                uint          `gorm:"uniqueIndex;not null" json:"business_id"`
	Enabled                   bool          `gorm:"default:true" json:"enabled"`
	PointsPerDollar           float64       `gorm:"default:1" json:"points_per_dollar"`                                                  // earn: points per $1 spent
	RedemptionPointsPerDollar float64       `gorm:"column:redemption_points_per_dollar;default:100" json:"redemption_points_per_dollar"` // redeem: points per $1 discount
	Tiers                     []LoyaltyTier `gorm:"foreignKey:LoyaltyProgramID;constraint:OnDelete:CASCADE" json:"tiers"`
	CreatedAt                 time.Time     `gorm:"not null" json:"created_at"`
	UpdatedAt                 time.Time     `gorm:"not null" json:"updated_at"`
}

// TableName method for LoyaltyProgram model.
func (LoyaltyProgram) TableName() string { return "loyalty_programs" }

// LoyaltyTier represents a tier within a LoyaltyProgram.
// Money is stored in cents (int64) and emitted as dollars (float64) on the wire.
type LoyaltyTier struct {
	ID                    uint   `gorm:"primaryKey" json:"id"`
	LoyaltyProgramID      uint   `gorm:"index;not null" json:"loyalty_program_id"`
	Name                  string `gorm:"not null" json:"name"`
	MinLifetimeSpentCents int64  `gorm:"not null" json:"-"`
	SortOrder             int    `gorm:"not null" json:"sort_order"`
	Color                 string `json:"color"`
}

// TableName method for LoyaltyTier model.
func (LoyaltyTier) TableName() string { return "loyalty_tiers" }

// MarshalJSON emits MinLifetimeSpentCents as dollars (float64) on the wire.
func (t LoyaltyTier) MarshalJSON() ([]byte, error) {
	type alias LoyaltyTier
	return json.Marshal(&struct {
		alias
		MinLifetimeSpent float64 `json:"min_lifetime_spent"`
	}{alias(t), float64(t.MinLifetimeSpentCents) / 100})
}

// UnmarshalJSON parses MinLifetimeSpent (dollars) into MinLifetimeSpentCents (cents).
func (t *LoyaltyTier) UnmarshalJSON(b []byte) error {
	type alias LoyaltyTier
	aux := &struct {
		*alias
		MinLifetimeSpent float64 `json:"min_lifetime_spent"`
	}{alias: (*alias)(t)}
	if err := json.Unmarshal(b, aux); err != nil {
		return err
	}
	// Round, don't truncate: float64 dollars*100 (e.g. 100.57 → 10056.9999)
	// would truncate and corrupt the value on every save/load round-trip.
	t.MinLifetimeSpentCents = int64(math.Round(aux.MinLifetimeSpent * 100))
	return nil
}

// Printer represents a physical thermal printer registered to a business.
type Printer struct {
	ID                  uint       `gorm:"primaryKey" json:"id"`
	BusinessID          uint       `gorm:"index;not null" json:"business_id"`
	LocationID          *uint      `gorm:"index" json:"location_id"`
	Name                string     `gorm:"not null" json:"name"`
	Role                string     `gorm:"not null" json:"role"`      // bill | kitchen | bar (extensible string)
	Transport           string     `gorm:"not null" json:"transport"` // browser | cloudprnt
	PaperWidthMM        int        `gorm:"not null;default:80" json:"paper_width_mm"`
	CodePage            string     `gorm:"not null;default:'CP858'" json:"code_page"`
	CloudPRNTToken      *string    `gorm:"column:cloudprnt_token;uniqueIndex" json:"-"`
	CloudPRNTLastSeenAt *time.Time `gorm:"column:cloudprnt_last_seen_at" json:"cloudprnt_last_seen_at"`
	Enabled             bool       `gorm:"not null;default:true" json:"enabled"`
	FallbackPrinterID   *uint      `json:"fallback_printer_id"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// TableName returns the explicit table name for the Printer model.
func (Printer) TableName() string { return "printers" }

type FiscalMode string

const (
	FiscalModeOff                  FiscalMode = "off"
	FiscalModeManual               FiscalMode = "manual"
	FiscalModeAutomaticNonBlocking FiscalMode = "automatic_non_blocking"
)

func (m FiscalMode) IsValid() bool {
	switch m {
	case FiscalModeOff, FiscalModeManual, FiscalModeAutomaticNonBlocking:
		return true
	default:
		return false
	}
}

type FiscalStatus string

const (
	FiscalStatusPending         FiscalStatus = "pending"
	FiscalStatusAuthorized      FiscalStatus = "authorized"
	FiscalStatusRejected        FiscalStatus = "rejected"
	FiscalStatusFailedRetryable FiscalStatus = "failed_retryable"
	FiscalStatusFailedPermanent FiscalStatus = "failed_permanent"
	FiscalStatusCancelled       FiscalStatus = "cancelled"
	FiscalStatusCredited        FiscalStatus = "credited"
)

func (s FiscalStatus) IsValid() bool {
	switch s {
	case FiscalStatusPending, FiscalStatusAuthorized, FiscalStatusRejected,
		FiscalStatusFailedRetryable, FiscalStatusFailedPermanent,
		FiscalStatusCancelled, FiscalStatusCredited:
		return true
	default:
		return false
	}
}

type BusinessFiscalSettings struct {
	ID                     uint                   `gorm:"primaryKey" json:"id"`
	BusinessID             uint                   `gorm:"index;uniqueIndex:idx_business_fiscal_settings_unique,priority:1;not null" json:"business_id"`
	Country                string                 `gorm:"size:2;uniqueIndex:idx_business_fiscal_settings_unique,priority:2;not null" json:"country"`
	Provider               string                 `gorm:"size:64;uniqueIndex:idx_business_fiscal_settings_unique,priority:3;not null" json:"provider"`
	Mode                   FiscalMode             `gorm:"size:32;not null;default:'off'" json:"mode"`
	Environment            string                 `gorm:"size:16;not null;default:'sandbox'" json:"environment"`
	TaxID                  string                 `gorm:"size:64" json:"tax_id"`
	TaxCondition           string                 `gorm:"size:64" json:"tax_condition"`
	PointOfSale            *int                   `json:"point_of_sale"`
	CredentialsEncrypted   []byte                 `json:"-"`
	CredentialsFingerprint string                 `gorm:"size:128" json:"credentials_fingerprint"`
	CredentialsExpiresAt   *time.Time             `json:"credentials_expires_at"`
	ProviderConfig         map[string]interface{} `gorm:"serializer:json;type:jsonb" json:"provider_config,omitempty"`
	SetupStatus            string                 `gorm:"size:32;not null;default:'draft'" json:"setup_status"`
	LastValidatedAt        *time.Time             `json:"last_validated_at"`
	LastValidationError    *string                `json:"last_validation_error"`
	CreatedAt              time.Time              `json:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at"`
}

func (BusinessFiscalSettings) TableName() string { return "business_fiscal_settings" }

type FiscalReceipt struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	BusinessID           uint       `gorm:"index;not null" json:"business_id"`
	SettingsID           uint       `gorm:"index;not null" json:"settings_id"`
	BillID               uint       `gorm:"index;not null" json:"bill_id"`
	PaymentID            *uint      `gorm:"index" json:"payment_id"`
	AlternativePaymentID *uint      `gorm:"index" json:"alternative_payment_id"`
	Country              string     `gorm:"size:2;not null" json:"country"`
	Provider             string     `gorm:"size:64;not null" json:"provider"`
	Action               string     `gorm:"size:32;not null" json:"action"`
	ReceiptType          string     `gorm:"size:32;not null" json:"receipt_type"`
	ReceiptNumber        *string    `gorm:"size:64" json:"receipt_number"`
	ProviderReceiptID    *string    `gorm:"size:128" json:"provider_receipt_id"`
	AuthCode             *string    `gorm:"size:128" json:"auth_code"`
	AuthExpiresAt        *time.Time `json:"auth_expires_at"`
	QRPayload            *string    `json:"qr_payload"`
	QRImagePath          *string    `gorm:"size:256" json:"qr_image_path"`
	PDFPath              *string    `gorm:"size:256" json:"pdf_path"`
	CustomerDocType      *string    `gorm:"size:32" json:"customer_doc_type"`
	CustomerDocNumber    *string    `gorm:"size:64" json:"customer_doc_number"`
	// CustomerName / CustomerTaxCondition are audit copies of the receptor
	// identity at issuance. Tax condition also drives RG 5616
	// CondicionIVAReceptorId on credit notes that re-read the original receipt.
	CustomerName         *string      `gorm:"type:text" json:"customer_name,omitempty"`
	CustomerTaxCondition *string      `gorm:"size:32" json:"customer_tax_condition,omitempty"`
	TotalAmountCents     int64        `gorm:"not null;default:0" json:"total_amount_cents"`
	TipAmountCents       int64        `gorm:"not null;default:0" json:"tip_amount_cents"`
	Currency             string       `gorm:"size:8;not null;default:'USD'" json:"currency"`
	Status               FiscalStatus `gorm:"size:32;not null;default:'pending'" json:"status"`
	ErrorCode            *string      `gorm:"size:64" json:"error_code"`
	ErrorMessage         *string      `json:"error_message"`
	// RawRequest/RawResponse persist the raw provider SOAP payloads for debugging.
	// They are json:"-" so they never leak to a client through ListReceipts — the
	// request can carry the WSAA token and other sensitive provider detail
	// (F-RAWCOLS). The gorm jsonb directive is retained so AutoMigrate leaves the
	// existing jsonb columns alone.
	RawRequest  map[string]interface{} `gorm:"serializer:json;type:jsonb" json:"-"`
	RawResponse map[string]interface{} `gorm:"serializer:json;type:jsonb" json:"-"`
	IssuedAt    *time.Time             `json:"issued_at"`
	// DeliveredAt records when the authorized receipt was delivered to the
	// customer (PDF emailed / queued for print). Nil means not-yet-delivered, so a
	// later sweep can retry; once set it is the idempotency guard against
	// double-sending. Delivery is best-effort and never gates the fiscal job.
	DeliveredAt *time.Time `json:"delivered_at"`
	// DeliveryLockedAt / DeliveryLockedBy implement a per-row sweep claim so two
	// concurrent fiscal workers cannot both deliver the same receipt. The sweep
	// claims a receipt with an atomic UPDATE … WHERE delivered_at IS NULL AND
	// (delivery_locked_at IS NULL OR delivery_locked_at < staleBefore). A worker
	// that crashes mid-delivery leaves the lock to expire after the TTL (5 min),
	// after which a later sweep reclaims it. A successfully-delivered receipt has
	// delivered_at set, which makes the lock moot (delivery is idempotent on that
	// guard).
	DeliveryLockedAt *time.Time `json:"delivery_locked_at,omitempty"`
	DeliveryLockedBy string     `json:"delivery_locked_by,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

func (FiscalReceipt) TableName() string { return "fiscal_receipts" }

type FiscalJob struct {
	ID         uint  `gorm:"primaryKey" json:"id"`
	BusinessID uint  `gorm:"index;not null" json:"business_id"`
	SettingsID uint  `gorm:"index;not null" json:"settings_id"`
	ReceiptID  *uint `gorm:"index" json:"receipt_id"`
	// ProducedReceiptID is the receipt this job produced; receipt_id stays the input receipt for credit notes.
	ProducedReceiptID    *uint  `gorm:"index" json:"produced_receipt_id"`
	BillID               uint   `gorm:"index;not null" json:"bill_id"`
	PaymentID            *uint  `gorm:"index" json:"payment_id"`
	AlternativePaymentID *uint  `gorm:"index" json:"alternative_payment_id"`
	Action               string `gorm:"size:32;not null" json:"action"`
	IdempotencyKey       string `gorm:"size:160;uniqueIndex:idx_fiscal_jobs_idempotency;not null" json:"idempotency_key"`
	// CreditAmountCents is the partial credit-note amount (net of tip, int64 cents)
	// for a credit_note job. Nil credits the full original receipt total. Only
	// meaningful when Action == credit_note.
	CreditAmountCents *int64 `json:"credit_amount_cents"`
	// AttemptedProviderReceiptID is the encoded "<type>-<pos>-<number>" tuple last
	// sent (or about to be sent) to the fiscal provider for this job. A retry
	// reconciles this number before allocating LastAuthorized+1, so a lost
	// authorization response cannot mint a second legal voucher. Nil if this job
	// has not yet attempted a numbered request.
	AttemptedProviderReceiptID *string      `gorm:"size:64" json:"attempted_provider_receipt_id,omitempty"`
	Status                     FiscalStatus `gorm:"size:32;not null;default:'pending'" json:"status"`
	Attempts                   int          `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts                int          `gorm:"not null;default:5" json:"max_attempts"`
	NextAttemptAt              *time.Time   `gorm:"index" json:"next_attempt_at"`
	LastErrorCode              *string      `gorm:"size:64" json:"last_error_code"`
	LastErrorMessage           *string      `json:"last_error_message"`
	LockedAt                   *time.Time   `json:"locked_at"`
	LockedBy                   *string      `gorm:"size:64" json:"locked_by"`
	CreatedBy                  string       `gorm:"size:64;not null;default:'system'" json:"created_by"`
	CreatedAt                  time.Time    `json:"created_at"`
	UpdatedAt                  time.Time    `json:"updated_at"`
}

func (FiscalJob) TableName() string { return "fiscal_jobs" }

type FiscalAuditEvent struct {
	ID         uint                   `gorm:"primaryKey" json:"id"`
	BusinessID uint                   `gorm:"index;not null" json:"business_id"`
	ReceiptID  *uint                  `gorm:"index" json:"receipt_id"`
	JobID      *uint                  `gorm:"index" json:"job_id"`
	Actor      string                 `gorm:"size:64;not null;default:'system'" json:"actor"`
	EventType  string                 `gorm:"size:64;not null" json:"event_type"`
	Message    string                 `gorm:"not null" json:"message"`
	Metadata   map[string]interface{} `gorm:"serializer:json;type:jsonb" json:"metadata,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
}

func (FiscalAuditEvent) TableName() string { return "fiscal_audit_events" }

// Fiscal delivery channel and status constants for durable per-channel delivery
// tasks (Wave 4). Status machine: pending → leased → succeeded | pending (retry)
// | dead. Operator requeue moves dead → pending.
const (
	FiscalDeliveryChannelArtifact = "artifact"
	FiscalDeliveryChannelEmail    = "email"
	FiscalDeliveryChannelPrint    = "print"

	FiscalDeliveryStatusPending   = "pending"
	FiscalDeliveryStatusLeased    = "leased"
	FiscalDeliveryStatusSucceeded = "succeeded"
	FiscalDeliveryStatusDead      = "dead"

	// DefaultFiscalDeliveryMaxAttempts is the per-task retry ceiling before dead-letter.
	DefaultFiscalDeliveryMaxAttempts = 8
	// FiscalDeliveryLastErrorMaxLen bounds last_error text stored on the task row.
	FiscalDeliveryLastErrorMaxLen = 1000
)

// FiscalDeliveryTask is one durable delivery intent for an authorized fiscal
// receipt on a single channel (artifact upload, email, or print enqueue).
// Authorization creates these rows in the same transaction as the receipt;
// a leased worker executes them with retries. UNIQUE(receipt_id, channel).
type FiscalDeliveryTask struct {
	ID                uint       `gorm:"primaryKey" json:"id"`
	BusinessID        uint       `gorm:"index;not null" json:"business_id"`
	ReceiptID         uint       `gorm:"index;not null;uniqueIndex:idx_fiscal_delivery_tasks_receipt_channel" json:"receipt_id"`
	Channel           string     `gorm:"size:16;not null;uniqueIndex:idx_fiscal_delivery_tasks_receipt_channel" json:"channel"`
	Status            string     `gorm:"size:16;not null;default:'pending'" json:"status"`
	Attempts          int        `gorm:"not null;default:0" json:"attempts"`
	MaxAttempts       int        `gorm:"not null;default:8" json:"max_attempts"`
	NextAttemptAt     *time.Time `gorm:"index" json:"next_attempt_at"`
	LeaseOwner        string     `gorm:"size:64" json:"lease_owner,omitempty"`
	LeaseExpiresAt    *time.Time `json:"lease_expires_at,omitempty"`
	LastError         string     `json:"last_error,omitempty"`
	IdempotencyKey    string     `gorm:"size:200;not null" json:"idempotency_key"`
	ProviderMessageID *string    `gorm:"size:128" json:"provider_message_id,omitempty"`
	// Locale is the email/print locale resolved at enqueue (safe non-PII).
	// Recipient email is re-resolved at execution from bill/CRM, never stored.
	Locale      *string    `gorm:"size:16" json:"locale,omitempty"`
	SucceededAt *time.Time `json:"succeeded_at,omitempty"`
	DeadAt      *time.Time `json:"dead_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

func (FiscalDeliveryTask) TableName() string { return "fiscal_delivery_tasks" }

// PrintJobStatus represents the lifecycle status of a PrintJob.
type PrintJobStatus string

const (
	PrintJobStatusPending         PrintJobStatus = "pending"
	PrintJobStatusRouted          PrintJobStatus = "routed"
	PrintJobStatusPrinting        PrintJobStatus = "printing"
	PrintJobStatusPrinted         PrintJobStatus = "printed"
	PrintJobStatusFailed          PrintJobStatus = "failed" // legacy terminal failure
	PrintJobStatusFailedRetryable PrintJobStatus = "failed_retryable"
	PrintJobStatusFailedPermanent PrintJobStatus = "failed_permanent"
	PrintJobStatusCancelled       PrintJobStatus = "cancelled"
)

// IsValid returns true if the status is one of the defined enum values.
func (s PrintJobStatus) IsValid() bool {
	switch s {
	case PrintJobStatusPending, PrintJobStatusRouted, PrintJobStatusPrinting,
		PrintJobStatusPrinted, PrintJobStatusFailed, PrintJobStatusFailedRetryable,
		PrintJobStatusFailedPermanent, PrintJobStatusCancelled:
		return true
	}
	return false
}

// PrintJobKind names what is being printed.
type PrintJobKind string

const (
	PrintJobKindBill    PrintJobKind = "bill"
	PrintJobKindReceipt PrintJobKind = "receipt"
	PrintJobKindKitchen PrintJobKind = "kitchen" // Sprint 2
	PrintJobKindBar     PrintJobKind = "bar"     // Sprint 2
	PrintJobKindVoid    PrintJobKind = "void"    // Sprint 2
	PrintJobKindModify  PrintJobKind = "modify"  // Sprint 2
)

// IsValid returns true for any defined kind. Kitchen/bar/void/modify exist
// for forward-compat with Sprint 2; the queue rejects them in Sprint 1.
func (k PrintJobKind) IsValid() bool {
	switch k {
	case PrintJobKindBill, PrintJobKindReceipt,
		PrintJobKindKitchen, PrintJobKindBar, PrintJobKindVoid, PrintJobKindModify:
		return true
	}
	return false
}

// PrintJob represents one queued or completed print job.
//
// IMP-02 added retry scheduling fields: NextAttemptAt, AttemptCount, MaxAttempts,
// plus IMP-04 added OrderID and KitchenAckedAt for kitchen-ticket durability.
//
// Wave 4 browser leases: ClaimedBy + LeaseExpiresAt let one elected
// browser agent own a job while presenting it. PresentedAt records that
// window.print() / the dialog was shown; PrintedAt still requires explicit
// operator confirmation (distinct from presentation).
type PrintJob struct {
	ID            uint           `gorm:"primaryKey" json:"id"`
	BusinessID    uint           `gorm:"index;not null" json:"business_id"`
	LocationID    *uint          `gorm:"index" json:"location_id"`
	PrinterID     *uint          `gorm:"index" json:"printer_id"`
	OrderID       *uint          `gorm:"index" json:"order_id,omitempty"`
	Kind          PrintJobKind   `gorm:"not null" json:"kind"`
	SourceType    string         `gorm:"not null" json:"source_type"`
	SourceID      uint           `gorm:"not null" json:"source_id"`
	Status        PrintJobStatus `gorm:"not null;default:'pending'" json:"status"`
	PayloadHTML   *string        `json:"payload_html,omitempty"`
	PayloadESCPOS []byte         `json:"-"`
	Retries       int            `gorm:"not null;default:0" json:"retries"`
	AttemptCount  int            `gorm:"not null;default:0" json:"attempt_count"`
	MaxAttempts   int            `gorm:"not null;default:6" json:"max_attempts"`
	NextAttemptAt *time.Time     `json:"next_attempt_at,omitempty"`
	LastError     *string        `json:"last_error,omitempty"`
	Language      string         `gorm:"not null;default:'en'" json:"language"`
	CreatedBy     string         `gorm:"not null;default:'system'" json:"created_by"`
	// Wave 4 browser lease fields.
	ClaimedBy      *string    `gorm:"size:64" json:"claimed_by,omitempty"`
	LeaseExpiresAt *time.Time `json:"lease_expires_at,omitempty"`
	PresentedAt    *time.Time `json:"presented_at,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	PrintedAt      *time.Time `json:"printed_at,omitempty"`
	KitchenAckedAt *time.Time `json:"kitchen_acked_at,omitempty"`
}

// TableName returns the explicit table name for the PrintJob model.
func (PrintJob) TableName() string { return "print_jobs" }

// PrintAuditLog records reprints, manual cancellations, token rotations.
type PrintAuditLog struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	BusinessID uint           `gorm:"index;not null" json:"business_id"`
	Actor      string         `gorm:"not null" json:"actor"`
	Action     string         `gorm:"not null" json:"action"`
	PrintJobID *uint          `json:"print_job_id"`
	PrinterID  *uint          `json:"printer_id"`
	Metadata   JSONRawMessage `gorm:"type:jsonb" json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// TableName returns the explicit table name for the PrintAuditLog model.
func (PrintAuditLog) TableName() string { return "print_audit_log" }

// Push principal types for explicit membership-scoped fanout.
const (
	PushPrincipalOwnerUser = "owner_user"
	PushPrincipalStaff     = "staff"
)

// PushSubscription stores Web Push API subscription credentials for a principal+business pair.
// PrincipalType/PrincipalID identify who should receive fanout (owner user or active staff);
// UserID is retained for legacy owner rows and list/delete scoping.
type PushSubscription struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	UserID        uint      `gorm:"index;not null" json:"user_id"`
	BusinessID    uint      `gorm:"index;not null;index:idx_push_subscriptions_principal_fanout,priority:1" json:"business_id"`
	PrincipalType string    `gorm:"size:16;not null;default:owner_user;index:idx_push_subscriptions_principal_fanout,priority:2" json:"principal_type"`
	PrincipalID   uint      `gorm:"not null;index:idx_push_subscriptions_principal_fanout,priority:3" json:"principal_id"`
	Endpoint      string    `gorm:"type:text;not null" json:"endpoint"`
	P256dhKey     string    `gorm:"type:text;not null" json:"p256dh_key"`
	AuthKey       string    `gorm:"type:text;not null" json:"auth_key"`
	UserAgent     string    `gorm:"type:text" json:"user_agent"`
	CreatedAt     time.Time `json:"created_at"`
	LastUsedAt    time.Time `json:"last_used_at"`
}

// InventoryAlertLog records when a low-stock alert was sent for a given item,
// used to debounce repeated alerts within a cooldown window.
type InventoryAlertLog struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	BusinessID      uint      `gorm:"index;not null" json:"business_id"`
	InventoryItemID uint      `gorm:"index;not null" json:"inventory_item_id"`
	AlertType       string    `gorm:"not null" json:"alert_type"`
	AlertedAt       time.Time `gorm:"not null" json:"alerted_at"`
}

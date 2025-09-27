package database

import (
	"time"
	"payverge/internal/structs"
)

// User represents the users table
type User struct {
	ID                      uint                             `gorm:"primaryKey" json:"id"`
	Address                 string                           `gorm:"uniqueIndex;not null" json:"address"`
	ReferralCode           string                           `gorm:"uniqueIndex" json:"referral_code"`
	TokenID                *string                          `json:"token_id"`
	NotificationPreferences structs.NotificationPreferences `gorm:"embedded" json:"notification_preferences"`
	CreatedAt              time.Time                        `json:"created_at"`
	UpdatedAt              time.Time                        `json:"updated_at"`
}

// Code represents the codes table
type Code struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Code      string    `gorm:"uniqueIndex;not null" json:"code"`
	Address   string    `json:"address"`
	Amount    string    `json:"amount"`
	Used      bool      `gorm:"default:false" json:"used"`
	Expiry    string    `json:"expiry"`
	ClaimedAt time.Time `json:"claimed_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ErrorLog represents the error_logs table
type ErrorLog struct {
	ID             uint                   `gorm:"primaryKey" json:"id"`
	Timestamp      time.Time              `gorm:"index" json:"timestamp"`
	Error          string                 `json:"error"`
	Component      string                 `json:"component"`
	Function       string                 `json:"function"`
	AdditionalInfo map[string]interface{} `gorm:"serializer:json" json:"additional_info,omitempty"`
	CreatedAt      time.Time              `json:"created_at"`
}

// FaucetTransaction represents the faucet_transactions table
type FaucetTransaction struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Address   string    `gorm:"index" json:"address"`
	Amount    string    `json:"amount"`
	TxHash    string    `gorm:"uniqueIndex" json:"tx_hash"`
	Timestamp time.Time `gorm:"index" json:"timestamp"`
	Ticker    string    `json:"ticker"`
	CreatedAt time.Time `json:"created_at"`
}

// Subscriber represents the subscribers table
type Subscriber struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Email     string    `gorm:"uniqueIndex;not null" json:"email"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// MultisigTx represents the multisig_transactions table
type MultisigTx struct {
	ID        uint                   `gorm:"primaryKey" json:"id"`
	TxID      int                    `gorm:"uniqueIndex;not null" json:"tx_id"`
	Data      map[string]interface{} `gorm:"serializer:json" json:"data"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}

// Payverge-specific models

// Business represents a restaurant/venue in the system
type Business struct {
	ID              uint            `gorm:"primaryKey" json:"id"`
	OwnerAddress    string          `gorm:"index;not null" json:"owner_address"`
	Name            string          `gorm:"not null" json:"name"`
	Logo            string          `json:"logo"`
	Address         BusinessAddress `gorm:"embedded" json:"address"`
	SettlementAddr  string          `gorm:"not null" json:"settlement_address"`
	TippingAddr     string          `gorm:"not null" json:"tipping_address"`
	TaxRate         float64         `gorm:"default:0" json:"tax_rate"`
	ServiceFeeRate  float64         `gorm:"default:0" json:"service_fee_rate"`
	TaxInclusive    bool            `gorm:"default:false" json:"tax_inclusive"`
	ServiceInclusive bool           `gorm:"default:false" json:"service_inclusive"`
	IsActive        bool            `gorm:"default:true" json:"is_active"`
	// New fields for enhanced business features
	Description     string          `json:"description"`
	CustomURL       string          `json:"custom_url"`
	Phone           string          `json:"phone"`
	Website         string          `json:"website"`
	SocialMedia     string          `json:"social_media"` // JSON string for social media links
	BannerImages    string          `json:"banner_images"` // JSON array of banner image URLs
	BusinessPageEnabled bool        `gorm:"default:false" json:"business_page_enabled"`
	ShowReviews     bool            `gorm:"default:true" json:"show_reviews"`
	GoogleReviewsEnabled bool       `gorm:"default:false" json:"google_reviews_enabled"`
	ReferredByCode  string          `json:"referred_by_code"` // Referral code used during registration
	CreatedAt       time.Time       `json:"created_at"`
	UpdatedAt       time.Time       `json:"updated_at"`
}

// BusinessAddress represents the physical address of a business
type BusinessAddress struct {
	Street     string `json:"street"`
	City       string `json:"city"`
	State      string `json:"state"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
}

// Menu represents a business's menu
type Menu struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	BusinessID  uint           `gorm:"index;not null" json:"business_id"`
	Categories  string         `gorm:"type:text" json:"categories"` // JSON string for SQLite
	IsActive    bool           `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
	Business    Business       `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
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
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Price       float64          `json:"price"`
	Currency    string           `json:"currency"`
	Image       string           `json:"image"`       // Keep for backward compatibility
	Images      []string         `json:"images"`      // New field for multiple images
	Options     []MenuItemOption `json:"options"`
	Allergens   []string         `json:"allergens"`
	DietaryTags []string         `json:"dietary_tags"`
	IsAvailable bool             `json:"is_available"`
	SortOrder   int              `json:"sort_order"`
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
	ID          uint      `gorm:"primaryKey" json:"id"`
	BusinessID  uint      `gorm:"index;not null" json:"business_id"`
	TableCode   string    `gorm:"uniqueIndex;not null" json:"table_code"`
	Name        string    `gorm:"not null" json:"name"`
	QRCode      string    `json:"qr_code"`
	IsActive    bool      `gorm:"default:true" json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Business    Business  `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// Bill represents a bill/check for a table
type Bill struct {
	ID               uint          `gorm:"primaryKey" json:"id"`
	BusinessID       uint          `gorm:"index;not null" json:"business_id"`
	TableID          uint          `gorm:"index" json:"table_id"`
	BillNumber       string        `gorm:"uniqueIndex;not null" json:"bill_number"`
	Items            string        `gorm:"type:text" json:"items"` // JSON string for SQLite
	Subtotal         float64       `json:"subtotal"`
	TaxAmount        float64       `json:"tax_amount"`
	ServiceFeeAmount float64       `json:"service_fee_amount"`
	TotalAmount      float64       `json:"total_amount"`
	PaidAmount       float64       `gorm:"default:0" json:"paid_amount"`
	TipAmount        float64       `gorm:"default:0" json:"tip_amount"`
	Status           BillStatus    `gorm:"default:'open'" json:"status"`
	SettlementAddr   string        `gorm:"not null" json:"settlement_address"`
	TippingAddr      string        `gorm:"not null" json:"tipping_address"`
	CreatedAt        time.Time     `json:"created_at"`
	UpdatedAt        time.Time     `json:"updated_at"`
	ClosedAt         *time.Time    `json:"closed_at"`
	Business         Business      `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
	Table            Table         `gorm:"foreignKey:TableID" json:"table,omitempty"`
	Payments         []Payment     `gorm:"foreignKey:BillID" json:"payments,omitempty"`
}

// BillItem represents an item on a bill
type BillItem struct {
	ID         string           `json:"id"`
	MenuItemID string           `json:"menu_item_id"`
	Name       string           `json:"name"`
	Price      float64          `json:"price"`
	Quantity   int              `json:"quantity"`
	Options    []MenuItemOption `json:"options"`
	Subtotal   float64          `json:"subtotal"`
}

// BillStatus represents the status of a bill
type BillStatus string

const (
	BillStatusOpen   BillStatus = "open"
	BillStatusPaid   BillStatus = "paid"
	BillStatusClosed BillStatus = "closed"
)

// Payment represents a payment made towards a bill
type Payment struct {
	ID        uint          `gorm:"primaryKey" json:"id"`
	BillID    uint          `gorm:"index;not null" json:"bill_id"`
	PayerAddr string        `gorm:"not null" json:"payer_address"`
	Amount    float64       `gorm:"not null" json:"amount"`
	TipAmount float64       `gorm:"default:0" json:"tip_amount"`
	TxHash    string        `gorm:"uniqueIndex" json:"tx_hash"`
	Status    PaymentStatus `gorm:"default:'pending'" json:"status"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
	Bill      Bill          `gorm:"foreignKey:BillID" json:"bill,omitempty"`
}

// PaymentStatus represents the status of a payment
type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusConfirmed PaymentStatus = "confirmed"
	PaymentStatusFailed    PaymentStatus = "failed"
)

// AlternativePayment represents a non-crypto payment (cash, card, etc.)
type AlternativePayment struct {
	ID              uint                     `gorm:"primaryKey" json:"id"`
	BillID          uint                     `gorm:"index;not null" json:"bill_id"`
	ParticipantAddr string                   `gorm:"not null" json:"participant_address"`
	ParticipantName string                   `json:"participant_name"` // Optional name for identification
	Amount          float64                  `gorm:"not null" json:"amount"`
	PaymentMethod   AlternativePaymentMethod `gorm:"not null" json:"payment_method"`
	Status          AlternativePaymentStatus `gorm:"default:'pending'" json:"status"`
	ConfirmedBy     string                   `json:"confirmed_by"` // Business owner who confirmed
	CreatedAt       time.Time                `json:"created_at"`
	UpdatedAt       time.Time                `json:"updated_at"`
	ConfirmedAt     *time.Time               `json:"confirmed_at"`
	Bill            Bill                     `gorm:"foreignKey:BillID" json:"bill,omitempty"`
}

// AlternativePaymentMethod represents the method used for alternative payment
type AlternativePaymentMethod string

const (
	PaymentMethodCash  AlternativePaymentMethod = "cash"
	PaymentMethodCard  AlternativePaymentMethod = "card"
	PaymentMethodVenmo AlternativePaymentMethod = "venmo"
	PaymentMethodOther AlternativePaymentMethod = "other"
)

// AlternativePaymentStatus represents the status of an alternative payment
type AlternativePaymentStatus string

const (
	AltPaymentStatusPending   AlternativePaymentStatus = "pending"
	AltPaymentStatusConfirmed AlternativePaymentStatus = "confirmed"
	AltPaymentStatusFailed    AlternativePaymentStatus = "failed"
)

// PaymentBreakdown represents the breakdown of crypto vs alternative payments
type PaymentBreakdown struct {
	TotalAmount     float64 `json:"total_amount"`
	CryptoPaid      float64 `json:"crypto_paid"`
	AlternativePaid float64 `json:"alternative_paid"`
	Remaining       float64 `json:"remaining"`
	IsComplete      bool    `json:"is_complete"`
}

// Staff represents employees/workers of a business
type Staff struct {
	ID           uint       `gorm:"primaryKey" json:"id"`
	BusinessID   uint       `gorm:"index;not null" json:"business_id"`
	Email        string     `gorm:"uniqueIndex;not null" json:"email"`
	Name         string     `gorm:"not null" json:"name"`
	Role         StaffRole  `gorm:"not null" json:"role"`
	IsActive     bool       `gorm:"default:true" json:"is_active"`
	LastLoginAt  *time.Time `json:"last_login_at"`
	InvitedBy    string     `gorm:"not null" json:"invited_by"` // Owner wallet address
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	Business     Business   `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// StaffInvitation represents pending staff invitations
type StaffInvitation struct {
	ID         uint             `gorm:"primaryKey" json:"id"`
	BusinessID uint             `gorm:"index;not null" json:"business_id"`
	Email      string           `gorm:"not null" json:"email"`
	Name       string           `gorm:"not null" json:"name"`
	Role       StaffRole        `gorm:"not null" json:"role"`
	Token      string           `gorm:"uniqueIndex;not null" json:"token"`
	Status     InvitationStatus `gorm:"default:'pending'" json:"status"`
	InvitedBy  string           `gorm:"not null" json:"invited_by"` // Owner wallet address
	ExpiresAt  time.Time        `json:"expires_at"`
	CreatedAt  time.Time        `json:"created_at"`
	UpdatedAt  time.Time        `json:"updated_at"`
	Business   Business         `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// StaffLoginCode represents temporary login codes for staff
type StaffLoginCode struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	StaffID   uint      `gorm:"index;not null" json:"staff_id"`
	Code      string    `gorm:"uniqueIndex;not null" json:"code"`
	ExpiresAt time.Time `json:"expires_at"`
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

func (Code) TableName() string {
	return "codes"
}

func (ErrorLog) TableName() string {
	return "error_logs"
}

func (FaucetTransaction) TableName() string {
	return "faucet_transactions"
}

func (Subscriber) TableName() string {
	return "subscribers"
}

func (MultisigTx) TableName() string {
	return "multisig_transactions"
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

func (Staff) TableName() string {
	return "staff"
}

func (StaffInvitation) TableName() string {
	return "staff_invitations"
}

func (StaffLoginCode) TableName() string {
	return "staff_login_codes"
}

// Referral system models

// ReferralTier represents the tier of a referrer (Basic or Premium)
type ReferralTier string

const (
	ReferralTierBasic   ReferralTier = "basic"
	ReferralTierPremium ReferralTier = "premium"
)

// ReferralStatus represents the status of a referrer
type ReferralStatus string

const (
	ReferralStatusActive   ReferralStatus = "active"
	ReferralStatusInactive ReferralStatus = "inactive"
	ReferralStatusSuspended ReferralStatus = "suspended"
)

// Referrer represents a user who can refer businesses
type Referrer struct {
	ID                  uint           `gorm:"primaryKey" json:"id"`
	WalletAddress       string         `gorm:"uniqueIndex;not null" json:"wallet_address"`
	ReferralCode        string         `gorm:"uniqueIndex;not null" json:"referral_code"`
	Tier                ReferralTier   `gorm:"not null" json:"tier"`
	Status              ReferralStatus `gorm:"default:'active'" json:"status"`
	TotalReferrals      int            `gorm:"default:0" json:"total_referrals"`
	TotalCommissions    string         `gorm:"default:'0'" json:"total_commissions"` // USDC amount as string
	ClaimableCommissions string        `gorm:"default:'0'" json:"claimable_commissions"` // USDC amount as string
	LastClaimedAt       *time.Time     `json:"last_claimed_at"`
	RegistrationTxHash  string         `json:"registration_tx_hash"` // Blockchain transaction hash
	CreatedAt           time.Time      `json:"created_at"`
	UpdatedAt           time.Time      `json:"updated_at"`
}

// ReferralRecord represents a successful referral of a business
type ReferralRecord struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	ReferrerID        uint      `gorm:"index;not null" json:"referrer_id"`
	BusinessID        uint      `gorm:"index;not null" json:"business_id"`
	RegistrationFee   string    `json:"registration_fee"`   // USDC amount as string
	Discount          string    `json:"discount"`           // USDC discount given to business
	Commission        string    `json:"commission"`         // USDC commission earned by referrer
	CommissionPaid    bool      `gorm:"default:false" json:"commission_paid"`
	CommissionTxHash  string    `json:"commission_tx_hash"` // Blockchain transaction hash for commission payment
	ProcessedTxHash   string    `json:"processed_tx_hash"`  // Blockchain transaction hash for referral processing
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Referrer          Referrer  `gorm:"foreignKey:ReferrerID" json:"referrer,omitempty"`
	Business          Business  `gorm:"foreignKey:BusinessID" json:"business,omitempty"`
}

// ReferralCommissionClaim represents a commission claim by a referrer
type ReferralCommissionClaim struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ReferrerID    uint      `gorm:"index;not null" json:"referrer_id"`
	Amount        string    `json:"amount"`         // USDC amount as string
	TxHash        string    `json:"tx_hash"`        // Blockchain transaction hash
	Status        string    `gorm:"default:'pending'" json:"status"` // pending, completed, failed
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Referrer      Referrer  `gorm:"foreignKey:ReferrerID" json:"referrer,omitempty"`
}

// TableName methods for referral models
func (Referrer) TableName() string {
	return "referrers"
}

func (ReferralRecord) TableName() string {
	return "referral_records"
}

func (ReferralCommissionClaim) TableName() string {
	return "referral_commission_claims"
}

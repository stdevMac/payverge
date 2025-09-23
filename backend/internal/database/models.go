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

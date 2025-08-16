package models

import (
	"time"

	"gorm.io/gorm"
)

type Invoice struct {
	ID           uint      `json:"id" gorm:"primaryKey"`
	InvoiceID    uint64    `json:"invoice_id" gorm:"uniqueIndex;not null"` // Smart contract invoice ID
	Creator      string    `json:"creator" gorm:"not null"`                // Creator wallet address
	CreatorName  string    `json:"creator_name"`                           // Optional creator name
	Title        string    `json:"title" gorm:"not null"`
	Description  string    `json:"description"`
	Amount       uint64    `json:"amount" gorm:"not null"`         // Amount in USDC (6 decimals)
	AmountPaid   uint64    `json:"amount_paid" gorm:"default:0"`   // Amount paid so far
	PayerEmail   string    `json:"payer_email"`                    // Optional payer email
	PayerName    string    `json:"payer_name"`                     // Optional payer name
	DueDate      *time.Time `json:"due_date"`                      // Optional due date
	Status       string    `json:"status" gorm:"default:pending"` // pending, partially_paid, paid, cancelled
	MetadataURI  string    `json:"metadata_uri"`                   // IPFS or backend URI
	PaymentLink  string    `json:"payment_link"`                   // Short payment link
	QRCodeURL    string    `json:"qr_code_url"`                    // QR code image URL
	TxHash       string    `json:"tx_hash"`                        // Creation transaction hash
	IsActive     bool      `json:"is_active" gorm:"default:true"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"-" gorm:"index"`
}

type Payment struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	InvoiceID uint64    `json:"invoice_id" gorm:"not null"` // Smart contract invoice ID
	Payer     string    `json:"payer" gorm:"not null"`      // Payer wallet address
	Amount    uint64    `json:"amount" gorm:"not null"`     // Payment amount
	Fee       uint64    `json:"fee" gorm:"not null"`        // Platform fee taken
	TxHash    string    `json:"tx_hash" gorm:"not null"`    // Payment transaction hash
	BlockNumber uint64  `json:"block_number"`               // Block number of payment
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type EmailLog struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	InvoiceID uint64    `json:"invoice_id" gorm:"not null"`
	EmailType string    `json:"email_type" gorm:"not null"` // initial, reminder, confirmation
	Recipient string    `json:"recipient" gorm:"not null"`
	Subject   string    `json:"subject"`
	Status    string    `json:"status" gorm:"default:pending"` // pending, sent, failed
	Error     string    `json:"error"`
	SentAt    *time.Time `json:"sent_at"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName overrides
func (Invoice) TableName() string {
	return "invoices"
}

func (Payment) TableName() string {
	return "payments"
}

func (EmailLog) TableName() string {
	return "email_logs"
}

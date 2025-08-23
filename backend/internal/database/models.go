package database

import (
	"time"
	"web3-boilerplate/internal/structs"
)

// User represents the users table
type User struct {
	ID                      uint                             `gorm:"primaryKey" json:"id"`
	Address                 string                           `gorm:"uniqueIndex;not null" json:"address"`
	ReferralCode           string                           `gorm:"uniqueIndex" json:"referral_code"`
	TokenID                string                           `gorm:"uniqueIndex" json:"token_id"`
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

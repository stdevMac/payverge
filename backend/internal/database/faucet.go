package database

import (
	"errors"
	"time"

	"gorm.io/gorm"
)

const (
	topUpCooldownDays = 3 // Days between allowed top-ups
)

// SaveFaucetTransaction records a new faucet transaction in the database
func SaveFaucetTransaction(address, amount, txHash, ticker string) error {
	transaction := FaucetTransaction{
		Address:   address,
		Amount:    amount,
		TxHash:    txHash,
		Timestamp: time.Now(),
		Ticker:    ticker,
	}

	result := db.Create(&transaction)
	return result.Error
}

// CanReceiveTopUp checks if an address is eligible to receive a top-up
// based on the cooldown period
func CanReceiveTopUp(address string) (bool, error) {
	var lastTx FaucetTransaction
	result := db.Where("address = ?", address).Order("timestamp desc").First(&lastTx)

	// If no record found, they've never received a top-up before
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return true, nil
	}

	if result.Error != nil {
		return false, result.Error
	}

	// Check if enough time has passed since the last top-up
	cooldownPeriod := time.Hour * 24 * topUpCooldownDays
	return time.Since(lastTx.Timestamp) > cooldownPeriod, nil
}

package database

import (
	"errors"
)

// GetMultisigTx retrieves the stored multisig transaction data
func GetMultisigTx() (map[string]interface{}, error) {
	var tx MultisigTx
	result := db.Where("tx_id = ?", 1).First(&tx)
	if result.Error != nil {
		return nil, result.Error
	}
	return tx.Data, nil
}

// StoreMultisigTx stores new multisig transaction data
func StoreMultisigTx(data map[string]interface{}) error {
	if data == nil {
		return errors.New("data cannot be nil")
	}

	tx := MultisigTx{
		TxID: 1,
		Data: data,
	}

	// Use GORM's Save method which will update if exists, create if not
	result := db.Save(&tx)
	return result.Error
}

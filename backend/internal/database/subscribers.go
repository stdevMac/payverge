package database

import (
	"web3-boilerplate/internal/structs"
	"gorm.io/gorm"
	"errors"
)

func AddSubscriber(sub structs.Subscriber) error {
	// Check if subscriber already exists
	var existingSub Subscriber
	result := db.Where("email = ?", sub.Email).First(&existingSub)
	if result.Error == nil {
		// Subscriber already exists
		return nil
	}

	// Only proceed if the error is "record not found"
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}

	// Insert new subscriber
	dbSub := Subscriber{
		Email: sub.Email,
	}
	
	result = db.Create(&dbSub)
	return result.Error
}

func GetSubscribers() ([]structs.Subscriber, error) {
	var dbSubscribers []Subscriber
	result := db.Find(&dbSubscribers)
	if result.Error != nil {
		return nil, result.Error
	}

	subscribers := make([]structs.Subscriber, len(dbSubscribers))
	for i, dbSub := range dbSubscribers {
		subscribers[i] = structs.Subscriber{
			Email: dbSub.Email,
		}
	}

	return subscribers, nil
}

func UnSubscribe(email string) error {
	result := db.Where("email = ?", email).Delete(&Subscriber{})
	return result.Error
}

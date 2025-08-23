package database

import (
	"context"
	"time"
)

// StoreErrorLog stores an error log in the database
func StoreErrorLog(ctx context.Context, log *ErrorLog) error {
	result := db.Create(log)
	return result.Error
}

// GetErrorLogs retrieves error logs within a time range
func GetErrorLogs(ctx context.Context, startTime, endTime time.Time) ([]*ErrorLog, error) {
	var logs []*ErrorLog
	result := db.Where("timestamp BETWEEN ? AND ?", startTime, endTime).
		Order("timestamp desc").
		Find(&logs)
	
	if result.Error != nil {
		return nil, result.Error
	}

	return logs, nil
}

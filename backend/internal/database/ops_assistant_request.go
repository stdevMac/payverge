package database

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ErrOpsAssistantRequestInFlight is returned when another worker holds the claim.
var ErrOpsAssistantRequestInFlight = errors.New("ops assistant request already in flight")

// ClaimOpsAssistantRequest inserts or claims a durable client request ledger row.
// Completed requests return the existing row so retries replay without re-spend.
func ClaimOpsAssistantRequest(businessID uint, clientRequestID string) (*OpsAssistantRequest, bool, error) {
	clientRequestID = strings.TrimSpace(clientRequestID)
	if businessID == 0 || clientRequestID == "" {
		return nil, false, fmt.Errorf("business_id and client_request_id required")
	}
	now := time.Now().UTC()
	var existing OpsAssistantRequest
	err := GetDB().Where("business_id = ? AND client_request_id = ?", businessID, clientRequestID).
		First(&existing).Error
	if err == nil {
		switch existing.Status {
		case "completed":
			return &existing, true, nil // replay
		case "pending", "processing":
			return nil, false, ErrOpsAssistantRequestInFlight
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	// Status values must match the chk_ops_assistant_requests_status CHECK:
	// pending | processing | completed | failed.
	row := &OpsAssistantRequest{
		BusinessID:      businessID,
		ClientRequestID: clientRequestID,
		Status:          "processing",
		ClaimToken:      fmt.Sprintf("claim_%d", now.UnixNano()),
		ClaimedAt:       &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := GetDB().Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "business_id"}, {Name: "client_request_id"}},
		DoNothing: true,
	}).Create(row).Error; err != nil {
		return nil, false, err
	}
	// Re-read in case of race.
	if row.ID == 0 {
		if err := GetDB().Where("business_id = ? AND client_request_id = ?", businessID, clientRequestID).
			First(&existing).Error; err != nil {
			return nil, false, err
		}
		if existing.Status == "completed" {
			return &existing, true, nil
		}
		if existing.Status == "processing" || existing.Status == "pending" {
			return nil, false, ErrOpsAssistantRequestInFlight
		}
		return &existing, false, nil
	}
	return row, false, nil
}

// CompleteOpsAssistantRequest marks a claim completed and links message IDs.
func CompleteOpsAssistantRequest(id uint, threadID, userMsgID, assistantMsgID uint, errCode string) error {
	updates := map[string]any{
		"status":               "completed",
		"thread_id":            threadID,
		"user_message_id":      userMsgID,
		"assistant_message_id": assistantMsgID,
		"error_code":           errCode,
		"updated_at":           time.Now().UTC(),
	}
	return GetDB().Model(&OpsAssistantRequest{}).Where("id = ?", id).Updates(updates).Error
}

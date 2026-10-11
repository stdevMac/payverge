package database

import (
	"encoding/json"
	"fmt"
)

// CreateAdminAction records an admin action taken on a user account.
func CreateAdminAction(adminUserID, targetUserID uint, actionType string, details map[string]interface{}) error {
	detailsJSON, err := json.Marshal(details)
	if err != nil {
		return fmt.Errorf("failed to marshal admin action details: %w", err)
	}

	action := AdminAction{
		AdminUserID:  adminUserID,
		TargetUserID: targetUserID,
		ActionType:   actionType,
		Details:      detailsJSON,
	}

	if err := db.Create(&action).Error; err != nil {
		return fmt.Errorf("failed to create admin action: %w", err)
	}

	return nil
}

// GetAdminActionsForUser retrieves admin actions for a given user, ordered by most recent first.
func GetAdminActionsForUser(targetUserID uint, limit int) ([]AdminAction, error) {
	var actions []AdminAction
	query := db.Where("target_user_id = ?", targetUserID).Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&actions).Error; err != nil {
		return nil, fmt.Errorf("failed to get admin actions for user %d: %w", targetUserID, err)
	}
	return actions, nil
}

// GetAdminActionsForBusinessContext returns admin actions tied to a business owner
// and/or rows whose JSON details include business_id (owner-less gifts, suspends, etc.).
func GetAdminActionsForBusinessContext(businessID uint, ownerUserID uint, limit int) ([]AdminAction, error) {
	var actions []AdminAction
	businessClause, businessArgs := adminActionBusinessIDEquals(businessID)
	query := db.Where("target_user_id = ? OR "+businessClause, append([]interface{}{ownerUserID}, businessArgs...)...).
		Order("created_at DESC")
	if limit > 0 {
		query = query.Limit(limit)
	}
	if err := query.Find(&actions).Error; err != nil {
		return nil, fmt.Errorf("failed to get admin actions for business %d: %w", businessID, err)
	}
	return actions, nil
}

func adminActionBusinessIDEquals(businessID uint) (string, []interface{}) {
	if db != nil && db.Dialector.Name() == "sqlite" {
		return "CAST(json_extract(details, '$.business_id') AS INTEGER) = ?", []interface{}{businessID}
	}
	return "CAST(details->>'business_id' AS INTEGER) = ?", []interface{}{businessID}
}

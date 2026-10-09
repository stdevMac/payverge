package operational_alerts

import (
	"context"
	"fmt"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func (s *Service) CreateStaleOccupiedTableAlert(
	ctx context.Context,
	businessID uint,
	tableID uint,
	tableName string,
	billID uint,
	openedAt time.Time,
) error {
	ageMinutes := int64(time.Since(openedAt).Minutes())
	_, err := s.UpsertAlert(ctx, UpsertAlertInput{
		BusinessID:   businessID,
		AlertType:    database.OperationalAlertTypeTableStaleOccupied,
		ResourceType: database.OperationalAlertResourceTypeTable,
		ResourceID:   tableID,
		Priority:     database.OperationalAlertPriorityHigh,
		Title:        "Stale occupied table",
		Body:         fmt.Sprintf("%s still has an old open bill", tableName),
		Metadata: map[string]any{
			"table_id":    tableID,
			"table_name":  tableName,
			"bill_id":     billID,
			"opened_at":   openedAt.UTC().Format(time.RFC3339),
			"age_minutes": ageMinutes,
		},
	})
	return err
}

// ResolveStaleOccupiedTableAlert is idempotent and deliberately scopes by
// alert type so a service-call alert for the same table is never resolved.
func (s *Service) ResolveStaleOccupiedTableAlert(
	ctx context.Context,
	businessID uint,
	tableID uint,
) error {
	var alert database.OperationalAlert
	result := s.db.WithContext(ctx).Where(
		"business_id = ? AND alert_type = ? AND resource_type = ? AND resource_id = ? AND status IN ?",
		businessID,
		database.OperationalAlertTypeTableStaleOccupied,
		database.OperationalAlertResourceTypeTable,
		tableID,
		[]database.OperationalAlertStatus{
			database.OperationalAlertStatusOpen,
			database.OperationalAlertStatusClaimed,
		},
	).Limit(1).Find(&alert)
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	_, err := s.ResolveAlertByIDForBusiness(
		ctx,
		businessID,
		alert.ID,
		Actor{Name: "system"},
		"table no longer has a stale active bill",
	)
	return err
}

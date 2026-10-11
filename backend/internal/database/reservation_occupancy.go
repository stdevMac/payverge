package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

var ErrReservationTableOccupied = errors.New("reservation table has an active bill")

type ReservationTableOccupiedError struct {
	TableID      uint
	State        TableOccupancyState
	ActiveBillID uint
	OpenedAt     time.Time
}

func (e *ReservationTableOccupiedError) Error() string {
	return fmt.Sprintf("table %d is %s", e.TableID, e.State)
}

func (e *ReservationTableOccupiedError) Unwrap() error {
	return ErrReservationTableOccupied
}

type ReservationWriteGuards struct {
	OccupancyCheckAt      *time.Time
	StaleAfter            time.Duration
	AllowOccupiedOverride bool
}

// TableOccupancyState describes whether an operator may safely assign a
// near-term reservation to a table. An old bill remains occupied; stale is an
// operational warning, not an availability escape hatch.
type TableOccupancyState string

const (
	TableOccupancyAvailable     TableOccupancyState = "available"
	TableOccupancyOccupied      TableOccupancyState = "occupied"
	TableOccupancyStaleOccupied TableOccupancyState = "stale_occupied"
)

type ReservationTableOccupancy struct {
	TableID            uint
	State              TableOccupancyState
	ActiveBillID       *uint
	ActiveBillOpenedAt *time.Time
	ActiveBillAge      time.Duration
}

func guardReservationTableOccupancyTx(
	tx *gorm.DB,
	tableID uint,
	guards ReservationWriteGuards,
) error {
	if guards.OccupancyCheckAt == nil || guards.AllowOccupiedOverride {
		return nil
	}
	var bill Bill
	result := tx.Select("id", "table_id", "created_at").
		Where("table_id = ? AND status IN ?", tableID, activeBillStatusStrings()).
		Order("created_at DESC").
		Limit(1).
		Find(&bill)
	if result.Error != nil || result.RowsAffected == 0 {
		return result.Error
	}
	state := TableOccupancyOccupied
	if guards.OccupancyCheckAt.Sub(bill.CreatedAt) >= guards.StaleAfter {
		state = TableOccupancyStaleOccupied
	}
	return &ReservationTableOccupiedError{
		TableID:      tableID,
		State:        state,
		ActiveBillID: bill.ID,
		OpenedAt:     bill.CreatedAt,
	}
}

// GetReservationTableOccupancySnapshot returns one in-memory result for every
// requested table and performs exactly one projected bill read. Callers must
// still repeat this read inside their write transaction after locking a table.
func GetReservationTableOccupancySnapshot(
	tx *gorm.DB,
	businessID uint,
	tableIDs []uint,
	now time.Time,
	staleAfter time.Duration,
) (map[uint]ReservationTableOccupancy, error) {
	out := make(map[uint]ReservationTableOccupancy, len(tableIDs))
	for _, tableID := range tableIDs {
		out[tableID] = ReservationTableOccupancy{
			TableID: tableID,
			State:   TableOccupancyAvailable,
		}
	}
	if len(tableIDs) == 0 {
		return out, nil
	}

	var bills []Bill
	if err := tx.Select("id", "table_id", "created_at").
		Where(
			"business_id = ? AND table_id IN ? AND status IN ?",
			businessID,
			tableIDs,
			activeBillStatusStrings(),
		).
		Order("created_at DESC").
		Find(&bills).Error; err != nil {
		return nil, err
	}

	for _, bill := range bills {
		current, requested := out[bill.TableID]
		if !requested || current.ActiveBillID != nil {
			continue
		}
		billID := bill.ID
		openedAt := bill.CreatedAt
		age := now.Sub(openedAt)
		state := TableOccupancyOccupied
		if age >= staleAfter {
			state = TableOccupancyStaleOccupied
		}
		out[bill.TableID] = ReservationTableOccupancy{
			TableID:            bill.TableID,
			State:              state,
			ActiveBillID:       &billID,
			ActiveBillOpenedAt: &openedAt,
			ActiveBillAge:      age,
		}
	}

	return out, nil
}

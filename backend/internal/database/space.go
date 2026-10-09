package database

import (
	"errors"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ErrSpaceNotFound is returned when no restaurant space matches the scoped lookup.
var ErrSpaceNotFound = errors.New("space not found")

// ErrSpaceRevisionConflict is returned when draft_revision does not match the
// expected optimistic-concurrency revision.
var ErrSpaceRevisionConflict = errors.New("space draft revision conflict")

// CreateRestaurantSpace inserts a new space scoped to a business.
func CreateRestaurantSpace(space *RestaurantSpace) error {
	if space == nil {
		return fmt.Errorf("space is nil")
	}
	if space.BusinessID == 0 {
		return fmt.Errorf("business_id is required")
	}
	if space.Name == "" {
		return fmt.Errorf("name is required")
	}
	if space.MeasurementUnit == "" {
		space.MeasurementUnit = SpaceUnitMeters
	}
	if space.Status == "" {
		space.Status = SpaceStatusDraft
	}
	if space.LayoutSchemaVersion == 0 {
		space.LayoutSchemaVersion = 1
	}
	if space.DraftRevision == 0 {
		space.DraftRevision = 1
	}
	if len(space.BoundaryJSON) == 0 {
		space.BoundaryJSON = JSONRawMessage(`{}`)
	}
	if len(space.DraftLayoutJSON) == 0 {
		space.DraftLayoutJSON = JSONRawMessage(`{}`)
	}
	if err := db.Create(space).Error; err != nil {
		return fmt.Errorf("create restaurant space: %w", err)
	}
	return nil
}

// GetRestaurantSpaceByID returns a space only when it belongs to businessID.
func GetRestaurantSpaceByID(businessID, spaceID uint) (*RestaurantSpace, error) {
	var space RestaurantSpace
	err := db.Where("id = ? AND business_id = ?", spaceID, businessID).First(&space).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrSpaceNotFound
		}
		return nil, fmt.Errorf("get restaurant space: %w", err)
	}
	return &space, nil
}

// ListRestaurantSpaces returns spaces for a business ordered by sort_order, id.
// When includeArchived is false, archived spaces are excluded.
func ListRestaurantSpaces(businessID uint, includeArchived bool) ([]RestaurantSpace, error) {
	var spaces []RestaurantSpace
	q := db.Where("business_id = ?", businessID)
	if !includeArchived {
		q = q.Where("status <> ?", SpaceStatusArchived)
	}
	if err := q.Order("sort_order ASC, id ASC").Find(&spaces).Error; err != nil {
		return nil, fmt.Errorf("list restaurant spaces: %w", err)
	}
	return spaces, nil
}

// UpdateRestaurantSpaceDraft persists draft layout JSON and related draft fields
// under optimistic concurrency. expectedRevision must match the current
// draft_revision; on success draft_revision is incremented by 1.
func UpdateRestaurantSpaceDraft(
	businessID, spaceID uint,
	expectedRevision int64,
	draftLayout JSONRawMessage,
	boundary JSONRawMessage,
	widthMm, heightMm *int,
	hasUnpublished bool,
) (*RestaurantSpace, error) {
	var out *RestaurantSpace
	err := db.Transaction(func(tx *gorm.DB) error {
		var space RestaurantSpace
		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).
			First(&space).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSpaceNotFound
			}
			return err
		}
		if space.DraftRevision != expectedRevision {
			return ErrSpaceRevisionConflict
		}

		updates := map[string]interface{}{
			"draft_layout_json":       draftLayout,
			"draft_revision":          space.DraftRevision + 1,
			"has_unpublished_changes": hasUnpublished,
			"updated_at":              time.Now().UTC(),
		}
		if boundary != nil {
			updates["boundary_json"] = boundary
		}
		if widthMm != nil {
			updates["width_mm"] = *widthMm
		}
		if heightMm != nil {
			updates["height_mm"] = *heightMm
		}

		res := tx.Model(&RestaurantSpace{}).
			Where("id = ? AND business_id = ? AND draft_revision = ?", spaceID, businessID, expectedRevision).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrSpaceRevisionConflict
		}
		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).First(&space).Error; err != nil {
			return err
		}
		out = &space
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// DiscardRestaurantSpaceDraft resets draft layout to the published layout (or empty).
func DiscardRestaurantSpaceDraft(businessID, spaceID uint, expectedRevision int64) (*RestaurantSpace, error) {
	var out *RestaurantSpace
	err := db.Transaction(func(tx *gorm.DB) error {
		var space RestaurantSpace
		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).
			First(&space).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSpaceNotFound
			}
			return err
		}
		if space.DraftRevision != expectedRevision {
			return ErrSpaceRevisionConflict
		}

		draft := space.PublishedLayoutJSON
		if len(draft) == 0 {
			draft = JSONRawMessage(`{}`)
		}
		now := time.Now().UTC()
		updates := map[string]interface{}{
			"draft_layout_json":       draft,
			"draft_revision":          space.DraftRevision + 1,
			"has_unpublished_changes": false,
			"updated_at":              now,
		}
		res := tx.Model(&RestaurantSpace{}).
			Where("id = ? AND business_id = ? AND draft_revision = ?", spaceID, businessID, expectedRevision).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrSpaceRevisionConflict
		}
		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).First(&space).Error; err != nil {
			return err
		}
		out = &space
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ArchiveRestaurantSpace soft-archives a space (status=archived, archived_at set).
// Does not hard-delete; tables are unassigned from the space.
func ArchiveRestaurantSpace(businessID, spaceID uint) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var space RestaurantSpace
		if err := tx.Where("id = ? AND business_id = ?", spaceID, businessID).
			First(&space).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return ErrSpaceNotFound
			}
			return err
		}
		now := time.Now().UTC()
		if err := tx.Model(&RestaurantSpace{}).
			Where("id = ? AND business_id = ?", spaceID, businessID).
			Updates(map[string]interface{}{
				"status":      SpaceStatusArchived,
				"archived_at": now,
				"updated_at":  now,
			}).Error; err != nil {
			return err
		}
		// Unassign tables from this space (keep table history intact).
		if err := tx.Model(&Table{}).
			Where("business_id = ? AND space_id = ?", businessID, spaceID).
			Updates(map[string]interface{}{
				"space_id":         nil,
				"region_id":        nil,
				"layout_in_draft":  false,
				"layout_published": false,
				"updated_at":       now,
			}).Error; err != nil {
			return err
		}
		return nil
	})
}

// SoftDeleteRestaurantSpace sets deleted_at (GORM soft delete) after archive semantics.
func SoftDeleteRestaurantSpace(businessID, spaceID uint) error {
	if err := ArchiveRestaurantSpace(businessID, spaceID); err != nil {
		return err
	}
	res := db.Where("id = ? AND business_id = ?", spaceID, businessID).Delete(&RestaurantSpace{})
	if res.Error != nil {
		return fmt.Errorf("soft delete restaurant space: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return ErrSpaceNotFound
	}
	return nil
}

// DuplicateRestaurantSpace clones space metadata + draft/published layout JSON
// into a new draft space for the same business. Relational elements/regions are
// not cloned here — callers in the domain service handle that when needed.
func DuplicateRestaurantSpace(businessID, spaceID uint, newName string) (*RestaurantSpace, error) {
	src, err := GetRestaurantSpaceByID(businessID, spaceID)
	if err != nil {
		return nil, err
	}
	clone := &RestaurantSpace{
		BusinessID:            businessID,
		Name:                  newName,
		SpaceType:             src.SpaceType,
		FloorLevel:            src.FloorLevel,
		SortOrder:             src.SortOrder + 1,
		MeasurementUnit:       src.MeasurementUnit,
		Status:                SpaceStatusDraft,
		WidthMm:               src.WidthMm,
		HeightMm:              src.HeightMm,
		BoundaryJSON:          append(JSONRawMessage(nil), src.BoundaryJSON...),
		DraftLayoutJSON:       append(JSONRawMessage(nil), src.DraftLayoutJSON...),
		PublishedLayoutJSON:   nil,
		LayoutSchemaVersion:   src.LayoutSchemaVersion,
		DraftRevision:         1,
		PublishedRevision:     0,
		HasUnpublishedChanges: true,
		ScanSource:            src.ScanSource,
	}
	if len(clone.DraftLayoutJSON) == 0 && len(src.PublishedLayoutJSON) > 0 {
		clone.DraftLayoutJSON = append(JSONRawMessage(nil), src.PublishedLayoutJSON...)
	}
	if err := CreateRestaurantSpace(clone); err != nil {
		return nil, err
	}
	return clone, nil
}

// AssignTablesToSpace sets space_id on the given tables for the business.
// Tables not owned by businessID are rejected (tenant isolation).
func AssignTablesToSpace(businessID, spaceID uint, tableIDs []uint) error {
	if len(tableIDs) == 0 {
		return nil
	}
	// Verify space belongs to business.
	if _, err := GetRestaurantSpaceByID(businessID, spaceID); err != nil {
		return err
	}
	var count int64
	if err := db.Model(&Table{}).
		Where("business_id = ? AND id IN ?", businessID, tableIDs).
		Count(&count).Error; err != nil {
		return fmt.Errorf("count tables for assign: %w", err)
	}
	if int(count) != len(tableIDs) {
		return fmt.Errorf("one or more tables not found in business %d", businessID)
	}
	now := time.Now().UTC()
	if err := db.Model(&Table{}).
		Where("business_id = ? AND id IN ?", businessID, tableIDs).
		Updates(map[string]interface{}{
			"space_id":        spaceID,
			"layout_in_draft": true,
			"updated_at":      now,
		}).Error; err != nil {
		return fmt.Errorf("assign tables to space: %w", err)
	}
	return nil
}

// ListTablesBySpace returns tables assigned to a space within a business.
func ListTablesBySpace(businessID, spaceID uint) ([]Table, error) {
	var tables []Table
	if err := db.Where("business_id = ? AND space_id = ?", businessID, spaceID).
		Order("id ASC").Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("list tables by space: %w", err)
	}
	return tables, nil
}

// ListUnassignedTables returns active tables with no space_id for a business.
func ListUnassignedTables(businessID uint) ([]Table, error) {
	var tables []Table
	if err := db.Where("business_id = ? AND is_active = ? AND space_id IS NULL", businessID, true).
		Order("id ASC").Find(&tables).Error; err != nil {
		return nil, fmt.Errorf("list unassigned tables: %w", err)
	}
	return tables, nil
}

// TableLayoutPlacement is the relational projection of a published layout table.
type TableLayoutPlacement struct {
	TableID          uint
	RegionID         *uint
	PosXMm           *int
	PosYMm           *int
	WidthMm          *int
	HeightMm         *int
	RotationDeg      float64
	Shape            string
	MinCapacity      *int
	MaxCapacity      *int
	VisibleSeatCount *int
	// Capacity updates the legacy tables.capacity column (list/reservations).
	Capacity     *int
	IsReservable *bool
	IsCombinable *bool
	IsAccessible *bool
}

// CountOpenBillsForTables returns how many active bills reference any of the table IDs.
func CountOpenBillsForTables(businessID uint, tableIDs []uint) (int64, error) {
	if len(tableIDs) == 0 {
		return 0, nil
	}
	var count int64
	if err := db.Model(&Bill{}).
		Where("business_id = ? AND table_id IN ? AND status IN ?",
			businessID, tableIDs, activeBillStatusStrings()).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count open bills for tables: %w", err)
	}
	return count, nil
}

// CountActiveReservationsForTables returns pending/confirmed future reservations for tables.
func CountActiveReservationsForTables(businessID uint, tableIDs []uint, now time.Time) (int64, error) {
	if len(tableIDs) == 0 {
		return 0, nil
	}
	var count int64
	if err := db.Model(&TableReservation{}).
		Where("business_id = ? AND table_id IN ? AND status IN ? AND reservation_time >= ?",
			businessID, tableIDs, []string{"pending", "confirmed"}, now).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count active reservations for tables: %w", err)
	}
	return count, nil
}

// UpdateRestaurantSpaceMeta patches rename/type/floor/unit/sort fields.
// Only non-nil pointers are applied. businessID scopes the update.
func UpdateRestaurantSpaceMeta(
	businessID, spaceID uint,
	name *string,
	spaceType *string,
	floorLevel *int,
	measurementUnit *string,
	sortOrder *int,
) (*RestaurantSpace, error) {
	space, err := GetRestaurantSpaceByID(businessID, spaceID)
	if err != nil {
		return nil, err
	}
	updates := map[string]interface{}{
		"updated_at": time.Now().UTC(),
	}
	if name != nil {
		if *name == "" {
			return nil, fmt.Errorf("name cannot be empty")
		}
		updates["name"] = *name
	}
	if spaceType != nil {
		updates["space_type"] = *spaceType
	}
	if floorLevel != nil {
		updates["floor_level"] = *floorLevel
	}
	if measurementUnit != nil {
		switch *measurementUnit {
		case SpaceUnitMeters, SpaceUnitFeet:
			updates["measurement_unit"] = *measurementUnit
		default:
			return nil, fmt.Errorf("invalid measurement_unit %q", *measurementUnit)
		}
	}
	if sortOrder != nil {
		updates["sort_order"] = *sortOrder
	}
	if len(updates) == 1 {
		// only updated_at — nothing to change
		return space, nil
	}
	res := db.Model(&RestaurantSpace{}).
		Where("id = ? AND business_id = ?", spaceID, businessID).
		Updates(updates)
	if res.Error != nil {
		return nil, fmt.Errorf("update restaurant space meta: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return nil, ErrSpaceNotFound
	}
	return GetRestaurantSpaceByID(businessID, spaceID)
}

// SpaceSortEntry is one item in a bulk reorder payload.
type SpaceSortEntry struct {
	ID        uint
	SortOrder int
}

// ReorderRestaurantSpaces updates sort_order for the given spaces scoped to businessID.
// Foreign space IDs are rejected (tenant isolation).
func ReorderRestaurantSpaces(businessID uint, entries []SpaceSortEntry) error {
	if len(entries) == 0 {
		return nil
	}
	ids := make([]uint, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	var count int64
	if err := db.Model(&RestaurantSpace{}).
		Where("business_id = ? AND id IN ?", businessID, ids).
		Count(&count).Error; err != nil {
		return fmt.Errorf("count spaces for reorder: %w", err)
	}
	if int(count) != len(ids) {
		return fmt.Errorf("one or more spaces not found in business %d", businessID)
	}
	return db.Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		for _, e := range entries {
			if err := tx.Model(&RestaurantSpace{}).
				Where("id = ? AND business_id = ?", e.ID, businessID).
				Updates(map[string]interface{}{
					"sort_order": e.SortOrder,
					"updated_at": now,
				}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SpacesSummary aggregates space/table counts for the operator dashboard.
type SpacesSummary struct {
	TotalSpaces      int64 `json:"total_spaces"`
	DraftSpaces      int64 `json:"draft_spaces"`
	PublishedSpaces  int64 `json:"published_spaces"`
	ArchivedSpaces   int64 `json:"archived_spaces"`
	UnassignedTables int64 `json:"unassigned_tables"`
	AssignedTables   int64 `json:"assigned_tables"`
}

// GetSpacesSummary returns space and table placement totals for a business.
func GetSpacesSummary(businessID uint) (*SpacesSummary, error) {
	s := &SpacesSummary{}
	if err := db.Model(&RestaurantSpace{}).
		Where("business_id = ?", businessID).
		Count(&s.TotalSpaces).Error; err != nil {
		return nil, fmt.Errorf("count spaces: %w", err)
	}
	if err := db.Model(&RestaurantSpace{}).
		Where("business_id = ? AND status = ?", businessID, SpaceStatusDraft).
		Count(&s.DraftSpaces).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&RestaurantSpace{}).
		Where("business_id = ? AND status = ?", businessID, SpaceStatusPublished).
		Count(&s.PublishedSpaces).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&RestaurantSpace{}).
		Where("business_id = ? AND status = ?", businessID, SpaceStatusArchived).
		Count(&s.ArchivedSpaces).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&Table{}).
		Where("business_id = ? AND is_active = ? AND space_id IS NULL", businessID, true).
		Count(&s.UnassignedTables).Error; err != nil {
		return nil, err
	}
	if err := db.Model(&Table{}).
		Where("business_id = ? AND is_active = ? AND space_id IS NOT NULL", businessID, true).
		Count(&s.AssignedTables).Error; err != nil {
		return nil, err
	}
	return s, nil
}

// CreateSpaceLayoutAuditEvent appends an audit row.
func CreateSpaceLayoutAuditEvent(ev *SpaceLayoutAuditEvent) error {
	if ev == nil {
		return fmt.Errorf("audit event is nil")
	}
	if ev.BusinessID == 0 {
		return fmt.Errorf("business_id is required")
	}
	if ev.Action == "" {
		return fmt.Errorf("action is required")
	}
	if len(ev.DetailJSON) == 0 {
		ev.DetailJSON = JSONRawMessage(`{}`)
	}
	if err := db.Create(ev).Error; err != nil {
		return fmt.Errorf("create space layout audit event: %w", err)
	}
	return nil
}

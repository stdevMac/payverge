package server

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/spaces"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

var spaceDomainService = spaces.NewService()

func getSpaceRouteBusiness(c *gin.Context) (*database.Business, bool) {
	business, err := database.GetBusinessByIdOrBusinessId(utils.BusinessIdentifierFromParam(c, "id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Business not found"})
		return nil, false
	}
	if !CheckBusinessAccess(c, business) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Access denied"})
		return nil, false
	}
	return business, true
}

func parseSpaceIDParam(c *gin.Context) (uint, bool) {
	raw := c.Param("spaceId")
	id, err := strconv.ParseUint(raw, 10, 32)
	if err != nil || id == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid space ID", "code": "invalid_space_id"})
		return 0, false
	}
	return uint(id), true
}

func mapSpaceDomainError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var conflict *spaces.ConflictError
	if errors.As(err, &conflict) {
		c.JSON(http.StatusConflict, gin.H{
			"error":             "Draft revision conflict",
			"code":              "revision_conflict",
			"expected_revision": conflict.ExpectedRevision,
			"actual_revision":   conflict.ActualRevision,
		})
		return true
	}
	var validation *spaces.ValidationError
	if errors.As(err, &validation) {
		code := "layout_invalid"
		// Promote known domain codes for FE branching (e.g. draft_has_content).
		if len(validation.Result.Issues) > 0 {
			switch validation.Result.Issues[0].Code {
			case "draft_has_content":
				code = "draft_has_content"
			case "review_layout_rejected":
				code = "review_layout_rejected"
			}
		}
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":  "Layout validation failed",
			"code":   code,
			"issues": validation.Result.Issues,
		})
		return true
	}
	var activity *spaces.ActivityBlockError
	if errors.As(err, &activity) {
		c.JSON(http.StatusConflict, gin.H{
			"error":             "Space has open activity",
			"code":              "has_dependencies",
			"open_bills":        activity.OpenBills,
			"open_reservations": activity.OpenReservations,
		})
		return true
	}
	if errors.Is(err, spaces.ErrNotFound) || errors.Is(err, database.ErrSpaceNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Space not found", "code": "not_found"})
		return true
	}
	if errors.Is(err, spaces.ErrTenantIsolation) {
		c.JSON(http.StatusForbidden, gin.H{"error": "Tenant isolation violation", "code": "tenant_isolation"})
		return true
	}
	if errors.Is(err, spaces.ErrInvalidArgument) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid argument", "code": "invalid_argument"})
		return true
	}
	return false
}

// CreateSpaceRequest is the body for POST /spaces.
type CreateSpaceRequest struct {
	Name            string `json:"name" binding:"required"`
	SpaceType       string `json:"space_type"`
	FloorLevel      *int   `json:"floor_level"`
	SortOrder       *int   `json:"sort_order"`
	MeasurementUnit string `json:"measurement_unit"`
}

// PatchSpaceRequest is the body for PATCH /spaces/:spaceId.
type PatchSpaceRequest struct {
	Name            *string `json:"name"`
	SpaceType       *string `json:"space_type"`
	FloorLevel      *int    `json:"floor_level"`
	SortOrder       *int    `json:"sort_order"`
	MeasurementUnit *string `json:"measurement_unit"`
}

// ReorderSpacesRequest is the body for POST /spaces/reorder.
type ReorderSpacesRequest struct {
	Items []struct {
		ID        uint `json:"id" binding:"required"`
		SortOrder int  `json:"sort_order"`
	} `json:"items"`
	// Also accept a bare array at top level via custom bind in handler.
}

// DraftLayoutRequest is the body for PUT layout/draft.
type DraftLayoutRequest struct {
	ExpectedRevision int64           `json:"expected_revision" binding:"required"`
	Layout           json.RawMessage `json:"layout" binding:"required"`
}

// PublishLayoutRequest is the body for POST layout/publish and discard.
type PublishLayoutRequest struct {
	ExpectedRevision int64 `json:"expected_revision" binding:"required"`
}

// AssignTablesRequest is the body for POST tables/assign.
type AssignTablesRequest struct {
	TableIDs []uint `json:"table_ids"`
}

// ListSpaces handles GET /businesses/:id/spaces
func ListSpaces(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	includeArchived := false
	if raw := c.Query("include_archived"); raw != "" {
		includeArchived = raw == "true" || raw == "1"
	}
	list, err := database.ListRestaurantSpaces(business.ID, includeArchived)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list spaces"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"spaces": list})
}

// GetSpacesSummary handles GET /businesses/:id/spaces/summary
func GetSpacesSummary(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	summary, err := database.GetSpacesSummary(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load spaces summary"})
		return
	}
	unassigned, err := database.ListUnassignedTables(business.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load unassigned tables"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"summary":           summary,
		"unassigned_tables": unassigned,
	})
}

// CreateSpace handles POST /businesses/:id/spaces
func CreateSpace(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	var req CreateSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Name is required", "code": "invalid_argument"})
		return
	}
	spaceType := strings.TrimSpace(req.SpaceType)
	if spaceType == "" {
		spaceType = "indoor"
	}
	unit := strings.TrimSpace(req.MeasurementUnit)
	if unit == "" {
		unit = database.SpaceUnitMeters
	}
	if unit != database.SpaceUnitMeters && unit != database.SpaceUnitFeet {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid measurement_unit", "code": "invalid_argument"})
		return
	}
	space := &database.RestaurantSpace{
		BusinessID:      business.ID,
		Name:            name,
		SpaceType:       spaceType,
		MeasurementUnit: unit,
		Status:          database.SpaceStatusDraft,
	}
	if req.FloorLevel != nil {
		space.FloorLevel = *req.FloorLevel
	}
	if req.SortOrder != nil {
		space.SortOrder = *req.SortOrder
	}
	if err := database.CreateRestaurantSpace(space); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create space"})
		return
	}
	actorUser, actorStaff := spaceActorIDs(c)
	_ = database.CreateSpaceLayoutAuditEvent(&database.SpaceLayoutAuditEvent{
		BusinessID:   business.ID,
		SpaceID:      &space.ID,
		ActorUserID:  actorUser,
		ActorStaffID: actorStaff,
		Action:       "space_created",
		DetailJSON:   database.JSONRawMessage(`{}`),
	})
	c.JSON(http.StatusCreated, space)
}

// GetSpace handles GET /businesses/:id/spaces/:spaceId
func GetSpace(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	space, err := database.GetRestaurantSpaceByID(business.ID, spaceID)
	if err != nil {
		if errors.Is(err, database.ErrSpaceNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Space not found", "code": "not_found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load space"})
		return
	}
	tables, _ := database.ListTablesBySpace(business.ID, spaceID)
	c.JSON(http.StatusOK, gin.H{"space": space, "tables": tables})
}

// PatchSpace handles PATCH /businesses/:id/spaces/:spaceId
func PatchSpace(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var req PatchSpaceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		req.Name = &trimmed
	}
	space, err := database.UpdateRestaurantSpaceMeta(
		business.ID, spaceID, req.Name, req.SpaceType, req.FloorLevel, req.MeasurementUnit, req.SortOrder,
	)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		if strings.Contains(err.Error(), "invalid measurement_unit") || strings.Contains(err.Error(), "name cannot be empty") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error(), "code": "invalid_argument"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update space"})
		return
	}
	c.JSON(http.StatusOK, space)
}

// DuplicateSpace handles POST /businesses/:id/spaces/:spaceId/duplicate
func DuplicateSpace(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	_ = c.ShouldBindJSON(&body)
	name := strings.TrimSpace(body.Name)
	if name == "" {
		src, err := database.GetRestaurantSpaceByID(business.ID, spaceID)
		if err != nil {
			if mapSpaceDomainError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load space"})
			return
		}
		name = src.Name + " (copy)"
	}
	clone, err := spaceDomainService.DuplicateSpace(business.ID, spaceID, name)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to duplicate space"})
		return
	}
	c.JSON(http.StatusCreated, clone)
}

// ReorderSpaces handles POST /businesses/:id/spaces/reorder
func ReorderSpaces(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	// Accept either {"items":[{id,sort_order}]} or a bare array [{id,sort_order}].
	raw, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid body"})
		return
	}
	type item struct {
		ID        uint `json:"id"`
		SortOrder int  `json:"sort_order"`
	}
	var entries []item
	var wrapped struct {
		Items []item `json:"items"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && len(wrapped.Items) > 0 {
		entries = wrapped.Items
	} else if err := json.Unmarshal(raw, &entries); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid reorder payload", "code": "invalid_argument"})
		return
	}
	dbEntries := make([]database.SpaceSortEntry, len(entries))
	for i, e := range entries {
		if e.ID == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Each item requires id", "code": "invalid_argument"})
			return
		}
		dbEntries[i] = database.SpaceSortEntry{ID: e.ID, SortOrder: e.SortOrder}
	}
	if err := database.ReorderRestaurantSpaces(business.ID, dbEntries); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusForbidden, gin.H{"error": "One or more spaces not found", "code": "tenant_isolation"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to reorder spaces"})
		return
	}
	list, _ := database.ListRestaurantSpaces(business.ID, true)
	c.JSON(http.StatusOK, gin.H{"spaces": list})
}

// ArchiveSpace handles POST /businesses/:id/spaces/:spaceId/archive
func ArchiveSpace(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	if err := spaceDomainService.SafeDeleteOrArchive(business.ID, spaceID, false); err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to archive space"})
		return
	}
	space, _ := database.GetRestaurantSpaceByID(business.ID, spaceID)
	c.JSON(http.StatusOK, gin.H{"space": space, "status": "archived"})
}

// DeleteSpace handles DELETE /businesses/:id/spaces/:spaceId (safe soft-delete).
func DeleteSpace(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	if err := spaceDomainService.SafeDeleteOrArchive(business.ID, spaceID, true); err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete space"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"deleted": true, "space_id": spaceID})
}

// GetSpaceLayoutDraft handles GET .../layout/draft
func GetSpaceLayoutDraft(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	space, err := database.GetRestaurantSpaceByID(business.ID, spaceID)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load space"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"space_id":                space.ID,
		"draft_revision":          space.DraftRevision,
		"has_unpublished_changes": space.HasUnpublishedChanges,
		"layout":                  json.RawMessage(space.DraftLayoutJSON),
		"measurement_unit":        space.MeasurementUnit,
		"width_mm":                space.WidthMm,
		"height_mm":               space.HeightMm,
	})
}

// PutSpaceLayoutDraft handles PUT .../layout/draft (autosave with revision).
func PutSpaceLayoutDraft(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var req DraftLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	space, res, err := spaceDomainService.UpdateDraft(business.ID, spaceID, req.ExpectedRevision, req.Layout)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save draft"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"space":                   space,
		"draft_revision":          space.DraftRevision,
		"has_unpublished_changes": space.HasUnpublishedChanges,
		"validation":              res,
	})
}

// ValidateSpaceLayout handles POST .../layout/validate
func ValidateSpaceLayout(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var body struct {
		Layout json.RawMessage `json:"layout"`
	}
	_ = c.ShouldBindJSON(&body)
	layout := body.Layout
	if len(layout) == 0 {
		space, err := database.GetRestaurantSpaceByID(business.ID, spaceID)
		if err != nil {
			if mapSpaceDomainError(c, err) {
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load space"})
			return
		}
		layout = json.RawMessage(space.DraftLayoutJSON)
	}
	res, err := spaceDomainService.ValidateLayout(business.ID, spaceID, layout)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to validate layout"})
		return
	}
	c.JSON(http.StatusOK, res)
}

// PublishSpaceLayout handles POST .../layout/publish
func PublishSpaceLayout(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var req PublishLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	space, res, err := spaceDomainService.PublishLayout(business.ID, spaceID, req.ExpectedRevision)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to publish layout"})
		return
	}
	invalidateReservationAvailability(business.ID)
	c.JSON(http.StatusOK, gin.H{"space": space, "validation": res})
}

// DiscardSpaceLayout handles POST .../layout/discard
func DiscardSpaceLayout(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var req PublishLayoutRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondBindError(c, err)
		return
	}
	space, err := spaceDomainService.DiscardDraft(business.ID, spaceID, req.ExpectedRevision)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to discard draft"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"space": space})
}

// GetSpaceLayoutPublished handles GET .../layout/published
func GetSpaceLayoutPublished(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	space, err := database.GetRestaurantSpaceByID(business.ID, spaceID)
	if err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load space"})
		return
	}
	layout := space.PublishedLayoutJSON
	if len(layout) == 0 {
		layout = database.JSONRawMessage(`{}`)
	}
	c.JSON(http.StatusOK, gin.H{
		"space_id":           space.ID,
		"published_revision": space.PublishedRevision,
		"status":             space.Status,
		"layout":             json.RawMessage(layout),
		"measurement_unit":   space.MeasurementUnit,
		"width_mm":           space.WidthMm,
		"height_mm":          space.HeightMm,
	})
}

// AssignTablesToSpace handles POST .../tables/assign
func AssignTablesToSpaceHandler(c *gin.Context) {
	business, ok := getSpaceRouteBusiness(c)
	if !ok {
		return
	}
	spaceID, ok := parseSpaceIDParam(c)
	if !ok {
		return
	}
	var req AssignTablesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Empty body = assign all unassigned
		req.TableIDs = nil
	}
	if err := spaceDomainService.AssignLegacyTables(business.ID, spaceID, req.TableIDs); err != nil {
		if mapSpaceDomainError(c, err) {
			return
		}
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusForbidden, gin.H{"error": "One or more tables not found", "code": "tenant_isolation"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to assign tables"})
		return
	}
	tables, _ := database.ListTablesBySpace(business.ID, spaceID)
	c.JSON(http.StatusOK, gin.H{"tables": tables})
}

func spaceActorIDs(c *gin.Context) (*uint, *uint) {
	var userID, staffID *uint
	if v, ok := c.Get("user_id"); ok {
		if id, ok := extractContextUint(v); ok {
			userID = &id
		}
	}
	if v, ok := c.Get("staff_id"); ok {
		if id, ok := extractContextUint(v); ok {
			staffID = &id
		}
	}
	return userID, staffID
}

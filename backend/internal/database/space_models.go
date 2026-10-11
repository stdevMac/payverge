package database

import (
	"time"

	"gorm.io/gorm"
)

// Space status constants for restaurant_spaces.status.
const (
	SpaceStatusDraft     = "draft"
	SpaceStatusPublished = "published"
	SpaceStatusArchived  = "archived"
)

// Space measurement units.
const (
	SpaceUnitMeters = "m"
	SpaceUnitFeet   = "ft"
)

// Layout element types (must match migration CHECK).
const (
	LayoutElementWall           = "wall"
	LayoutElementDoor           = "door"
	LayoutElementWindow         = "window"
	LayoutElementColumn         = "column"
	LayoutElementBar            = "bar"
	LayoutElementCounter        = "counter"
	LayoutElementEntrance       = "entrance"
	LayoutElementStairs         = "stairs"
	LayoutElementServiceStation = "service_station"
	LayoutElementRestroom       = "restroom"
	LayoutElementDivider        = "divider"
	LayoutElementObstacle       = "obstacle"
	LayoutElementLabel          = "label"
)

// Scan session status constants.
const (
	ScanStatusWaitingForPhone = "waiting_for_phone"
	ScanStatusPhoneConnected  = "phone_connected"
	ScanStatusScanning        = "scanning"
	ScanStatusUploading       = "uploading"
	ScanStatusProcessing      = "processing"
	ScanStatusReviewReady     = "review_ready"
	ScanStatusFailed          = "failed"
	ScanStatusExpired         = "expired"
	ScanStatusCancelled       = "cancelled"
	ScanStatusCompleted       = "completed"
)

// Scan upload kinds.
const (
	ScanUploadRoomPlanJSON = "roomplan_json"
	ScanUploadKeyframes    = "keyframes"
	ScanUploadVideo        = "video"
	ScanUploadDepth        = "depth"
	ScanUploadMetadata     = "metadata"
)

// Table shape constants.
const (
	TableShapeRound     = "round"
	TableShapeSquare    = "square"
	TableShapeRectangle = "rectangle"
	TableShapeOval      = "oval"
	TableShapeBar       = "bar"
	TableShapeCustom    = "custom"
)

// RestaurantSpace is a floor-plan space belonging to a business (indoor dining
// room, patio, rooftop, etc.). Draft/published layout JSON is the document
// source of truth for the editor; relational tables/elements are synced on publish.
type RestaurantSpace struct {
	ID                    uint           `gorm:"primaryKey" json:"id"`
	BusinessID            uint           `gorm:"index;not null" json:"business_id"`
	Name                  string         `gorm:"not null" json:"name"`
	SpaceType             string         `gorm:"not null;default:'indoor'" json:"space_type"`
	FloorLevel            int            `gorm:"not null;default:0" json:"floor_level"`
	SortOrder             int            `gorm:"not null;default:0" json:"sort_order"`
	MeasurementUnit       string         `gorm:"not null;default:'m'" json:"measurement_unit"`
	Status                string         `gorm:"not null;default:'draft'" json:"status"`
	WidthMm               *int           `json:"width_mm,omitempty"`
	HeightMm              *int           `json:"height_mm,omitempty"`
	BoundaryJSON          JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"boundary_json"`
	DraftLayoutJSON       JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"draft_layout_json"`
	PublishedLayoutJSON   JSONRawMessage `gorm:"type:jsonb" json:"published_layout_json,omitempty"`
	LayoutSchemaVersion   int            `gorm:"not null;default:1" json:"layout_schema_version"`
	DraftRevision         int64          `gorm:"not null;default:1" json:"draft_revision"`
	PublishedRevision     int64          `gorm:"not null;default:0" json:"published_revision"`
	HasUnpublishedChanges bool           `gorm:"not null;default:false" json:"has_unpublished_changes"`
	ScanSource            *string        `json:"scan_source,omitempty"`
	ArchivedAt            *time.Time     `json:"archived_at,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// TableName returns the explicit table name (avoid bare "spaces" keyword).
func (RestaurantSpace) TableName() string { return "restaurant_spaces" }

// SpaceRegion is a named polygon region within a space (VIP, smoking, patio zone).
type SpaceRegion struct {
	ID          uint           `gorm:"primaryKey" json:"id"`
	SpaceID     uint           `gorm:"index;not null" json:"space_id"`
	BusinessID  uint           `gorm:"index;not null" json:"business_id"`
	Name        string         `gorm:"not null" json:"name"`
	Color       *string        `gorm:"size:7" json:"color,omitempty"`
	Description *string        `json:"description,omitempty"`
	Purpose     *string        `json:"purpose,omitempty"`
	PolygonJSON JSONRawMessage `gorm:"type:jsonb;not null;default:'[]'" json:"polygon_json"`
	SortOrder   int            `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

func (SpaceRegion) TableName() string { return "space_regions" }

// SpaceLayoutElement is a non-table structural or decorative element on a layout.
type SpaceLayoutElement struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	SpaceID      uint           `gorm:"index;not null" json:"space_id"`
	BusinessID   uint           `gorm:"index;not null" json:"business_id"`
	ElementType  string         `gorm:"not null" json:"element_type"`
	Name         *string        `json:"name,omitempty"`
	GeometryJSON JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"geometry_json"`
	XMm          *int           `json:"x_mm,omitempty"`
	YMm          *int           `json:"y_mm,omitempty"`
	WidthMm      *int           `json:"width_mm,omitempty"`
	HeightMm     *int           `json:"height_mm,omitempty"`
	RotationDeg  float64        `gorm:"not null;default:0" json:"rotation_deg"`
	ZIndex       int            `gorm:"not null;default:0" json:"z_index"`
	IsDraft      bool           `gorm:"not null;default:true" json:"is_draft"`
	MetaJSON     JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"meta_json"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
}

func (SpaceLayoutElement) TableName() string { return "space_layout_elements" }

// TableCombination groups tables that can be combined for large parties.
type TableCombination struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	BusinessID     uint      `gorm:"index;not null" json:"business_id"`
	Name           string    `gorm:"not null" json:"name"`
	PrimaryTableID uint      `gorm:"index;not null" json:"primary_table_id"`
	IsActive       bool      `gorm:"not null;default:true" json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func (TableCombination) TableName() string { return "table_combinations" }

// TableCombinationMember is a junction row for combination membership.
type TableCombinationMember struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	CombinationID uint      `gorm:"not null;uniqueIndex:uq_table_combination_members,priority:1" json:"combination_id"`
	BusinessID    uint      `gorm:"index;not null" json:"business_id"`
	TableID       uint      `gorm:"index;not null;uniqueIndex:uq_table_combination_members,priority:2" json:"table_id"`
	SortOrder     int       `gorm:"not null;default:0" json:"sort_order"`
	CreatedAt     time.Time `json:"created_at"`
}

// SpaceScanSession is a phone-scan capture session for generating a draft layout.
// The raw opaque token is never stored — only token_hash and a short prefix.
type SpaceScanSession struct {
	ID               uint           `gorm:"primaryKey" json:"id"`
	BusinessID       uint           `gorm:"index;not null" json:"business_id"`
	SpaceID          *uint          `json:"space_id,omitempty"`
	CreatedByUserID  *uint          `json:"created_by_user_id,omitempty"`
	CreatedByStaffID *uint          `json:"created_by_staff_id,omitempty"`
	TokenHash        string         `gorm:"uniqueIndex;not null" json:"-"`
	TokenPrefix      string         `gorm:"not null" json:"token_prefix"`
	Status           string         `gorm:"not null;default:'waiting_for_phone'" json:"status"`
	ExpiresAt        time.Time      `gorm:"not null" json:"expires_at"`
	ConnectedAt      *time.Time     `json:"connected_at,omitempty"`
	CompletedAt      *time.Time     `json:"completed_at,omitempty"`
	ProgressPct      int            `gorm:"not null;default:0" json:"progress_pct"`
	ProgressMessage  *string        `json:"progress_message,omitempty"`
	ErrorCode        *string        `json:"error_code,omitempty"`
	ErrorMessage     *string        `json:"error_message,omitempty"`
	DeviceMetaJSON   JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"device_meta_json"`
	CalibrationMm    *int           `json:"calibration_mm,omitempty"`
	IdempotencyKey   *string        `json:"idempotency_key,omitempty"`
	ResultLayoutJSON JSONRawMessage `gorm:"type:jsonb" json:"result_layout_json,omitempty"`
	RetainRawUntil   *time.Time     `json:"retain_raw_until,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
}

func (SpaceScanSession) TableName() string { return "space_scan_sessions" }

// SpaceScanUpload is one uploaded artifact (RoomPlan JSON, keyframes, etc.) for a scan session.
type SpaceScanUpload struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	SessionID      uint      `gorm:"index;not null" json:"session_id"`
	BusinessID     uint      `gorm:"index;not null" json:"business_id"`
	UploadKind     string    `gorm:"not null" json:"upload_kind"`
	ContentType    *string   `json:"content_type,omitempty"`
	ByteSize       *int64    `json:"byte_size,omitempty"`
	ChecksumSHA256 *string   `json:"checksum_sha256,omitempty"`
	S3Key          *string   `json:"s3_key,omitempty"`
	FormatVersion  int       `gorm:"not null;default:1" json:"format_version"`
	PartIndex      int       `gorm:"not null;default:0" json:"part_index"`
	IsComplete     bool      `gorm:"not null;default:false" json:"is_complete"`
	IdempotencyKey *string   `gorm:"uniqueIndex" json:"idempotency_key,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

func (SpaceScanUpload) TableName() string { return "space_scan_uploads" }

// SpaceLayoutAuditEvent records operator/system layout mutations for audit.
type SpaceLayoutAuditEvent struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	BusinessID   uint           `gorm:"index;not null" json:"business_id"`
	SpaceID      *uint          `json:"space_id,omitempty"`
	ActorUserID  *uint          `json:"actor_user_id,omitempty"`
	ActorStaffID *uint          `json:"actor_staff_id,omitempty"`
	Action       string         `gorm:"not null" json:"action"`
	DetailJSON   JSONRawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"detail_json"`
	CreatedAt    time.Time      `json:"created_at"`
}

func (SpaceLayoutAuditEvent) TableName() string { return "space_layout_audit_events" }

// Package scan converts phone-scan capture payloads into draft layout documents.
package scan

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/spaces"
	"github.com/stdevmac/payverge/backend/internal/spaces/geometry"
)

// ProcessInput is the shared input for all scan processors.
type ProcessInput struct {
	// BusinessID is required for tenant scoping of any table references.
	BusinessID uint
	// SpaceID is optional (session may not be bound yet).
	SpaceID *uint
	// Kind identifies the primary payload type.
	Kind string
	// Payload is the raw upload body (JSON for roomplan / metadata).
	Payload []byte
	// CalibrationMm is an optional real-world calibration length in millimeters.
	CalibrationMm *int
	// DeviceMeta is optional device metadata JSON.
	DeviceMeta json.RawMessage
	// Frames is optional keyframe pose/metadata (used by KeyframeProcessor).
	Frames []Keyframe
}

// Keyframe is a single capture frame with optional camera pose and user hints.
// Coordinates are in the device's capture frame; units may be meters or arbitrary.
type Keyframe struct {
	// Index is the frame order.
	Index int `json:"index"`
	// PosePosition is camera/device position in capture space (meters if Metric=true).
	PosePosition *geometry.Point `json:"pose_position,omitempty"`
	// PoseYawDeg is optional yaw in degrees.
	PoseYawDeg *float64 `json:"pose_yaw_deg,omitempty"`
	// Metric is true when pose units are meters; false means approximate/unitless.
	Metric bool `json:"metric"`
	// TableCandidates are operator or CV-suggested table rectangles in capture space.
	TableCandidates []TableCandidate `json:"table_candidates,omitempty"`
	// WallHints are optional wall endpoints in capture space.
	WallHints []WallHint `json:"wall_hints,omitempty"`
}

// TableCandidate is a suggested table placement from scan metadata.
type TableCandidate struct {
	Name        string  `json:"name,omitempty"`
	CenterX     float64 `json:"center_x"`
	CenterY     float64 `json:"center_y"`
	Width       float64 `json:"width"`
	Height      float64 `json:"height"`
	RotationDeg float64 `json:"rotation_deg,omitempty"`
	// SeatHint is optional seat count from user metadata (not metric precision).
	SeatHint *int `json:"seat_hint,omitempty"`
	// Confidence in [0,1].
	Confidence float64 `json:"confidence"`
}

// WallHint is a wall segment in capture space.
type WallHint struct {
	X1, Y1, X2, Y2 float64
	Confidence     float64 `json:"confidence"`
}

// DraftLayoutResult is the processor output applied as a space draft.
type DraftLayoutResult struct {
	Layout      *spaces.LayoutDocument `json:"layout"`
	Approximate bool                   `json:"approximate"`
	Confidence  float64                `json:"confidence"`
	Source      string                 `json:"source"`
	Notes       []string               `json:"notes,omitempty"`
	RawMeta     map[string]interface{} `json:"raw_meta,omitempty"`
}

// SpaceScanProcessor converts scan input into a draft layout.
type SpaceScanProcessor interface {
	Process(ctx context.Context, input ProcessInput) (*DraftLayoutResult, error)
}

// ProcessorForKind returns a processor for the given upload kind.
func ProcessorForKind(kind string) (SpaceScanProcessor, error) {
	switch kind {
	case "roomplan_json":
		return RoomPlanProcessor{}, nil
	case "keyframes", "metadata":
		return KeyframeProcessor{}, nil
	default:
		return nil, fmt.Errorf("no scan processor for kind %q", kind)
	}
}

// finalizeLayout sets schema version and meta flags on the document.
func finalizeLayout(doc *spaces.LayoutDocument, approximate bool, confidence float64, source string) {
	if doc == nil {
		return
	}
	doc.SchemaVersion = spaces.LayoutSchemaVersion
	if doc.Meta == nil {
		doc.Meta = map[string]interface{}{}
	}
	doc.Meta["approximate"] = approximate
	doc.Meta["confidence"] = confidence
	doc.Meta["source"] = source
}

// layoutFromBoundary builds a rectangular or polygon layout shell.
func layoutFromBoundary(points []geometry.Point, widthMM, heightMM int, unit string) *spaces.LayoutDocument {
	doc := &spaces.LayoutDocument{
		SchemaVersion:   spaces.LayoutSchemaVersion,
		WidthMm:         widthMM,
		HeightMm:        heightMM,
		MeasurementUnit: unit,
		Boundary: &spaces.LayoutBoundary{
			PointsMm: points,
			Closed:   true,
		},
		Tables:   []spaces.LayoutTable{},
		Elements: []spaces.LayoutElement{},
		Regions:  []spaces.LayoutRegion{},
	}
	return doc
}

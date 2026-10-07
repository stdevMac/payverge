package spaces

import (
	"encoding/json"
	"fmt"

	"github.com/stdevmac/payverge/backend/internal/spaces/geometry"
)

// LayoutSchemaVersion is the canonical layout document version.
const LayoutSchemaVersion = 1

// ValidationMode selects draft (candidates allowed, soft geometry) vs publish
// (linked table IDs required for every table).
type ValidationMode int

const (
	// ValidateDraft allows candidate tables (table_id == 0) and treats
	// overlap / out-of-bounds as non-blocking warnings.
	ValidateDraft ValidationMode = iota
	// ValidatePublish requires every table to have a real table_id > 0.
	// Soft geometry issues remain warnings (do not block publish).
	ValidatePublish
)

// LayoutDocument is the version-1 draft/published layout JSON shape.
type LayoutDocument struct {
	SchemaVersion   int                    `json:"schema_version"`
	WidthMm         int                    `json:"width_mm"`
	HeightMm        int                    `json:"height_mm"`
	MeasurementUnit string                 `json:"measurement_unit,omitempty"`
	Boundary        *LayoutBoundary        `json:"boundary,omitempty"`
	Regions         []LayoutRegion         `json:"regions,omitempty"`
	Elements        []LayoutElement        `json:"elements,omitempty"`
	Tables          []LayoutTable          `json:"tables,omitempty"`
	Meta            map[string]interface{} `json:"meta,omitempty"`
}

// LayoutBoundary is the space outline polygon in millimeters.
type LayoutBoundary struct {
	PointsMm []geometry.Point `json:"points_mm"`
	Closed   bool             `json:"closed"`
}

// LayoutRegion is a named region polygon inside a layout.
type LayoutRegion struct {
	ID        *uint            `json:"id,omitempty"`
	Name      string           `json:"name"`
	Color     string           `json:"color,omitempty"`
	Purpose   string           `json:"purpose,omitempty"`
	PolygonMm []geometry.Point `json:"polygon_mm"`
}

// LayoutElement is a non-table structural element.
type LayoutElement struct {
	ID          *uint                  `json:"id,omitempty"`
	ElementType string                 `json:"element_type"`
	Name        string                 `json:"name,omitempty"`
	Geometry    map[string]interface{} `json:"geometry,omitempty"`
	XMm         *int                   `json:"x_mm,omitempty"`
	YMm         *int                   `json:"y_mm,omitempty"`
	WidthMm     *int                   `json:"width_mm,omitempty"`
	HeightMm    *int                   `json:"height_mm,omitempty"`
	RotationDeg float64                `json:"rotation_deg"`
	ZIndex      int                    `json:"z_index"`
	Meta        map[string]interface{} `json:"meta,omitempty"`
}

// LayoutTable is a table placement in the layout document.
// TableID == 0 means an unlinked scan/editor candidate; allowed in drafts only.
type LayoutTable struct {
	TableID uint `json:"table_id"`
	// ClientKey is an optional stable id for candidates before materialization.
	ClientKey        string  `json:"client_key,omitempty"`
	Name             string  `json:"name,omitempty"`
	XMm              int     `json:"x_mm"`
	YMm              int     `json:"y_mm"`
	WidthMm          int     `json:"width_mm"`
	HeightMm         int     `json:"height_mm"`
	RotationDeg      float64 `json:"rotation_deg"`
	Shape            string  `json:"shape"`
	MinCapacity      *int    `json:"min_capacity,omitempty"`
	MaxCapacity      *int    `json:"max_capacity,omitempty"`
	VisibleSeatCount *int    `json:"visible_seat_count,omitempty"`
	RegionID         *uint   `json:"region_id,omitempty"`
	IsReservable     *bool   `json:"is_reservable,omitempty"`
	IsCombinable     *bool   `json:"is_combinable,omitempty"`
	IsAccessible     *bool   `json:"is_accessible,omitempty"`
}

// IsCandidate reports whether this placement is not yet linked to a tables row.
func (t LayoutTable) IsCandidate() bool {
	return t.TableID == 0
}

// ParseLayoutDocument unmarshals layout JSON. Empty/null becomes an empty v1 doc.
func ParseLayoutDocument(raw []byte) (*LayoutDocument, error) {
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return &LayoutDocument{SchemaVersion: LayoutSchemaVersion}, nil
	}
	var doc LayoutDocument
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("parse layout document: %w", err)
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = LayoutSchemaVersion
	}
	return &doc, nil
}

// MarshalLayoutDocument serializes a layout document to JSON.
func MarshalLayoutDocument(doc *LayoutDocument) ([]byte, error) {
	if doc == nil {
		return []byte(`{}`), nil
	}
	if doc.SchemaVersion == 0 {
		doc.SchemaVersion = LayoutSchemaVersion
	}
	return json.Marshal(doc)
}

// ValidationIssue is a single layout validation problem or warning.
type ValidationIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
}

// ValidationResult holds layout validation outcome.
// Hard Issues set Valid=false; Warnings never block draft/publish.
type ValidationResult struct {
	Valid    bool              `json:"valid"`
	Issues   []ValidationIssue `json:"issues,omitempty"`
	Warnings []ValidationIssue `json:"warnings,omitempty"`
}

func (r *ValidationResult) add(code, path, message string) {
	r.Valid = false
	r.Issues = append(r.Issues, ValidationIssue{Code: code, Path: path, Message: message})
}

func (r *ValidationResult) warn(code, path, message string) {
	r.Warnings = append(r.Warnings, ValidationIssue{Code: code, Path: path, Message: message})
}

// ValidateLayoutDocument validates with draft rules (candidates allowed, soft geometry).
func ValidateLayoutDocument(doc *LayoutDocument) ValidationResult {
	return ValidateLayoutDocumentMode(doc, ValidateDraft)
}

// ValidateLayoutDocumentMode validates a layout for draft or publish.
func ValidateLayoutDocumentMode(doc *LayoutDocument, mode ValidationMode) ValidationResult {
	res := ValidationResult{Valid: true}
	if doc == nil {
		res.add("empty", "", "layout document is nil")
		return res
	}
	if doc.SchemaVersion != 0 && doc.SchemaVersion != LayoutSchemaVersion {
		res.add("schema_version", "schema_version",
			fmt.Sprintf("unsupported layout schema_version %d (expected %d)", doc.SchemaVersion, LayoutSchemaVersion))
	}
	if doc.WidthMm < 0 || doc.HeightMm < 0 {
		res.add("dimensions", "width_mm/height_mm", "width_mm and height_mm must be non-negative")
	}
	if doc.Boundary != nil && len(doc.Boundary.PointsMm) > 0 {
		if err := geometry.ValidateBoundary(doc.Boundary.PointsMm); err != nil {
			res.add("boundary", "boundary", err.Error())
		}
	}
	for i, region := range doc.Regions {
		path := fmt.Sprintf("regions[%d]", i)
		if region.Name == "" {
			res.add("region_name", path+".name", "region name is required")
		}
		if len(region.PolygonMm) > 0 {
			if err := geometry.ValidatePolygon(region.PolygonMm); err != nil {
				res.add("region_polygon", path+".polygon_mm", err.Error())
			}
		}
	}
	validShapes := map[string]bool{
		"": true, "round": true, "square": true, "rectangle": true,
		"oval": true, "bar": true, "custom": true,
	}
	validElements := map[string]bool{
		"wall": true, "door": true, "window": true, "column": true,
		"bar": true, "counter": true, "entrance": true, "stairs": true,
		"service_station": true, "restroom": true, "divider": true,
		"obstacle": true, "label": true,
	}
	for i, el := range doc.Elements {
		path := fmt.Sprintf("elements[%d]", i)
		if el.ElementType == "" || !validElements[el.ElementType] {
			res.add("element_type", path+".element_type",
				fmt.Sprintf("invalid element_type %q", el.ElementType))
		}
	}
	// Table placement validation + pairwise intersection.
	seenIDs := map[uint]int{}
	rects := make([]geometry.OrientedRect, 0, len(doc.Tables))
	for i, tbl := range doc.Tables {
		path := fmt.Sprintf("tables[%d]", i)
		if tbl.TableID == 0 {
			if mode == ValidatePublish {
				res.add("table_id", path+".table_id",
					"table_id is required for publish (materialize candidates first)")
			}
			// Draft: candidate allowed — no hard error.
		} else {
			if prev, ok := seenIDs[tbl.TableID]; ok {
				res.add("duplicate_table", path+".table_id",
					fmt.Sprintf("table_id %d already appears at tables[%d]", tbl.TableID, prev))
			}
			seenIDs[tbl.TableID] = i
		}
		if tbl.WidthMm <= 0 || tbl.HeightMm <= 0 {
			res.add("table_size", path, "table width_mm and height_mm must be positive")
		}
		if !validShapes[tbl.Shape] {
			res.add("table_shape", path+".shape", fmt.Sprintf("invalid shape %q", tbl.Shape))
		}
		// L3-24: capacity invariants gate PUBLISH, not draft.
		//
		// Layouts saved before this rule (and rows the CRUD side never clamped)
		// can legitimately hold an out-of-range capacity. Hard-failing the draft
		// path would 400 every autosave and leave those editors with no reachable
		// valid state, so draft only warns; publish is where it blocks.
		capacityIssue := res.add
		if mode != ValidatePublish {
			capacityIssue = res.warn
		}
		if tbl.MaxCapacity != nil {
			if *tbl.MaxCapacity < 1 || *tbl.MaxCapacity > 99 {
				capacityIssue("table_capacity", path+".max_capacity",
					"max_capacity must be between 1 and 99")
			}
		}
		if tbl.MinCapacity != nil {
			if *tbl.MinCapacity < 1 || *tbl.MinCapacity > 99 {
				capacityIssue("table_capacity", path+".min_capacity",
					"min_capacity must be between 1 and 99")
			}
		}
		if tbl.MinCapacity != nil && tbl.MaxCapacity != nil &&
			*tbl.MinCapacity > *tbl.MaxCapacity {
			capacityIssue("table_capacity", path,
				"min_capacity cannot exceed max_capacity")
		}
		// Soft: out-of-bounds and overlaps never block save/publish.
		// Use axis-aligned bounding box for all rotations (oriented OBB soft-warn
		// is still covered by overlap checks; AABB warns when the unrotated
		// extent exits the room).
		if doc.WidthMm > 0 && doc.HeightMm > 0 {
			if tbl.XMm < 0 || tbl.YMm < 0 ||
				tbl.XMm+tbl.WidthMm > doc.WidthMm ||
				tbl.YMm+tbl.HeightMm > doc.HeightMm {
				res.warn("table_bounds", path, "table placement exceeds space dimensions")
			}
		}
		rects = append(rects, geometry.OrientedRect{
			X: float64(tbl.XMm), Y: float64(tbl.YMm),
			W: float64(tbl.WidthMm), H: float64(tbl.HeightMm),
			RotationDeg: tbl.RotationDeg,
		})
	}
	for i := 0; i < len(rects); i++ {
		for j := i + 1; j < len(rects); j++ {
			if geometry.TableIntersects(rects[i], rects[j]) {
				idA, idB := doc.Tables[i].TableID, doc.Tables[j].TableID
				labelA, labelB := fmt.Sprintf("%d", idA), fmt.Sprintf("%d", idB)
				if idA == 0 {
					labelA = "candidate"
				}
				if idB == 0 {
					labelB = "candidate"
				}
				res.warn("table_overlap",
					fmt.Sprintf("tables[%d]/tables[%d]", i, j),
					fmt.Sprintf("tables %s and %s overlap", labelA, labelB))
			}
		}
	}
	return res
}

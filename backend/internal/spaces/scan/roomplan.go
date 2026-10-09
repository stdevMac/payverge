package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/stdevmac/payverge/backend/internal/spaces"
	"github.com/stdevmac/payverge/backend/internal/spaces/geometry"
)

// RoomPlanProcessor converts Apple RoomPlan (or RoomPlan-like) structured JSON
// into a layout schema v1 draft. Coordinates are converted from meters to mm
// when the payload declares meters (default).
type RoomPlanProcessor struct{}

// roomPlanPayload is a tolerant subset of RoomPlan export JSON.
type roomPlanPayload struct {
	// Unit is "m" or "mm"; defaults to "m" (RoomPlan uses meters).
	Unit string `json:"unit"`
	// Walls are wall segments in capture coordinates.
	Walls []roomPlanWall `json:"walls"`
	// Objects are furniture / table-like objects.
	Objects []roomPlanObject `json:"objects"`
	// Floors optional floor polygons.
	Floors []roomPlanFloor `json:"floors"`
	// Dimensions optional explicit room size in payload units.
	Dimensions *struct {
		Width  float64 `json:"width"`
		Height float64 `json:"height"` // depth on floor plan → height_mm in layout
		Length float64 `json:"length"` // alias for height when Height unset
	} `json:"dimensions"`
	// Confidence optional overall confidence 0-1.
	Confidence *float64 `json:"confidence"`
}

type roomPlanWall struct {
	// Dimensions: length × thickness in payload units; transform gives pose.
	Dimensions []float64 `json:"dimensions"` // [length, height, thickness] or [length, thickness]
	// Transform is a 4x4 row-major matrix; translation in last column/row variants supported.
	Transform []float64 `json:"transform"`
	// Start/End optional explicit endpoints when transform absent.
	Start    *geometry.Point `json:"start"`
	End      *geometry.Point `json:"end"`
	Category string          `json:"category"`
}

type roomPlanObject struct {
	Category   string    `json:"category"` // table, chair, storage, etc.
	Dimensions []float64 `json:"dimensions"`
	Transform  []float64 `json:"transform"`
	// Center optional when transform absent.
	Center     *geometry.Point `json:"center"`
	Confidence float64         `json:"confidence"`
	Label      string          `json:"label"`
}

type roomPlanFloor struct {
	// Polygon vertices in payload units.
	Polygon []geometry.Point `json:"polygon"`
}

// Process implements SpaceScanProcessor.
func (RoomPlanProcessor) Process(ctx context.Context, input ProcessInput) (*DraftLayoutResult, error) {
	_ = ctx
	if len(input.Payload) == 0 {
		return nil, fmt.Errorf("roomplan payload is empty")
	}
	var payload roomPlanPayload
	if err := json.Unmarshal(input.Payload, &payload); err != nil {
		return nil, fmt.Errorf("parse roomplan json: %w", err)
	}

	unit := payload.Unit
	if unit == "" {
		unit = "m"
	}
	toMM := 1000.0
	if unit == "mm" {
		toMM = 1
	} else if unit == "ft" {
		toMM = 304.8
	}

	// Collect points for normalization / bounds.
	var allPts []geometry.Point
	for _, fl := range payload.Floors {
		for _, p := range fl.Polygon {
			allPts = append(allPts, geometry.Point{X: p.X * toMM, Y: p.Y * toMM})
		}
	}
	for _, w := range payload.Walls {
		if w.Start != nil && w.End != nil {
			allPts = append(allPts,
				geometry.Point{X: w.Start.X * toMM, Y: w.Start.Y * toMM},
				geometry.Point{X: w.End.X * toMM, Y: w.End.Y * toMM},
			)
		}
		if cx, cy, ok := transformTranslation(w.Transform); ok {
			allPts = append(allPts, geometry.Point{X: cx * toMM, Y: cy * toMM})
		}
	}
	for _, o := range payload.Objects {
		if o.Center != nil {
			allPts = append(allPts, geometry.Point{X: o.Center.X * toMM, Y: o.Center.Y * toMM})
		}
		if cx, cy, ok := transformTranslation(o.Transform); ok {
			allPts = append(allPts, geometry.Point{X: cx * toMM, Y: cy * toMM})
		}
	}

	var widthMM, heightMM float64
	var originShift geometry.Point
	if len(allPts) > 0 {
		norm, w, h, _ := geometry.NormalizeScanToLayout(allPts, geometry.Point{}, geometry.Point{}, 0)
		// Normalize already scaled; we pre-multiplied by toMM so scale=1.
		// Recompute min to shift other objects.
		minX, minY := allPts[0].X, allPts[0].Y
		for _, p := range allPts[1:] {
			if p.X < minX {
				minX = p.X
			}
			if p.Y < minY {
				minY = p.Y
			}
		}
		originShift = geometry.Point{X: minX, Y: minY}
		widthMM, heightMM = w, h
		_ = norm
	}
	if payload.Dimensions != nil {
		dw := payload.Dimensions.Width * toMM
		dh := payload.Dimensions.Height * toMM
		if dh == 0 {
			dh = payload.Dimensions.Length * toMM
		}
		if dw > widthMM {
			widthMM = dw
		}
		if dh > heightMM {
			heightMM = dh
		}
	}
	// Calibration override from session.
	if input.CalibrationMm != nil && *input.CalibrationMm > 0 && widthMM > 0 {
		// Already in mm; leave as-is. Calibration is applied by keyframe path more often.
	}

	// Boundary from first floor polygon or axis-aligned box.
	var boundaryPts []geometry.Point
	if len(payload.Floors) > 0 && len(payload.Floors[0].Polygon) >= 3 {
		for _, p := range payload.Floors[0].Polygon {
			boundaryPts = append(boundaryPts, geometry.Point{
				X: p.X*toMM - originShift.X,
				Y: p.Y*toMM - originShift.Y,
			})
		}
	} else {
		w := int(math.Max(widthMM, 1000))
		h := int(math.Max(heightMM, 1000))
		widthMM, heightMM = float64(w), float64(h)
		boundaryPts = []geometry.Point{
			{X: 0, Y: 0}, {X: float64(w), Y: 0}, {X: float64(w), Y: float64(h)}, {X: 0, Y: float64(h)},
		}
	}

	doc := layoutFromBoundary(boundaryPts, int(math.Ceil(widthMM)), int(math.Ceil(heightMM)), "mm")

	// Walls → elements.
	for i, w := range payload.Walls {
		el := spaces.LayoutElement{
			ElementType: "wall",
			Name:        fmt.Sprintf("Wall %d", i+1),
			ZIndex:      0,
			Meta:        map[string]interface{}{"source": "roomplan"},
		}
		if w.Start != nil && w.End != nil {
			x1 := w.Start.X*toMM - originShift.X
			y1 := w.Start.Y*toMM - originShift.Y
			x2 := w.End.X*toMM - originShift.X
			y2 := w.End.Y*toMM - originShift.Y
			minX, maxX := math.Min(x1, x2), math.Max(x1, x2)
			minY, maxY := math.Min(y1, y2), math.Max(y1, y2)
			xi, yi := int(minX), int(minY)
			wi, hi := int(math.Max(maxX-minX, 50)), int(math.Max(maxY-minY, 50))
			el.XMm, el.YMm = &xi, &yi
			el.WidthMm, el.HeightMm = &wi, &hi
			el.Geometry = map[string]interface{}{
				"points_mm": []geometry.Point{{X: x1, Y: y1}, {X: x2, Y: y2}},
			}
		} else if cx, cy, ok := transformTranslation(w.Transform); ok {
			xi := int(cx*toMM - originShift.X)
			yi := int(cy*toMM - originShift.Y)
			length := 1000.0
			thickness := 100.0
			if len(w.Dimensions) >= 1 {
				length = w.Dimensions[0] * toMM
			}
			if len(w.Dimensions) >= 3 {
				thickness = w.Dimensions[2] * toMM
			} else if len(w.Dimensions) >= 2 {
				thickness = w.Dimensions[1] * toMM
			}
			li, ti := int(length), int(math.Max(thickness, 50))
			el.XMm, el.YMm = &xi, &yi
			el.WidthMm, el.HeightMm = &li, &ti
		}
		doc.Elements = append(doc.Elements, el)
	}

	// Table-like objects → layout tables (table_id=0 means unlinked candidates).
	tableIdx := 0
	for _, o := range payload.Objects {
		if !isTableCategory(o.Category) {
			continue
		}
		tableIdx++
		cx, cy := 0.0, 0.0
		if o.Center != nil {
			cx, cy = o.Center.X*toMM, o.Center.Y*toMM
		} else if tx, ty, ok := transformTranslation(o.Transform); ok {
			cx, cy = tx*toMM, ty*toMM
		}
		w, h := 800.0, 800.0
		if len(o.Dimensions) >= 2 {
			w = o.Dimensions[0] * toMM
			h = o.Dimensions[1] * toMM
		}
		x := int(cx - originShift.X - w/2)
		y := int(cy - originShift.Y - h/2)
		name := o.Label
		if name == "" {
			name = fmt.Sprintf("Table %d", tableIdx)
		}
		doc.Tables = append(doc.Tables, spaces.LayoutTable{
			TableID:  0, // unlinked; AssignLegacyTables / editor binds later
			Name:     name,
			XMm:      x,
			YMm:      y,
			WidthMm:  int(math.Max(w, 400)),
			HeightMm: int(math.Max(h, 400)),
			Shape:    "rectangle",
		})
	}

	conf := 0.85
	if payload.Confidence != nil {
		conf = *payload.Confidence
	}
	// RoomPlan is metric when unit is known; still mark confidence < 1.
	approximate := unit != "m" && unit != "mm"
	if conf < 0.5 {
		approximate = true
	}
	finalizeLayout(doc, approximate, conf, "roomplan")

	notes := []string{"processed RoomPlan structured JSON"}
	if tableIdx == 0 {
		notes = append(notes, "no table objects detected; place tables manually or assign legacy tables")
	}
	return &DraftLayoutResult{
		Layout:      doc,
		Approximate: approximate,
		Confidence:  conf,
		Source:      "roomplan",
		Notes:       notes,
	}, nil
}

func isTableCategory(cat string) bool {
	switch cat {
	case "table", "diningTable", "dining_table", "Table", "furniture.table":
		return true
	default:
		return false
	}
}

// transformTranslation extracts X/Y translation from a 4x4 matrix (row- or column-major).
func transformTranslation(m []float64) (x, y float64, ok bool) {
	if len(m) < 16 {
		// 3-vector translation fallback.
		if len(m) >= 2 {
			return m[0], m[1], true
		}
		return 0, 0, false
	}
	// Column-major (common in SceneKit): translation at m[12], m[13].
	// Row-major: translation at m[3], m[7].
	// Prefer column-major when those slots look like translation (larger magnitude than scale off-diagonals).
	cx, cy := m[12], m[13]
	rx, ry := m[3], m[7]
	// Heuristic: if column-major translation is non-zero or row-major is zero, use column-major.
	if math.Abs(cx)+math.Abs(cy) >= math.Abs(rx)+math.Abs(ry) {
		return cx, cy, true
	}
	return rx, ry, true
}

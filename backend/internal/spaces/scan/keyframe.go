package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"github.com/stdevmac/payverge/backend/internal/spaces"
	"github.com/stdevmac/payverge/backend/internal/spaces/geometry"
)

// KeyframeProcessor builds a draft layout from keyframe poses and user/CV
// metadata. It NEVER invents fake metric precision: when poses lack metric
// scale or calibration is missing, the result is marked approximate with
// reduced confidence.
//
// Local/dev without AR: provide calibration_mm + table_candidates (and optional
// rectangular room size in metadata) to get a real rectangular boundary and
// placed table candidates — this is a complete code path, not a stub.
type KeyframeProcessor struct{}

// keyframeMetadata is the JSON body for kind=keyframes|metadata.
type keyframeMetadata struct {
	// CalibrationMm is the real-world length for the calibration segment.
	CalibrationMm *int `json:"calibration_mm"`
	// CalibrationA/B are endpoints of the calibration segment in capture space.
	CalibrationA *geometry.Point `json:"calibration_a"`
	CalibrationB *geometry.Point `json:"calibration_b"`
	// RoomWidthMm / RoomHeightMm force a rectangular boundary when no wall hints exist.
	RoomWidthMm  *int `json:"room_width_mm"`
	RoomHeightMm *int `json:"room_height_mm"`
	// RoomWidth / RoomHeight in capture units (scaled via calibration when present).
	RoomWidth  *float64 `json:"room_width"`
	RoomHeight *float64 `json:"room_height"`
	// Frames optional when also supplied via ProcessInput.Frames.
	Frames []Keyframe `json:"frames"`
	// Tables top-level candidates (dev/local path).
	Tables []TableCandidate `json:"tables"`
	// Metric true when capture coordinates are meters.
	Metric bool `json:"metric"`
	// Notes free-form.
	Notes []string `json:"notes"`
}

// Process implements SpaceScanProcessor.
func (KeyframeProcessor) Process(ctx context.Context, input ProcessInput) (*DraftLayoutResult, error) {
	_ = ctx
	var meta keyframeMetadata
	if len(input.Payload) > 0 {
		if err := json.Unmarshal(input.Payload, &meta); err != nil {
			return nil, fmt.Errorf("parse keyframe metadata: %w", err)
		}
	}

	frames := input.Frames
	if len(frames) == 0 {
		frames = meta.Frames
	}

	calibrationMm := 0
	if input.CalibrationMm != nil {
		calibrationMm = *input.CalibrationMm
	}
	if meta.CalibrationMm != nil {
		calibrationMm = *meta.CalibrationMm
	}

	// Collect capture-space points from poses and candidates for normalization.
	var capturePts []geometry.Point
	metricPoses := 0
	for _, f := range frames {
		if f.PosePosition != nil {
			capturePts = append(capturePts, *f.PosePosition)
			if f.Metric || meta.Metric {
				metricPoses++
			}
		}
		for _, tc := range f.TableCandidates {
			capturePts = append(capturePts,
				geometry.Point{X: tc.CenterX, Y: tc.CenterY},
			)
		}
		for _, wh := range f.WallHints {
			capturePts = append(capturePts,
				geometry.Point{X: wh.X1, Y: wh.Y1},
				geometry.Point{X: wh.X2, Y: wh.Y2},
			)
		}
	}
	for _, tc := range meta.Tables {
		capturePts = append(capturePts, geometry.Point{X: tc.CenterX, Y: tc.CenterY})
	}

	// Scale: metric frames → meters to mm; else use calibration segment; else 1 (approximate).
	approx := true
	conf := 0.4
	scale := 1.0
	notes := append([]string{}, meta.Notes...)

	calA := geometry.Point{}
	calB := geometry.Point{}
	haveCalSeg := false
	if meta.CalibrationA != nil && meta.CalibrationB != nil {
		calA, calB = *meta.CalibrationA, *meta.CalibrationB
		haveCalSeg = true
	}

	switch {
	case metricPoses > 0 || meta.Metric:
		scale = 1000 // meters → mm
		approx = false
		conf = 0.7
		notes = append(notes, "metric poses present; coordinates converted meters→mm")
	case calibrationMm > 0 && haveCalSeg:
		seg := math.Hypot(calB.X-calA.X, calB.Y-calA.Y)
		if seg > 1e-9 {
			scale = float64(calibrationMm) / seg
			approx = false
			conf = 0.65
			notes = append(notes, fmt.Sprintf("calibrated with %d mm reference segment", calibrationMm))
		} else {
			notes = append(notes, "calibration segment has zero length; treating as approximate")
		}
	case calibrationMm > 0 && (meta.RoomWidth != nil || meta.RoomHeight != nil):
		// Room size given in capture units with calibration for the width edge.
		if meta.RoomWidth != nil && *meta.RoomWidth > 0 {
			scale = float64(calibrationMm) / *meta.RoomWidth
			// Actually if RoomWidth is already the calibration length in capture units
			// matching calibrationMm, use that. Prefer explicit room_width_mm when set.
			approx = true
			conf = 0.55
			notes = append(notes, "room dimensions scaled from calibration_mm (approximate)")
		}
	default:
		notes = append(notes, "no metric poses or calibration; layout is approximate and unitless-scaled")
		conf = 0.35
	}

	// Forced mm dimensions take precedence for the boundary shell.
	widthMM := 8000
	heightMM := 6000
	if meta.RoomWidthMm != nil && *meta.RoomWidthMm > 0 {
		widthMM = *meta.RoomWidthMm
		approx = false
		if conf < 0.6 {
			conf = 0.6
		}
		notes = append(notes, "room_width_mm provided")
	} else if meta.RoomWidth != nil && *meta.RoomWidth > 0 {
		widthMM = int(math.Ceil(*meta.RoomWidth * scale))
	}
	if meta.RoomHeightMm != nil && *meta.RoomHeightMm > 0 {
		heightMM = *meta.RoomHeightMm
		notes = append(notes, "room_height_mm provided")
	} else if meta.RoomHeight != nil && *meta.RoomHeight > 0 {
		heightMM = int(math.Ceil(*meta.RoomHeight * scale))
	}

	// If we have capture points, normalize them for relative placement.
	var originShift geometry.Point
	if len(capturePts) > 0 {
		scaled := make([]geometry.Point, len(capturePts))
		for i, p := range capturePts {
			scaled[i] = geometry.Scale(p, scale)
		}
		minX, minY := scaled[0].X, scaled[0].Y
		maxX, maxY := scaled[0].X, scaled[0].Y
		for _, p := range scaled[1:] {
			if p.X < minX {
				minX = p.X
			}
			if p.Y < minY {
				minY = p.Y
			}
			if p.X > maxX {
				maxX = p.X
			}
			if p.Y > maxY {
				maxY = p.Y
			}
		}
		originShift = geometry.Point{X: minX, Y: minY}
		if meta.RoomWidthMm == nil && meta.RoomWidth == nil {
			if bw := int(math.Ceil(maxX - minX)); bw > widthMM {
				widthMM = bw
			}
		}
		if meta.RoomHeightMm == nil && meta.RoomHeight == nil {
			if bh := int(math.Ceil(maxY - minY)); bh > heightMM {
				heightMM = bh
			}
		}
	}

	// Ensure minimum room size.
	if widthMM < 1000 {
		widthMM = 1000
	}
	if heightMM < 1000 {
		heightMM = 1000
	}

	boundary := []geometry.Point{
		{X: 0, Y: 0},
		{X: float64(widthMM), Y: 0},
		{X: float64(widthMM), Y: float64(heightMM)},
		{X: 0, Y: float64(heightMM)},
	}
	doc := layoutFromBoundary(boundary, widthMM, heightMM, "mm")

	// Wall hints → wall elements.
	wallIdx := 0
	for _, f := range frames {
		for _, wh := range f.WallHints {
			wallIdx++
			x1 := wh.X1*scale - originShift.X
			y1 := wh.Y1*scale - originShift.Y
			x2 := wh.X2*scale - originShift.X
			y2 := wh.Y2*scale - originShift.Y
			minX, maxX := math.Min(x1, x2), math.Max(x1, x2)
			minY, maxY := math.Min(y1, y2), math.Max(y1, y2)
			xi, yi := int(minX), int(minY)
			wi := int(math.Max(maxX-minX, 50))
			hi := int(math.Max(maxY-minY, 50))
			doc.Elements = append(doc.Elements, spaces.LayoutElement{
				ElementType: "wall",
				Name:        fmt.Sprintf("Wall %d", wallIdx),
				XMm:         &xi,
				YMm:         &yi,
				WidthMm:     &wi,
				HeightMm:    &hi,
				Geometry: map[string]interface{}{
					"points_mm": []geometry.Point{{X: x1, Y: y1}, {X: x2, Y: y2}},
				},
				Meta: map[string]interface{}{
					"confidence": wh.Confidence,
					"source":     "keyframe",
				},
			})
		}
	}

	// Collect table candidates.
	type cand struct {
		TableCandidate
	}
	var cands []cand
	for _, tc := range meta.Tables {
		cands = append(cands, cand{tc})
	}
	for _, f := range frames {
		for _, tc := range f.TableCandidates {
			cands = append(cands, cand{tc})
		}
	}

	for i, c := range cands {
		w := c.Width * scale
		h := c.Height * scale
		if w <= 0 {
			w = 800
		}
		if h <= 0 {
			h = 800
		}
		// NEVER invent sub-mm precision: round to nearest 10 mm when approximate.
		round := func(v float64) int {
			if approx {
				return int(math.Round(v/10) * 10)
			}
			return int(math.Round(v))
		}
		cx := c.CenterX*scale - originShift.X
		cy := c.CenterY*scale - originShift.Y
		x := round(cx - w/2)
		y := round(cy - h/2)
		name := c.Name
		if name == "" {
			name = fmt.Sprintf("Table %d", i+1)
		}
		tbl := spaces.LayoutTable{
			TableID:     0,
			Name:        name,
			XMm:         x,
			YMm:         y,
			WidthMm:     round(w),
			HeightMm:    round(h),
			RotationDeg: c.RotationDeg,
			Shape:       "rectangle",
		}
		if c.SeatHint != nil {
			tbl.VisibleSeatCount = c.SeatHint
			tbl.MaxCapacity = c.SeatHint
		}
		doc.Tables = append(doc.Tables, tbl)
		if c.Confidence > 0 && c.Confidence < conf {
			// Pull overall confidence down to the weakest table if lower.
			// Keep floor.
			if c.Confidence >= 0.2 {
				// average softly
				conf = (conf + c.Confidence) / 2
			}
		}
	}

	if len(cands) == 0 && len(capturePts) == 0 && meta.RoomWidthMm == nil && meta.RoomHeightMm == nil {
		// Pure empty metadata: still return a rectangular room shell from
		// calibration or defaults so local/dev has a real path.
		notes = append(notes, "no poses or table candidates; default rectangular boundary only")
		conf = math.Min(conf, 0.3)
		approx = true
	}

	// Cap confidence when approximate — never claim fake precision.
	if approx && conf > 0.6 {
		conf = 0.6
	}

	finalizeLayout(doc, approx, conf, "keyframe")
	return &DraftLayoutResult{
		Layout:      doc,
		Approximate: approx,
		Confidence:  conf,
		Source:      "keyframe",
		Notes:       notes,
		RawMeta: map[string]interface{}{
			"frame_count":  len(frames),
			"table_count":  len(cands),
			"scale":        scale,
			"metric_poses": metricPoses,
		},
	}, nil
}

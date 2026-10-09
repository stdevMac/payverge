package scan

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/spaces/geometry"
)

func TestRoomPlanProcessor_Basic(t *testing.T) {
	payload := map[string]interface{}{
		"unit": "m",
		"dimensions": map[string]float64{
			"width":  10,
			"height": 8,
		},
		"floors": []map[string]interface{}{
			{"polygon": []map[string]float64{
				{"x": 0, "y": 0}, {"x": 10, "y": 0}, {"x": 10, "y": 8}, {"x": 0, "y": 8},
			}},
		},
		"walls": []map[string]interface{}{
			{"start": map[string]float64{"x": 0, "y": 0}, "end": map[string]float64{"x": 10, "y": 0}},
		},
		"objects": []map[string]interface{}{
			{
				"category":   "table",
				"label":      "T1",
				"center":     map[string]float64{"x": 2, "y": 3},
				"dimensions": []float64{0.8, 0.8},
			},
		},
		"confidence": 0.9,
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	res, err := (RoomPlanProcessor{}).Process(context.Background(), ProcessInput{
		BusinessID: 1,
		Payload:    raw,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Layout)
	assert.Equal(t, "roomplan", res.Source)
	assert.False(t, res.Approximate)
	assert.InDelta(t, 0.9, res.Confidence, 0.01)
	assert.Equal(t, 10000, res.Layout.WidthMm)
	assert.Equal(t, 8000, res.Layout.HeightMm)
	require.Len(t, res.Layout.Tables, 1)
	assert.Equal(t, "T1", res.Layout.Tables[0].Name)
	assert.Equal(t, 800, res.Layout.Tables[0].WidthMm)
	require.NotEmpty(t, res.Layout.Elements)
}

func TestKeyframeProcessor_DevPathWithCalibration(t *testing.T) {
	// Local/dev without AR: rectangular room + table candidates from metadata.
	cal := 5000
	payload := map[string]interface{}{
		"calibration_mm": cal,
		"room_width_mm":  10000,
		"room_height_mm": 7000,
		"tables": []map[string]interface{}{
			{
				"name":       "Bar-side",
				"center_x":   2500,
				"center_y":   2000,
				"width":      800,
				"height":     800,
				"seat_hint":  4,
				"confidence": 0.8,
			},
			{
				"name":       "Window",
				"center_x":   7000,
				"center_y":   4000,
				"width":      1200,
				"height":     800,
				"seat_hint":  6,
				"confidence": 0.7,
			},
		},
	}
	raw, err := json.Marshal(payload)
	require.NoError(t, err)

	res, err := (KeyframeProcessor{}).Process(context.Background(), ProcessInput{
		BusinessID:    1,
		Payload:       raw,
		CalibrationMm: &cal,
	})
	require.NoError(t, err)
	require.NotNil(t, res.Layout)
	assert.Equal(t, "keyframe", res.Source)
	assert.Equal(t, 10000, res.Layout.WidthMm)
	assert.Equal(t, 7000, res.Layout.HeightMm)
	require.Len(t, res.Layout.Tables, 2)
	assert.Equal(t, "Bar-side", res.Layout.Tables[0].Name)
	// When room_*_mm is provided, scale for table coords from capture is still
	// applied; with room_width_mm and center already in mm-ish units without
	// metric flag, centers use scale=1 (approx path) or calibration.
	assert.True(t, res.Layout.Tables[0].WidthMm > 0)
	assert.NotNil(t, res.Layout.Meta["approximate"])
}

func TestKeyframeProcessor_NeverFakesPrecision(t *testing.T) {
	// No calibration, no metric, only unitless candidates.
	payload := map[string]interface{}{
		"tables": []map[string]interface{}{
			{"center_x": 1.234567, "center_y": 2.345678, "width": 0.8, "height": 0.8, "confidence": 0.5},
		},
	}
	raw, _ := json.Marshal(payload)
	res, err := (KeyframeProcessor{}).Process(context.Background(), ProcessInput{Payload: raw})
	require.NoError(t, err)
	assert.True(t, res.Approximate, "must mark approximate without metric/calibration")
	assert.LessOrEqual(t, res.Confidence, 0.6)
	// Positions rounded to 10mm grid when approximate.
	require.Len(t, res.Layout.Tables, 1)
	assert.Equal(t, 0, res.Layout.Tables[0].XMm%10)
	assert.Equal(t, 0, res.Layout.Tables[0].YMm%10)
}

func TestKeyframeProcessor_MetricPoses(t *testing.T) {
	pos := geometry.Point{X: 0, Y: 0}
	pos2 := geometry.Point{X: 5, Y: 4}
	res, err := (KeyframeProcessor{}).Process(context.Background(), ProcessInput{
		Frames: []Keyframe{
			{Index: 0, PosePosition: &pos, Metric: true},
			{Index: 1, PosePosition: &pos2, Metric: true, TableCandidates: []TableCandidate{
				{CenterX: 2, CenterY: 2, Width: 0.9, Height: 0.9, Confidence: 0.8, Name: "A"},
			}},
		},
	})
	require.NoError(t, err)
	assert.False(t, res.Approximate)
	assert.GreaterOrEqual(t, res.Confidence, 0.6)
	require.Len(t, res.Layout.Tables, 1)
	// 0.9 m → 900 mm
	assert.Equal(t, 900, res.Layout.Tables[0].WidthMm)
}

func TestProcessorForKind(t *testing.T) {
	p, err := ProcessorForKind("roomplan_json")
	require.NoError(t, err)
	assert.IsType(t, RoomPlanProcessor{}, p)

	p, err = ProcessorForKind("keyframes")
	require.NoError(t, err)
	assert.IsType(t, KeyframeProcessor{}, p)

	_, err = ProcessorForKind("video")
	assert.Error(t, err)
}

package geometry

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestScaleAndRotate(t *testing.T) {
	p := Point{X: 10, Y: 0}
	assert.Equal(t, Point{X: 20, Y: 0}, Scale(p, 2))

	// 90° clockwise in Y-down layout: +X (right) → +Y (down).
	got := RotatePoint(Point{X: 10, Y: 0}, Point{X: 0, Y: 0}, 90)
	assert.InDelta(t, 0, got.X, 1e-9)
	assert.InDelta(t, 10, got.Y, 1e-9)

	// 180° about origin.
	got = RotatePoint(Point{X: 5, Y: 3}, Point{X: 0, Y: 0}, 180)
	assert.InDelta(t, -5, got.X, 1e-9)
	assert.InDelta(t, -3, got.Y, 1e-9)

	// 90° from down should go left.
	got = RotatePoint(Point{X: 0, Y: 10}, Point{X: 0, Y: 0}, 90)
	assert.InDelta(t, -10, got.X, 1e-9)
	assert.InDelta(t, 0, got.Y, 1e-9)
}

func TestValidateBoundary(t *testing.T) {
	err := ValidateBoundary([]Point{{0, 0}, {10, 0}})
	assert.Error(t, err)

	err = ValidateBoundary([]Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}})
	assert.NoError(t, err)

	err = ValidateBoundary([]Point{{0, 0}, {10, 0}, {5, 0}}) // collinear zero area
	assert.Error(t, err)

	err = ValidateBoundary([]Point{{0, 0}, {math.NaN(), 1}, {1, 1}})
	assert.Error(t, err)
}

func TestValidatePolygonSelfIntersection(t *testing.T) {
	// Simple square — ok.
	err := ValidatePolygon([]Point{{0, 0}, {10, 0}, {10, 10}, {0, 10}})
	assert.NoError(t, err)

	// Bow-tie self-intersecting.
	err = ValidatePolygon([]Point{{0, 0}, {10, 10}, {10, 0}, {0, 10}})
	assert.Error(t, err)
}

func TestTableIntersects(t *testing.T) {
	a := OrientedRect{X: 0, Y: 0, W: 100, H: 100, RotationDeg: 0}
	b := OrientedRect{X: 50, Y: 50, W: 100, H: 100, RotationDeg: 0}
	assert.True(t, TableIntersects(a, b))

	c := OrientedRect{X: 200, Y: 200, W: 50, H: 50, RotationDeg: 0}
	assert.False(t, TableIntersects(a, c))

	// Rotated table that still overlaps.
	d := OrientedRect{X: 80, Y: 80, W: 40, H: 40, RotationDeg: 45}
	assert.True(t, TableIntersects(a, d))
}

func TestNormalizeScanToLayout(t *testing.T) {
	// Unit square in scan space; calibration diagonal from (0,0) to (1,0) = 1 unit = 1000 mm.
	pts := []Point{{0, 0}, {1, 0}, {1, 1}, {0, 1}}
	norm, w, h, scale := NormalizeScanToLayout(pts, Point{0, 0}, Point{1, 0}, 1000)
	assert.InDelta(t, 1000, scale, 1e-9)
	assert.InDelta(t, 1000, w, 1e-6)
	assert.InDelta(t, 1000, h, 1e-6)
	assert.InDelta(t, 0, norm[0].X, 1e-6)
	assert.InDelta(t, 0, norm[0].Y, 1e-6)
	assert.InDelta(t, 1000, norm[2].X, 1e-6)
	assert.InDelta(t, 1000, norm[2].Y, 1e-6)
}

func TestNormalizeScanToLayout_NoCalibration(t *testing.T) {
	pts := []Point{{2, 2}, {4, 2}, {4, 4}}
	norm, w, h, scale := NormalizeScanToLayout(pts, Point{}, Point{}, 0)
	assert.Equal(t, 1.0, scale)
	assert.InDelta(t, 2, w, 1e-9)
	assert.InDelta(t, 2, h, 1e-9)
	assert.InDelta(t, 0, norm[0].X, 1e-9)
	assert.InDelta(t, 0, norm[0].Y, 1e-9)
}

func TestRotatePoint_QuarterTurns(t *testing.T) {
	origin := Point{X: 0, Y: 0}
	// 270° CW: +X → -Y (up in Y-down space)
	got := RotatePoint(Point{X: 10, Y: 0}, origin, 270)
	assert.InDelta(t, 0, got.X, 1e-9)
	assert.InDelta(t, -10, got.Y, 1e-9)

	// rotate about non-origin
	got = RotatePoint(Point{X: 2, Y: 0}, Point{X: 1, Y: 0}, 90)
	assert.InDelta(t, 1, got.X, 1e-9)
	assert.InDelta(t, 1, got.Y, 1e-9)
}

func TestValidateBoundary_InfAndDuplicates(t *testing.T) {
	err := ValidateBoundary([]Point{{0, 0}, {math.Inf(1), 0}, {1, 1}})
	assert.Error(t, err)

	// consecutive duplicates collapse to zero unique vertices
	err = ValidateBoundary([]Point{{0, 0}, {0, 0}, {0, 0}})
	assert.Error(t, err)
}

func TestTableIntersects_TouchingEdges(t *testing.T) {
	a := OrientedRect{X: 0, Y: 0, W: 100, H: 100, RotationDeg: 0}
	// Touching on right edge only (SAT treats edge-touch as overlap).
	b := OrientedRect{X: 100, Y: 0, W: 50, H: 50, RotationDeg: 0}
	assert.True(t, TableIntersects(a, b))

	// Completely separate.
	c := OrientedRect{X: 101, Y: 0, W: 50, H: 50, RotationDeg: 0}
	assert.False(t, TableIntersects(a, c))
}

func TestNormalizeScanToLayout_Empty(t *testing.T) {
	norm, w, h, scale := NormalizeScanToLayout(nil, Point{}, Point{}, 1000)
	assert.Nil(t, norm)
	assert.Equal(t, 0.0, w)
	assert.Equal(t, 0.0, h)
	assert.Equal(t, 1.0, scale)
}

func TestNormalizeScanToLayout_ZeroSegmentFallsBack(t *testing.T) {
	pts := []Point{{1, 1}, {3, 1}, {3, 4}}
	// Calibration A==B → scale 1, then origin shifted.
	norm, w, h, scale := NormalizeScanToLayout(pts, Point{5, 5}, Point{5, 5}, 1000)
	assert.Equal(t, 1.0, scale)
	assert.InDelta(t, 2, w, 1e-9)
	assert.InDelta(t, 3, h, 1e-9)
	assert.InDelta(t, 0, norm[0].X, 1e-9)
	assert.InDelta(t, 0, norm[0].Y, 1e-9)
}

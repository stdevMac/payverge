// Package geometry provides pure layout geometry helpers for Payverge Spaces.
//
// Coordinate system (layout schema version 1):
//   - Origin: top-left corner of the space bounding box
//   - X axis: increases to the right
//   - Y axis: increases downward (screen / floor-plan convention)
//   - Units: millimeters for all stored coordinates and sizes
//   - Rotation: degrees clockwise from the +X axis
//
// Layout schema version 1 document shape (JSON):
//
//	{
//	  "schema_version": 1,
//	  "width_mm": 12000,
//	  "height_mm": 8000,
//	  "measurement_unit": "m",
//	  "boundary": { "points_mm": [{"x":0,"y":0}, ...], "closed": true },
//	  "regions": [
//	    { "id": 1, "name": "Patio", "color": "#AABBCC",
//	      "polygon_mm": [{"x":0,"y":0}, ...], "purpose": "outdoor" }
//	  ],
//	  "elements": [
//	    { "id": 1, "element_type": "wall", "name": "North wall",
//	      "geometry": { "points_mm": [...] },
//	      "x_mm": 0, "y_mm": 0, "width_mm": 12000, "height_mm": 150,
//	      "rotation_deg": 0, "z_index": 0, "meta": {} }
//	  ],
//	  "tables": [
//	    { "table_id": 42, "name": "T1", "x_mm": 1000, "y_mm": 2000,
//	      "width_mm": 800, "height_mm": 800, "rotation_deg": 0,
//	      "shape": "round", "min_capacity": 2, "max_capacity": 4,
//	      "visible_seat_count": 4, "region_id": null,
//	      "is_reservable": true, "is_combinable": false, "is_accessible": false }
//	  ],
//	  "meta": { "approximate": false, "confidence": 1.0, "source": "manual" }
//	}
package geometry

import (
	"fmt"
	"math"
)

// Point is a 2D point in millimeters.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Rect is an axis-aligned rectangle in millimeters (before rotation).
// X,Y is the top-left corner; W,H are width and height.
type Rect struct {
	X float64
	Y float64
	W float64
	H float64
}

// OrientedRect is a rectangle with clockwise rotation about its center.
type OrientedRect struct {
	X           float64 // top-left X before rotation (layout space)
	Y           float64 // top-left Y before rotation
	W           float64
	H           float64
	RotationDeg float64
}

// Unit represents a linear measurement unit for convert helpers.
type Unit string

const (
	UnitMillimeter Unit = "mm"
	UnitMeter      Unit = "m"
	UnitFoot       Unit = "ft"
	UnitInch       Unit = "in"
)

// Scale multiplies a point by a uniform scale factor about the origin.
func Scale(p Point, factor float64) Point {
	return Point{X: p.X * factor, Y: p.Y * factor}
}

// RotatePoint rotates p about origin by degrees clockwise in the layout
// coordinate system (Y increases downward). Visual clockwise on a floor plan
// matches standard mathematical counter-clockwise in a Y-up plane, so a +90°
// turn takes +X to +Y (right → down).
func RotatePoint(p, origin Point, degreesCW float64) Point {
	rad := degreesCW * math.Pi / 180
	cos := math.Cos(rad)
	sin := math.Sin(rad)
	dx := p.X - origin.X
	dy := p.Y - origin.Y
	return Point{
		X: origin.X + dx*cos - dy*sin,
		Y: origin.Y + dx*sin + dy*cos,
	}
}

// ValidateBoundary checks that a space boundary polygon is usable.
// Requires at least 3 distinct points, finite coordinates, and non-zero area.
func ValidateBoundary(points []Point) error {
	if len(points) < 3 {
		return fmt.Errorf("boundary requires at least 3 points, got %d", len(points))
	}
	for i, p := range points {
		if math.IsNaN(p.X) || math.IsNaN(p.Y) || math.IsInf(p.X, 0) || math.IsInf(p.Y, 0) {
			return fmt.Errorf("boundary point %d has non-finite coordinates", i)
		}
	}
	// Drop consecutive duplicates for area calculation.
	clean := dedupeConsecutive(points)
	if len(clean) < 3 {
		return fmt.Errorf("boundary has fewer than 3 unique consecutive points")
	}
	area := polygonArea(clean)
	if math.Abs(area) < 1e-6 {
		return fmt.Errorf("boundary has zero area")
	}
	return nil
}

// ValidatePolygon is like ValidateBoundary but also rejects self-intersections
// for simple polygons used as regions.
func ValidatePolygon(points []Point) error {
	if err := ValidateBoundary(points); err != nil {
		return err
	}
	clean := dedupeConsecutive(points)
	if selfIntersects(clean) {
		return fmt.Errorf("polygon self-intersects")
	}
	return nil
}

// TableIntersects returns true if two oriented table rectangles overlap
// (including edge touch). Uses separating-axis theorem on rotated corners.
func TableIntersects(a, b OrientedRect) bool {
	ca := orientedCorners(a)
	cb := orientedCorners(b)
	return polygonsIntersect(ca, cb)
}

// NormalizeScanToLayout maps scan-space points (arbitrary origin/scale/unit)
// into layout millimeters with origin at top-left of the axis-aligned bounding box.
//
// calibrationLength is the real-world length of the segment between
// calibrationA and calibrationB in millimeters. When calibrationLength <= 0
// or the calibration segment has zero length, scale defaults to 1 (caller
// should mark the result approximate).
//
// Returns normalized points and the resulting width/height of the bounding box.
func NormalizeScanToLayout(
	points []Point,
	calibrationA, calibrationB Point,
	calibrationLengthMM float64,
) (normalized []Point, widthMM, heightMM float64, scale float64) {
	if len(points) == 0 {
		return nil, 0, 0, 1
	}
	scale = 1
	segLen := math.Hypot(calibrationB.X-calibrationA.X, calibrationB.Y-calibrationA.Y)
	if calibrationLengthMM > 0 && segLen > 1e-9 {
		scale = calibrationLengthMM / segLen
	}

	scaled := make([]Point, len(points))
	for i, p := range points {
		scaled[i] = Scale(p, scale)
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

	normalized = make([]Point, len(scaled))
	for i, p := range scaled {
		normalized[i] = Point{X: p.X - minX, Y: p.Y - minY}
	}
	widthMM = maxX - minX
	heightMM = maxY - minY
	return normalized, widthMM, heightMM, scale
}

// --- internals ---

func dedupeConsecutive(points []Point) []Point {
	if len(points) == 0 {
		return nil
	}
	out := []Point{points[0]}
	for i := 1; i < len(points); i++ {
		prev := out[len(out)-1]
		if math.Abs(points[i].X-prev.X) < 1e-9 && math.Abs(points[i].Y-prev.Y) < 1e-9 {
			continue
		}
		out = append(out, points[i])
	}
	// Drop closing point if equal to first.
	if len(out) > 1 {
		last := out[len(out)-1]
		if math.Abs(last.X-out[0].X) < 1e-9 && math.Abs(last.Y-out[0].Y) < 1e-9 {
			out = out[:len(out)-1]
		}
	}
	return out
}

func polygonArea(points []Point) float64 {
	n := len(points)
	var sum float64
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		sum += points[i].X*points[j].Y - points[j].X*points[i].Y
	}
	return sum / 2
}

func orientedCorners(r OrientedRect) []Point {
	cx := r.X + r.W/2
	cy := r.Y + r.H/2
	origin := Point{X: cx, Y: cy}
	corners := []Point{
		{X: r.X, Y: r.Y},
		{X: r.X + r.W, Y: r.Y},
		{X: r.X + r.W, Y: r.Y + r.H},
		{X: r.X, Y: r.Y + r.H},
	}
	if math.Abs(r.RotationDeg) < 1e-9 {
		return corners
	}
	out := make([]Point, 4)
	for i, c := range corners {
		out[i] = RotatePoint(c, origin, r.RotationDeg)
	}
	return out
}

func polygonsIntersect(a, b []Point) bool {
	// SAT: for each edge normal of both polygons, check projection overlap.
	if !satOverlap(a, b) {
		return false
	}
	return satOverlap(b, a)
}

func satOverlap(poly, other []Point) bool {
	n := len(poly)
	for i := 0; i < n; i++ {
		j := (i + 1) % n
		edge := Point{X: poly[j].X - poly[i].X, Y: poly[j].Y - poly[i].Y}
		// Perpendicular axis.
		axis := Point{X: -edge.Y, Y: edge.X}
		minA, maxA := project(poly, axis)
		minB, maxB := project(other, axis)
		if maxA < minB-1e-9 || maxB < minA-1e-9 {
			return false
		}
	}
	return true
}

func project(poly []Point, axis Point) (float64, float64) {
	min := poly[0].X*axis.X + poly[0].Y*axis.Y
	max := min
	for _, p := range poly[1:] {
		v := p.X*axis.X + p.Y*axis.Y
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}
	return min, max
}

func selfIntersects(points []Point) bool {
	n := len(points)
	if n < 4 {
		return false
	}
	for i := 0; i < n; i++ {
		a1, a2 := points[i], points[(i+1)%n]
		for j := i + 1; j < n; j++ {
			// Skip adjacent edges and the same edge.
			if j == i || (j+1)%n == i || (i+1)%n == j {
				continue
			}
			// Also skip the pair that closes the polygon with the first edge.
			if i == 0 && j == n-1 {
				continue
			}
			b1, b2 := points[j], points[(j+1)%n]
			if segmentsIntersectProper(a1, a2, b1, b2) {
				return true
			}
		}
	}
	return false
}

func segmentsIntersectProper(a1, a2, b1, b2 Point) bool {
	d1 := crossOrient(b1, b2, a1)
	d2 := crossOrient(b1, b2, a2)
	d3 := crossOrient(a1, a2, b1)
	d4 := crossOrient(a1, a2, b2)
	if ((d1 > 0 && d2 < 0) || (d1 < 0 && d2 > 0)) &&
		((d3 > 0 && d4 < 0) || (d3 < 0 && d4 > 0)) {
		return true
	}
	return false
}

func crossOrient(a, b, c Point) float64 {
	return (b.X-a.X)*(c.Y-a.Y) - (b.Y-a.Y)*(c.X-a.X)
}

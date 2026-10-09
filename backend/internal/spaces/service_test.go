package spaces

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/spaces/geometry"
)

func TestValidateLayoutDocument_OK(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       5000,
		HeightMm:      4000,
		Boundary: &LayoutBoundary{
			PointsMm: []geometry.Point{{X: 0, Y: 0}, {X: 5000, Y: 0}, {X: 5000, Y: 4000}, {X: 0, Y: 4000}},
			Closed:   true,
		},
		Tables: []LayoutTable{
			{TableID: 1, XMm: 100, YMm: 100, WidthMm: 800, HeightMm: 800, Shape: "round"},
			{TableID: 2, XMm: 2000, YMm: 100, WidthMm: 800, HeightMm: 800, Shape: "rectangle"},
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.True(t, res.Valid, "issues: %+v", res.Issues)
}

func TestValidateLayoutDocument_OverlapIsSoftWarning(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       5000,
		HeightMm:      4000,
		Tables: []LayoutTable{
			{TableID: 1, XMm: 100, YMm: 100, WidthMm: 800, HeightMm: 800, Shape: "rectangle"},
			{TableID: 2, XMm: 500, YMm: 500, WidthMm: 800, HeightMm: 800, Shape: "rectangle"},
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.True(t, res.Valid, "overlap must not block draft: issues=%+v", res.Issues)
	found := false
	for _, iss := range res.Warnings {
		if iss.Code == "table_overlap" {
			found = true
		}
	}
	assert.True(t, found, "expected table_overlap warning, got %+v", res.Warnings)
}

func TestValidateLayoutDocument_CandidateTablesAllowedInDraft(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       5000,
		HeightMm:      4000,
		Tables: []LayoutTable{
			{TableID: 0, Name: "Scan A", XMm: 100, YMm: 100, WidthMm: 800, HeightMm: 800, Shape: "rectangle"},
			{TableID: 0, Name: "Scan B", XMm: 2000, YMm: 100, WidthMm: 800, HeightMm: 800, Shape: "round"},
		},
	}
	res := ValidateLayoutDocumentMode(doc, ValidateDraft)
	assert.True(t, res.Valid, "candidates must be allowed in draft: issues=%+v", res.Issues)
	assert.True(t, doc.Tables[0].IsCandidate() && doc.Tables[1].IsCandidate())

	pub := ValidateLayoutDocumentMode(doc, ValidatePublish)
	assert.False(t, pub.Valid, "publish requires real table ids")
	found := false
	for _, iss := range pub.Issues {
		if iss.Code == "table_id" {
			found = true
		}
	}
	assert.True(t, found, "expected table_id issue on publish: %+v", pub.Issues)
}

func TestValidateLayoutDocument_DuplicateTable(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       5000,
		HeightMm:      4000,
		Tables: []LayoutTable{
			{TableID: 1, XMm: 100, YMm: 100, WidthMm: 400, HeightMm: 400, Shape: "square"},
			{TableID: 1, XMm: 2000, YMm: 100, WidthMm: 400, HeightMm: 400, Shape: "square"},
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.False(t, res.Valid)
}

func TestValidateLayoutDocument_BadBoundary(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		Boundary: &LayoutBoundary{
			PointsMm: []geometry.Point{{X: 0, Y: 0}, {X: 10, Y: 0}},
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.False(t, res.Valid)
}

func TestParseAndMarshalLayoutRoundTrip(t *testing.T) {
	raw := []byte(`{"schema_version":1,"width_mm":1000,"height_mm":800,"tables":[{"table_id":9,"x_mm":10,"y_mm":20,"width_mm":100,"height_mm":100,"shape":"round"}]}`)
	doc, err := ParseLayoutDocument(raw)
	require.NoError(t, err)
	assert.Equal(t, 1, doc.SchemaVersion)
	assert.Equal(t, uint(9), doc.Tables[0].TableID)

	out, err := MarshalLayoutDocument(doc)
	require.NoError(t, err)
	var again LayoutDocument
	require.NoError(t, json.Unmarshal(out, &again))
	assert.Equal(t, doc.WidthMm, again.WidthMm)
}

func TestConflictError_Unwrap(t *testing.T) {
	err := &ConflictError{SpaceID: 1, ExpectedRevision: 2, ActualRevision: 3}
	assert.True(t, errors.Is(err, ErrRevisionConflict))
	assert.Contains(t, err.Error(), "revision conflict")
}

func TestActivityBlockError_Unwrap(t *testing.T) {
	err := &ActivityBlockError{OpenBills: 2, OpenReservations: 1}
	assert.True(t, errors.Is(err, ErrHasOpenActivity))
}

func TestValidationError_Unwrap(t *testing.T) {
	err := &ValidationError{Result: ValidationResult{Valid: false, Issues: []ValidationIssue{
		{Code: "x", Message: "y"},
	}}}
	assert.True(t, errors.Is(err, ErrValidation))
}

func TestValidateLayoutDocument_InvalidShapeAndElement(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       5000,
		HeightMm:      4000,
		Tables: []LayoutTable{
			{TableID: 1, XMm: 10, YMm: 10, WidthMm: 400, HeightMm: 400, Shape: "hexagon"},
		},
		Elements: []LayoutElement{
			{ElementType: "spaceship"},
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.False(t, res.Valid)
	codes := map[string]bool{}
	for _, iss := range res.Issues {
		codes[iss.Code] = true
	}
	assert.True(t, codes["table_shape"], "issues: %+v", res.Issues)
	assert.True(t, codes["element_type"], "issues: %+v", res.Issues)
}

func TestValidateLayoutDocument_TableBoundsSoftAndZeroSizeHard(t *testing.T) {
	// Out-of-bounds alone is a soft warning.
	boundsOnly := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       1000,
		HeightMm:      1000,
		Tables: []LayoutTable{
			{TableID: 1, XMm: 900, YMm: 900, WidthMm: 200, HeightMm: 200, Shape: "square"},
		},
	}
	res := ValidateLayoutDocument(boundsOnly)
	assert.True(t, res.Valid, "bounds must not block draft: issues=%+v", res.Issues)
	warnCodes := map[string]bool{}
	for _, iss := range res.Warnings {
		warnCodes[iss.Code] = true
	}
	assert.True(t, warnCodes["table_bounds"], "warnings: %+v", res.Warnings)

	// Zero size remains hard-invalid.
	zero := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       1000,
		HeightMm:      1000,
		Tables: []LayoutTable{
			{TableID: 2, XMm: 0, YMm: 0, WidthMm: 0, HeightMm: 100, Shape: "round"},
		},
	}
	res2 := ValidateLayoutDocument(zero)
	assert.False(t, res2.Valid)
	codes := map[string]bool{}
	for _, iss := range res2.Issues {
		codes[iss.Code] = true
	}
	assert.True(t, codes["table_size"], "issues: %+v", res2.Issues)
}

func TestValidateLayoutDocument_BadRegion(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       2000,
		HeightMm:      2000,
		Regions: []LayoutRegion{
			{Name: "", PolygonMm: []geometry.Point{{X: 0, Y: 0}, {X: 10, Y: 10}, {X: 10, Y: 0}, {X: 0, Y: 10}}}, // self-intersect + empty name
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.False(t, res.Valid)
}

func TestValidateLayoutDocument_SchemaVersion(t *testing.T) {
	doc := &LayoutDocument{SchemaVersion: 99, WidthMm: 100, HeightMm: 100}
	res := ValidateLayoutDocument(doc)
	assert.False(t, res.Valid)
}

func TestValidateLayoutDocument_Nil(t *testing.T) {
	res := ValidateLayoutDocument(nil)
	assert.False(t, res.Valid)
}

func TestParseLayoutDocument_Empty(t *testing.T) {
	doc, err := ParseLayoutDocument(nil)
	require.NoError(t, err)
	assert.Equal(t, LayoutSchemaVersion, doc.SchemaVersion)

	doc, err = ParseLayoutDocument([]byte("{}"))
	require.NoError(t, err)
	assert.Equal(t, LayoutSchemaVersion, doc.SchemaVersion)

	_, err = ParseLayoutDocument([]byte(`{not-json`))
	assert.Error(t, err)
}

func TestDraftHasOperatorContent(t *testing.T) {
	assert.False(t, draftHasOperatorContent(nil))
	assert.False(t, draftHasOperatorContent(database.JSONRawMessage(`{}`)))
	assert.True(t, draftHasOperatorContent(database.JSONRawMessage(
		`{"schema_version":1,"width_mm":1000,"height_mm":1000,"tables":[{"table_id":1,"x_mm":0,"y_mm":0,"width_mm":100,"height_mm":100,"shape":"round"}]}`,
	)))
	// Dimension-only drafts are operator content (must not be auto-clobbered).
	assert.True(t, draftHasOperatorContent(database.JSONRawMessage(
		`{"schema_version":1,"width_mm":5000,"height_mm":4000}`,
	)))
}

func TestValidateReviewLayoutAgainstResult_RejectsInventedElements(t *testing.T) {
	x0, y0 := 0, 0
	resultDoc := LayoutDocument{
		SchemaVersion: 1, WidthMm: 2000, HeightMm: 2000,
		Elements: []LayoutElement{{ElementType: "wall", Name: "W1", XMm: &x0, YMm: &y0}},
	}
	result, err := MarshalLayoutDocument(&resultDoc)
	require.NoError(t, err)
	// Same count but invented type/position must fail (not count-only).
	sub := []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"elements":[
		{"element_type":"bar","name":"Bar","x_mm":500,"y_mm":500}
	],"tables":[]}`)
	err = ValidateReviewLayoutAgainstResult(result, sub)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "elements")
}

func TestValidateReviewLayoutAgainstResult_RejectsForeignAndInvented(t *testing.T) {
	result := []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"A","client_key":"k1","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round"}
	]}`)
	// Foreign real table_id
	err := ValidateReviewLayoutAgainstResult(result, []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":99,"name":"X","x_mm":0,"y_mm":0,"width_mm":100,"height_mm":100,"shape":"round"}
	]}`))
	require.Error(t, err)

	// Invented candidate not in result
	err = ValidateReviewLayoutAgainstResult(result, []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"Invented","x_mm":500,"y_mm":500,"width_mm":400,"height_mm":400,"shape":"square"}
	]}`))
	require.Error(t, err)

	// Filtered subset with client_key is OK
	err = ValidateReviewLayoutAgainstResult(result, []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[
		{"table_id":0,"name":"A","client_key":"k1","x_mm":10,"y_mm":10,"width_mm":400,"height_mm":400,"shape":"round"}
	]}`))
	require.NoError(t, err)

	// Empty tables (reject all candidates) OK
	err = ValidateReviewLayoutAgainstResult(result, []byte(`{"schema_version":1,"width_mm":2000,"height_mm":2000,"tables":[]}`))
	require.NoError(t, err)
}

func TestValidateLayoutDocument_RotatedBoundsWarns(t *testing.T) {
	doc := &LayoutDocument{
		SchemaVersion: 1,
		WidthMm:       1000,
		HeightMm:      1000,
		Tables: []LayoutTable{
			{TableID: 1, XMm: 900, YMm: 900, WidthMm: 200, HeightMm: 200, Shape: "square", RotationDeg: 45},
		},
	}
	res := ValidateLayoutDocument(doc)
	assert.True(t, res.Valid)
	found := false
	for _, w := range res.Warnings {
		if w.Code == "table_bounds" {
			found = true
		}
	}
	assert.True(t, found, "rotated OOB must soft-warn: %+v", res.Warnings)
}

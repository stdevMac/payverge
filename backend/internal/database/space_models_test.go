package database

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRestaurantSpace_TableName(t *testing.T) {
	assert.Equal(t, "restaurant_spaces", (RestaurantSpace{}).TableName())
}

func TestSpaceRegion_TableName(t *testing.T) {
	assert.Equal(t, "space_regions", (SpaceRegion{}).TableName())
}

func TestSpaceLayoutElement_TableName(t *testing.T) {
	assert.Equal(t, "space_layout_elements", (SpaceLayoutElement{}).TableName())
}

func TestSpaceScanSession_TableName(t *testing.T) {
	assert.Equal(t, "space_scan_sessions", (SpaceScanSession{}).TableName())
}

func TestSpaceScanUpload_TableName(t *testing.T) {
	assert.Equal(t, "space_scan_uploads", (SpaceScanUpload{}).TableName())
}

func TestTableCombination_TableName(t *testing.T) {
	assert.Equal(t, "table_combinations", (TableCombination{}).TableName())
}

func TestSpaceLayoutAuditEvent_TableName(t *testing.T) {
	assert.Equal(t, "space_layout_audit_events", (SpaceLayoutAuditEvent{}).TableName())
}

func TestHashScanToken_Deterministic(t *testing.T) {
	a := HashScanToken("opaque-token-abc")
	b := HashScanToken("opaque-token-abc")
	assert.Equal(t, a, b)
	assert.Len(t, a, 64) // sha256 hex
	assert.NotEqual(t, a, HashScanToken("other"))
}

func TestScanTokenPrefix(t *testing.T) {
	assert.Equal(t, "12345678", ScanTokenPrefix("1234567890abcdef"))
	assert.Equal(t, "short", ScanTokenPrefix("short"))
}

func TestTable_LayoutFieldsZeroValue(t *testing.T) {
	// New layout fields must be zero/nil by default for backward compatibility.
	var tbl Table
	assert.Nil(t, tbl.SpaceID)
	assert.Nil(t, tbl.PosXMm)
	assert.False(t, tbl.LayoutPublished)
	assert.Equal(t, float64(0), tbl.RotationDeg)
}

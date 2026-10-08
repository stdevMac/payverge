package server

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// These guards lock in audit items that were fixed in prior batches so the
// long-tail contract work cannot silently regress them (2026-05-29 contract
// long-tail triage).

// #130 — hospitality day_of_week validation must stay wired.
func TestContractLongtail_HospitalityDayOfWeekStillValidated(t *testing.T) {
	err := database.ValidateBusinessOperatingHours([]database.BusinessOperatingHours{
		{DayOfWeek: 7, OpenTime: "09:00", CloseTime: "17:00"},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "day_of_week must be between 0 and 6")
}

// #126 — the live table routes use the slug-aware getTableRouteBusiness helper
// (CreateTableWithQR / UpdateTable / UpdateTableDetails / DeleteTableSoft /
// GetTablesWithStatus). Dead raw-ParseUint CreateTable/GetTables/DeleteTable
// wrappers were removed. If getTableRouteBusiness is removed, live routes break.
func TestContractLongtail_SlugAwareTableHelperExists(t *testing.T) {
	// Compile-time reference: getTableRouteBusiness is the canonical slug-aware
	// table business lookup used by every wired table route.
	var _ = getTableRouteBusiness
}

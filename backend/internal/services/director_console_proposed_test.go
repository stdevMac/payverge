package services

import (
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services/director_actions"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// proposedTestBizSeq is a monotonic counter used to generate unique BusinessId
// strings when creating multiple businesses within a single test DB.
var proposedTestBizSeq uint64

// newProposedTestDB returns an in-memory SQLite DB with the tables required
// for assembleProposedActions tests. It mirrors newServiceTestDB but also
// auto-migrates the proposal table.
func newProposedTestDB(t *testing.T) *database.DB {
	t.Helper()
	db := newServiceTestDB(t)
	require.NoError(t, db.GetGorm().AutoMigrate(
		&database.DirectorProposedAction{},
	))
	return db
}

// makePreviewJSON builds a preview_json blob that the assembler decodes.
// It includes both the ActionPreview fields and the title/description/warnings/
// requires_reconfirm meta fields stored in the same blob (Task 4 pattern).
func makePreviewJSON(t *testing.T, preview director_actions.ActionPreview, title, desc string, warnings []string, reconfirm bool) string {
	t.Helper()
	blob := map[string]any{
		"affected_count":     preview.AffectedCount,
		"examples":           preview.Examples,
		"summary":            preview.Summary,
		"title":              title,
		"description":        desc,
		"warnings":           warnings,
		"requires_reconfirm": reconfirm,
	}
	b, err := json.Marshal(blob)
	require.NoError(t, err)
	return string(b)
}

// createUniqueBizForProposedTest creates a Business with a unique BusinessId so
// multiple businesses can be created in the same SQLite test DB without
// hitting the uniqueIndex on BusinessId.
func createUniqueBizForProposedTest(t *testing.T, db *database.DB, name string) database.Business {
	t.Helper()
	seq := atomic.AddUint64(&proposedTestBizSeq, 1)
	biz := database.Business{
		BusinessId:     fmt.Sprintf("biz_%s_%d", name, seq),
		Name:           name,
		SettlementAddr: "settlement-" + name,
		TippingAddr:    "tipping-" + name,
	}
	require.NoError(t, db.GetGorm().Create(&biz).Error)
	return biz
}

// seedProposal creates and returns a DirectorProposedAction in the test DB with
// a generated public_id so the unique constraint is not violated.
func seedProposal(t *testing.T, db *database.DB, bizID uint, status database.DirectorProposalStatus, previewJSON string) *database.DirectorProposedAction {
	t.Helper()
	seq := atomic.AddUint64(&proposedTestBizSeq, 1)
	row := &database.DirectorProposedAction{
		PublicID:    fmt.Sprintf("pa_test_%d", seq),
		BusinessID:  bizID,
		ThreadID:    1,
		Kind:        string(director_actions.KindAdjustPrices),
		ParamsJSON:  `{"scope":"all","mode":"percent","value":20,"direction":"up"}`,
		PreviewJSON: previewJSON,
		MenuVersion: 3,
		Status:      status,
		ExpiresAt:   time.Now().Add(time.Hour),
	}
	require.NoError(t, db.GetGorm().Create(row).Error)
	return row
}

func TestAssembleProposedActions_OrderPreservedAndFiltered(t *testing.T) {
	db := newProposedTestDB(t)
	biz := createTestBusinessForService(t, db, "Proposal Test Restaurant")

	// Seed two pending proposals with distinct data.
	preview1 := director_actions.ActionPreview{
		AffectedCount: 5,
		Summary:       "Raise all prices 20%",
		Examples: []director_actions.PreviewExample{
			{Name: "Burger", Before: 10.0, After: 12.0},
		},
	}
	pj1 := makePreviewJSON(t, preview1, "Raise All Prices 20%", "Applies to 5 items.", []string{"Large swing"}, true)
	row1 := seedProposal(t, db, biz.ID, database.DirectorProposalPending, pj1)

	preview2 := director_actions.ActionPreview{
		AffectedCount: 2,
		Summary:       "Mark 2 items unavailable",
	}
	pj2 := makePreviewJSON(t, preview2, "Set Items Unavailable", "2 items will be hidden.", nil, false)
	row2 := seedProposal(t, db, biz.ID, database.DirectorProposalPending, pj2)

	// Seed a third row that is already applied — it should be filtered out.
	pj3 := makePreviewJSON(t, director_actions.ActionPreview{AffectedCount: 1}, "Applied Action", "Already done.", nil, false)
	seedProposal(t, db, biz.ID, database.DirectorProposalApplied, pj3)

	// Build a service (we only need the assembler, no real AI needed).
	svc := newDirectorServiceWithProvider(t, db, &fakeProvider{})

	// Call in order: row1 then row2 (the applied row3 is intentionally omitted from ids
	// because the loop only tracks ids for proposals it created in this ask).
	result := svc.assembleProposedActions(biz.ID, []uint{row1.ID, row2.ID})

	require.Len(t, result, 2, "expected exactly 2 pending proposals")

	// Order must be preserved.
	assert.Equal(t, row1.PublicID, result[0].ID, "first element must map to row1's public_id")
	assert.Equal(t, row2.PublicID, result[1].ID)

	// Verify first proposal fields.
	assert.Equal(t, director_actions.KindAdjustPrices, result[0].Kind)
	assert.Equal(t, "Raise All Prices 20%", result[0].Title)
	assert.Equal(t, "Applies to 5 items.", result[0].Description)
	assert.Equal(t, 5, result[0].Preview.AffectedCount)
	assert.Equal(t, "Raise all prices 20%", result[0].Preview.Summary)
	assert.Equal(t, []string{"Large swing"}, result[0].Warnings)
	assert.True(t, result[0].RequiresReconfirm)
	assert.Equal(t, uint(3), result[0].MenuVersion)
	assert.False(t, result[0].ExpiresAt.IsZero())

	// Verify second proposal fields.
	assert.Equal(t, "Set Items Unavailable", result[1].Title)
	assert.Equal(t, 2, result[1].Preview.AffectedCount)
	assert.False(t, result[1].RequiresReconfirm)
	assert.Nil(t, result[1].Warnings)
}

func TestAssembleProposedActions_AppliedRowFilteredOut(t *testing.T) {
	db := newProposedTestDB(t)
	biz := createTestBusinessForService(t, db, "Filter Test Restaurant")

	pj := makePreviewJSON(t, director_actions.ActionPreview{AffectedCount: 1}, "Done Action", "was applied.", nil, false)
	applied := seedProposal(t, db, biz.ID, database.DirectorProposalApplied, pj)

	dismissed := seedProposal(t, db, biz.ID, database.DirectorProposalDismissed, pj)

	svc := newDirectorServiceWithProvider(t, db, &fakeProvider{})
	result := svc.assembleProposedActions(biz.ID, []uint{applied.ID, dismissed.ID})

	assert.Empty(t, result, "applied and dismissed proposals must be filtered out")
}

func TestAssembleProposedActions_EmptyIDs(t *testing.T) {
	db := newProposedTestDB(t)
	biz := createTestBusinessForService(t, db, "Empty IDs Restaurant")

	svc := newDirectorServiceWithProvider(t, db, &fakeProvider{})
	result := svc.assembleProposedActions(biz.ID, nil)
	assert.Nil(t, result)

	result = svc.assembleProposedActions(biz.ID, []uint{})
	assert.Nil(t, result)
}

func TestAssembleProposedActions_BusinessScopeIsolation(t *testing.T) {
	db := newProposedTestDB(t)
	biz1 := createUniqueBizForProposedTest(t, db, "Business One")
	biz2 := createUniqueBizForProposedTest(t, db, "Business Two")

	pj := makePreviewJSON(t, director_actions.ActionPreview{AffectedCount: 3}, "Biz2 Action", "belongs to biz2.", nil, false)
	row := seedProposal(t, db, biz2.ID, database.DirectorProposalPending, pj)

	svc := newDirectorServiceWithProvider(t, db, &fakeProvider{})
	// Ask for biz1 but pass the row ID that belongs to biz2 — must return nothing.
	result := svc.assembleProposedActions(biz1.ID, []uint{row.ID})
	assert.Empty(t, result, "rows from a different business must not be returned")
}

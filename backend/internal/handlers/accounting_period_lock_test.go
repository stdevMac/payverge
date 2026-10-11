package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/accounting"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// L6-24: empty locked_through string on close must 400 (not silent reopen).
func TestPostPeriodLock_EmptyDateReturns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xPeriodOwner")

	h := NewAccountingHandler(database.GetDBWrapper())
	router := gin.New()
	router.Use(func(c *gin.Context) {
		c.Set("token_type", "web3")
		c.Set("address", "0xPeriodOwner")
		c.Next()
	})
	router.POST("/inside/businesses/:id/accounting/period-lock", h.PostPeriodLock)

	path := fmt.Sprintf("/inside/businesses/%d/accounting/period-lock", business.ID)
	empty := ""
	w := performAccountingRequest(t, router, http.MethodPost, path, map[string]any{
		"locked_through": empty,
		"note":           "attempted close without date",
	})
	assert.Equal(t, http.StatusBadRequest, w.Code, w.Body.String())

	// Null still reopens / records a null lock row (explicit reopen path).
	w2 := performAccountingRequest(t, router, http.MethodPost, path, map[string]any{
		"locked_through": nil,
		"note":           "explicit reopen",
	})
	assert.Equal(t, http.StatusCreated, w2.Code, w2.Body.String())

	// Valid date closes.
	w3 := performAccountingRequest(t, router, http.MethodPost, path, map[string]any{
		"locked_through": "2026-08-01",
		"note":           "month end",
	})
	require.Equal(t, http.StatusCreated, w3.Code, w3.Body.String())
}

// ACCT-LOCK-TZ: a Buenos Aires business locked through 2026-03-31 must reject
// an entry whose UTC instant is already 2026-04-01 but whose local calendar
// date is still 2026-03-31 (23:30 ART = 02:30 UTC).
func TestPeriodLock_CheckUnlockedUsesBusinessTimezone(t *testing.T) {
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xTZLock")
	require.NoError(t, database.GetDB().Model(business).Update("timezone", "America/Argentina/Buenos_Aires").Error)

	through := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.AccountingPeriodLock{
		BusinessID:    business.ID,
		LockedThrough: &through,
		Note:          "march close",
	}).Error)

	// Local 2026-03-31 23:30 (ART, UTC-3) is still inside the locked day.
	locked := time.Date(2026, 4, 1, 2, 30, 0, 0, time.UTC)
	err := accounting.CheckPeriodUnlocked(database.GetDB(), business.ID, locked)
	require.Error(t, err)
	require.True(t, errors.Is(err, accounting.ErrPeriodLocked))

	// Local 2026-04-01 01:00 is the next calendar day and stays open.
	open := time.Date(2026, 4, 1, 4, 0, 0, 0, time.UTC)
	require.NoError(t, accounting.CheckPeriodUnlocked(database.GetDB(), business.ID, open))
}

// CheckDateUnlocked compares a date-only value as stored: the first open day
// at UTC midnight stays open for a business west of UTC, and the locked day
// stays locked.
func TestPeriodLock_CheckDateUnlockedDoesNotShiftDates(t *testing.T) {
	setupAccountingHandlerDB(t)
	business := createAccountingHandlerBusiness(t, "0xDateOnlyLock")
	require.NoError(t, database.GetDB().Model(business).Update("timezone", "America/Argentina/Buenos_Aires").Error)

	through := time.Date(2026, 3, 31, 0, 0, 0, 0, time.UTC)
	require.NoError(t, database.GetDB().Create(&database.AccountingPeriodLock{
		BusinessID:    business.ID,
		LockedThrough: &through,
		Note:          "march close",
	}).Error)

	firstOpen := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, accounting.CheckDateUnlocked(database.GetDB(), business.ID, firstOpen))

	err := accounting.CheckDateUnlocked(database.GetDB(), business.ID, through)
	require.Error(t, err)
	require.True(t, errors.Is(err, accounting.ErrPeriodLocked))
}

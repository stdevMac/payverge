package fiscal

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

// TestUpdateReceiptStatusDoesNotClobberAuthorized locks F-STATUSDOWN: a status
// downgrade (e.g. a stale status_check reconcile or a budget-exhaustion) must
// never move an authorized or credited receipt backwards — that would hide a real
// CAE'd legal invoice. The authorized backfill path is separate and unaffected.
func TestUpdateReceiptStatusDoesNotClobberAuthorized(t *testing.T) {
	for _, terminal := range []database.FiscalStatus{
		database.FiscalStatusAuthorized,
		database.FiscalStatusCredited,
	} {
		t.Run(string(terminal), func(t *testing.T) {
			db := newFiscalTestDB(t)
			repo := NewRepository(db)
			createFiscalReceiptWithStatus(t, db, 5, 1, 1, 1, terminal)

			require.NoError(t, repo.UpdateReceiptStatus(5, database.FiscalStatusFailedPermanent, time.Now()))

			var r database.FiscalReceipt
			require.NoError(t, db.First(&r, 5).Error)
			require.Equal(t, terminal, r.Status, "must not downgrade a %s receipt", terminal)
		})
	}
}

// TestUpdateReceiptStatusDowngradesNonTerminal confirms the guard still lets a
// non-terminal receipt (pending / failed_retryable) be reconciled to a terminal
// failure — the legitimate use of UpdateReceiptStatus.
func TestUpdateReceiptStatusDowngradesNonTerminal(t *testing.T) {
	db := newFiscalTestDB(t)
	repo := NewRepository(db)
	createFiscalReceiptWithStatus(t, db, 5, 1, 1, 1, database.FiscalStatusFailedRetryable)

	require.NoError(t, repo.UpdateReceiptStatus(5, database.FiscalStatusFailedPermanent, time.Now()))

	var r database.FiscalReceipt
	require.NoError(t, db.First(&r, 5).Error)
	require.Equal(t, database.FiscalStatusFailedPermanent, r.Status)
}

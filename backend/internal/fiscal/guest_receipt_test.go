package fiscal

import (
	"context"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"

	"github.com/stretchr/testify/require"
)

func TestIsDemoPlaceholderFiscalReceipt(t *testing.T) {
	auth := "AUTH-144"
	demoNum := "DEMO-144"
	localQR := "https://payverge.local/fiscal/144"
	realCAE := "71234567890123"
	realQR := "https://www.arca.gob.ar/fe/qr/?p=abc"
	realNum := "00000042"

	require.True(t, IsDemoPlaceholderFiscalReceipt(&database.FiscalReceipt{AuthCode: &auth}))
	require.True(t, IsDemoPlaceholderFiscalReceipt(&database.FiscalReceipt{ReceiptNumber: &demoNum}))
	require.True(t, IsDemoPlaceholderFiscalReceipt(&database.FiscalReceipt{QRPayload: &localQR}))
	require.False(t, IsDemoPlaceholderFiscalReceipt(&database.FiscalReceipt{
		AuthCode: &realCAE, ReceiptNumber: &realNum, QRPayload: &realQR,
	}))
	require.False(t, IsDemoPlaceholderFiscalReceipt(nil))
}

func TestGuestMayExposeFiscalReceiptFailClosed(t *testing.T) {
	db := newFiscalTestDB(t)
	require.NoError(t, db.Create(&database.Business{
		ID: 7, BusinessId: "biz-guest-real", Name: "Real Bistro",
		OwnerAddress: "0xreal", Kind: database.BusinessKindReal,
	}).Error)
	require.NoError(t, db.Create(&database.Business{
		ID: 8, BusinessId: "biz-guest-demo", Name: "Demo Cafe",
		OwnerAddress: "0xdemo", Kind: database.BusinessKindDemo, IsDemo: true,
	}).Error)

	ctx := context.Background()
	require.True(t, GuestMayExposeFiscalReceipt(ctx, db, 7), "real tenant with fiscal enabled")
	require.False(t, GuestMayExposeFiscalReceipt(ctx, db, 8), "demo tenant")
	require.False(t, GuestMayExposeFiscalReceipt(ctx, db, 0), "missing business id")

	require.NoError(t, db.Model(&runtimecontrol.Control{}).
		Where("key = ?", runtimecontrol.ControlFiscal).
		Update("enabled", false).Error)
	require.False(t, GuestMayExposeFiscalReceipt(ctx, db, 7), "fiscal_enabled=false")

	require.NoError(t, db.Model(&runtimecontrol.Control{}).
		Where("key = ?", runtimecontrol.ControlFiscal).
		Updates(map[string]any{"enabled": true, "expires_at": time.Now().Add(-time.Hour)}).Error)
	require.False(t, GuestMayExposeFiscalReceipt(ctx, db, 7), "expired control falls back to disabled")
}

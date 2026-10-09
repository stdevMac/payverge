package services

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/structs"
)

// TestMarketingEmailOptedOut reproduces the unsubscribe-bypass finding: every
// lifecycle email footer links "manage notifications", but disabling email
// notifications there controlled NOTHING — winback/inactivity/feedback/
// milestone campaigns emailed straight through the opt-out (CAN-SPAM/GDPR
// exposure with a working-looking unsubscribe link). Marketing-toned campaigns
// must consult the owner's EmailEnabled preference; billing-critical emails
// (trial ending, grace, suspension) stay unsuppressible by design.
func TestMarketingEmailOptedOut(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.User{}))

	optedOut := &database.User{Email: "optout@example.com"}
	optedOut.NotificationPreferences = structs.NotificationPreferences{EmailEnabled: false}
	require.NoError(t, db.Create(optedOut).Error)

	optedIn := &database.User{Email: "optin@example.com"}
	optedIn.NotificationPreferences = structs.NotificationPreferences{EmailEnabled: true}
	require.NoError(t, db.Create(optedIn).Error)
	// EmailEnabled=true is the zero-adjacent default; force-write it so the
	// row is unambiguous regardless of GORM default handling.
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", optedIn.ID).
		Update("email_enabled", true).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", optedOut.ID).
		Update("email_enabled", false).Error)

	bizOut := &database.Business{BusinessId: "biz-optout", Name: "OptOut", OwnerAddress: "0xoo", Email: "biz-optout@example.com", UserID: &optedOut.ID}
	require.NoError(t, db.Create(bizOut).Error)
	bizIn := &database.Business{BusinessId: "biz-optin", Name: "OptIn", OwnerAddress: "0xoi", Email: "biz-optin@example.com", UserID: &optedIn.ID}
	require.NoError(t, db.Create(bizIn).Error)
	// No linked user: fail-open (send).
	bizOrphan := &database.Business{BusinessId: "biz-orphan", Name: "Orphan", OwnerAddress: "0xor", Email: "orphan@example.com"}
	require.NoError(t, db.Create(bizOrphan).Error)
	// No user row but the business email matches an opted-out user account.
	bizByEmail := &database.Business{BusinessId: "biz-byemail", Name: "ByEmail", OwnerAddress: "0xbe", Email: "OPTOUT@example.com"}
	require.NoError(t, db.Create(bizByEmail).Error)

	assert.True(t, marketingEmailOptedOut(bizOut), "owner disabled email notifications → marketing sends must skip")
	assert.False(t, marketingEmailOptedOut(bizIn), "opted-in owner keeps receiving campaigns")
	assert.False(t, marketingEmailOptedOut(bizOrphan), "no resolvable owner → fail open")
	assert.True(t, marketingEmailOptedOut(bizByEmail), "email-matched owner opt-out is honored when user_id is not set")
}

// P3: two accounts differing only by case — if EITHER opted out, marketing
// must suppress (conservative + deterministic; order of rows must not matter).
func TestMarketingEmailOptedOut_CaseCollidingUsersConservative(t *testing.T) {
	db := setupTestDB(t)
	require.NoError(t, db.AutoMigrate(&database.User{}))

	a := &database.User{Email: "collide@example.com"}
	require.NoError(t, db.Create(a).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", a.ID).Update("email_enabled", true).Error)
	b := &database.User{Email: "COLLIDE@example.com"}
	require.NoError(t, db.Create(b).Error)
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", b.ID).Update("email_enabled", false).Error)

	biz := &database.Business{BusinessId: "collide-biz", Name: "C", Email: "collide@example.com"}
	require.NoError(t, db.Create(biz).Error)

	require.True(t, marketingEmailOptedOut(biz),
		"any case-colliding opt-out must suppress marketing sends")
}

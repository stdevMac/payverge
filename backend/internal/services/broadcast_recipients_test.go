package services

import (
	"fmt"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupBroadcastRecipientsDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:broadcast_rcpt_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	database.SetTestDB(db)
	require.NoError(t, db.AutoMigrate(&database.User{}, &database.Business{}, &database.Bill{}))
	return db
}

// Task 13: non-deliverable domains hard-bounce and damage Resend reputation.
func TestIsDeliverableEmail_ExcludesReservedDomains(t *testing.T) {
	cases := map[string]bool{
		"owner@restaurant.com":     true,
		"ops@payverge.io":          true,
		"  Owner@Restaurant.COM  ": true,
		// example.com is a real public zone; the reserved TLD is bare ".example".
		"user@example.com":       true,
		"ci@payverge.test":       false,
		"qa@payverge.local":      false,
		"nobody@example.invalid": false,
		"user@sub.example":       false,
		"demo@foo.local":         false,
		"x@bar.test":             false,
		"y@baz.invalid":          false,
		"z@qux.example":          false,
		"":                       false,
		"not-an-email":           false,
		"@missing-local":         false,
		"missing-domain@":        false,
	}
	for email, want := range cases {
		assert.Equalf(t, want, IsDeliverableEmail(email), "IsDeliverableEmail(%q)", email)
	}
}

// Task 13: broadcast recipient resolver excludes kind!=real and non-deliverable
// domains so "All Businesses" never mails fixtures or reserved TLDs.
func TestResolveBroadcastRecipients_ExcludesFixturesAndNonDeliverable(t *testing.T) {
	db := setupBroadcastRecipientsDB(t)
	now := time.Now().UTC()

	seed := func(businessID, name, email string, kind database.BusinessKind) {
		t.Helper()
		require.NoError(t, db.Create(&database.Business{
			BusinessId: businessID,
			Name:       name,
			OwnerName:  name + " Owner",
			Email:      email,
			Kind:       kind,
			IsDemo:     kind == database.BusinessKindDemo,
			CreatedAt:  now,
			UpdatedAt:  now,
		}).Error)
	}

	seed("bc-real-1", "Real Bistro", "owner@real-bistro.com", database.BusinessKindReal)
	seed("bc-real-2", "Real Cafe", "hello@real-cafe.io", database.BusinessKindReal)
	// kind=real but reserved domain — must still be excluded by the domain guard.
	seed("bc-real-local", "Misclassified Local", "ops@payverge.local", database.BusinessKindReal)
	seed("bc-demo", "Demo Cafe", "demo@payverge.local", database.BusinessKindDemo)
	seed("bc-test", "CI Fixture", "ci@payverge.test", database.BusinessKindTest)
	// Blank email must not appear.
	seed("bc-real-blank", "No Email Biz", "", database.BusinessKindReal)

	got, err := ResolveBroadcastRecipients([]string{"all_businesses"})
	require.NoError(t, err)

	assert.Len(t, got, 2, "only deliverable kind=real addresses")
	assert.Contains(t, got, "owner@real-bistro.com")
	assert.Contains(t, got, "hello@real-cafe.io")
	assert.NotContains(t, got, "ops@payverge.local")
	assert.NotContains(t, got, "demo@payverge.local")
	assert.NotContains(t, got, "ci@payverge.test")
}

// Literal recipient addresses inherit the same domain guard — an admin cannot
// force-mail a reserved TLD through the free-form path.
func TestResolveBroadcastRecipients_LiteralAddressDomainGuard(t *testing.T) {
	_ = setupBroadcastRecipientsDB(t)

	got, err := ResolveBroadcastRecipients([]string{
		"safe@restaurant.com",
		"bounce@payverge.local",
		"ci@payverge.test",
	})
	require.NoError(t, err)
	assert.Len(t, got, 1)
	assert.Contains(t, got, "safe@restaurant.com")
}

// Preview helpers used by the admin compose screen: count + sample of the
// resolved deliverable set (not the raw registry size).
func TestListBroadcastableBusinessEmails_CountAndSample(t *testing.T) {
	db := setupBroadcastRecipientsDB(t)
	now := time.Now().UTC()

	require.NoError(t, db.Create(&database.Business{
		BusinessId: "bc-s1", Name: "Alpha", OwnerName: "A", Email: "a@alpha.com",
		Kind:      database.BusinessKindReal,
		CreatedAt: now, UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "bc-s2", Name: "Beta", OwnerName: "B", Email: "b@beta.com",
		Kind:      database.BusinessKindReal,
		CreatedAt: now.Add(time.Second), UpdatedAt: now,
	}).Error)
	require.NoError(t, db.Create(&database.Business{
		BusinessId: "bc-s3", Name: "Fixture", OwnerName: "F", Email: "f@payverge.local",
		Kind:      database.BusinessKindTest,
		CreatedAt: now, UpdatedAt: now,
	}).Error)

	rows, total, sample, err := ListBroadcastableBusinessEmails()
	require.NoError(t, err)
	assert.EqualValues(t, 2, total)
	assert.Len(t, rows, 2)
	assert.NotEmpty(t, sample)
	assert.LessOrEqual(t, len(sample), BroadcastRecipientSampleSize)
	for _, email := range sample {
		assert.True(t, IsDeliverableEmail(email), "sample must only contain deliverable addresses")
	}
}

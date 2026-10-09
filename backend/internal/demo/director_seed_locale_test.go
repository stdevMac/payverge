package demo

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Issue #820: the Director Console on EN operator chrome served a Spanish
// (voseo) Sage briefing card. The demo seed keyed the operator-facing director
// thread off business.DefaultLanguage — the GUEST menu default — so a venue
// with an es guest menu but an English-operating owner got "Impulsá cócteles"
// pinned in its console. Operator-facing seed copy must follow the demo
// owner's UI language (User.LanguageSelected), with the business language only
// as a fallback when the owner has none stored.

const (
	demoSeedTitleEN = "Demo Director Briefing"
	demoSeedTitleES = "Informe del Director de demostración"
)

// An English-operating owner whose demo venue later carries a Spanish guest
// menu must keep the ENGLISH briefing card after a reseed — and must not gain
// a duplicate Spanish one.
func TestDirectorDemoSeedFollowsOperatorLanguageNotGuestDefault(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := database.User{Email: "demo-820-en-op@example.com", Role: "admin", LanguageSelected: "en"}
	require.NoError(t, db.Create(&admin).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-820", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// Operator flips the GUEST menu default to Spanish (es-AR venue); the
	// operator UI language stays English.
	require.NoError(t, db.Model(&database.Business{}).
		Where("demo_owner_user_id = ?", admin.ID).
		Updates(map[string]interface{}{"default_language": "es", "source_language": "es"}).Error)

	// Reseed (the hourly ensure re-runs the static seeding idempotently).
	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var esThreads int64
	require.NoError(t, db.Model(&database.DirectorConsoleThread{}).
		Where("title = ?", demoSeedTitleES).Count(&esThreads).Error)
	require.Zero(t, esThreads,
		"EN operator must not get a Spanish (voseo) briefing card because the guest menu is Spanish")

	var threads []database.DirectorConsoleThread
	require.NoError(t, db.Where("title = ? AND pinned = ?", demoSeedTitleEN, true).Find(&threads).Error)
	require.NotEmpty(t, threads, "the English briefing card must survive the reseed")
	for _, th := range threads {
		require.Equal(t, "en", th.Locale)
	}
}

// A Spanish-operating owner gets the Spanish card even when the venue's guest
// default is English — the console is an operator surface.
func TestDirectorDemoSeedSpanishOperatorGetsSpanishCard(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := database.User{Email: "demo-820-es-op@example.com", Role: "admin", LanguageSelected: "es-AR"}
	require.NoError(t, db.Create(&admin).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-820", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var enThreads int64
	require.NoError(t, db.Model(&database.DirectorConsoleThread{}).
		Where("title = ?", demoSeedTitleEN).Count(&enThreads).Error)
	require.Zero(t, enThreads)

	var threads []database.DirectorConsoleThread
	require.NoError(t, db.Where("title = ? AND pinned = ?", demoSeedTitleES, true).Find(&threads).Error)
	require.NotEmpty(t, threads)
	for _, th := range threads {
		require.Equal(t, "es", th.Locale)
	}
}

// B7 follow-up to #820: when a CURRENT-locale pinned card already exists
// (created after a locale switch), the heal path must NOT rename the stale
// card onto the same title — that would mint the very duplicate pair of
// identically-titled pinned threads #820 complained about. The stale card is
// archived and unpinned instead; renaming only happens when no current-locale
// twin exists.
func TestDirectorDemoSeedHealArchivesStaleWhenCurrentTwinExists(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := database.User{Email: "demo-820-b7@example.com", Role: "admin"}
	require.NoError(t, db.Create(&admin).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-820", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// The seed ran with no operator language → the business language (es for
	// the AR demo venues) wins, so a Spanish pinned card exists.
	var stale database.DirectorConsoleThread
	require.NoError(t, db.Where("title = ? AND pinned = ?", demoSeedTitleES, true).First(&stale).Error)

	// Operator switches to English AND a current-locale pinned card already
	// exists in the business scope (e.g. minted after the locale switch).
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", admin.ID).
		Update("language_selected", "en").Error)
	twin := database.DirectorConsoleThread{
		BusinessID:    stale.BusinessID,
		Title:         demoSeedTitleEN,
		Locale:        "en",
		LastMessageAt: fixedNow().UTC(),
		Pinned:        true,
	}
	require.NoError(t, db.Create(&twin).Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var pinnedEN int64
	require.NoError(t, db.Model(&database.DirectorConsoleThread{}).
		Where("business_id = ? AND title = ? AND pinned = ?", stale.BusinessID, demoSeedTitleEN, true).
		Count(&pinnedEN).Error)
	require.Equal(t, int64(1), pinnedEN,
		"healing must never leave two identically-titled pinned briefing cards")

	var archived database.DirectorConsoleThread
	require.NoError(t, db.First(&archived, stale.ID).Error)
	require.Equal(t, demoSeedTitleES, archived.Title,
		"the stale card is archived, not renamed onto its twin's title")
	require.False(t, archived.Pinned, "the stale card must be unpinned")
	require.NotNil(t, archived.ArchivedAt, "the stale card must be archived")

	// The surviving pinned Spanish card is the pre-existing twin, untouched.
	var survivor database.DirectorConsoleThread
	require.NoError(t, db.First(&survivor, twin.ID).Error)
	require.True(t, survivor.Pinned)
	require.Nil(t, survivor.ArchivedAt)
}

// Self-heal: a demo that was seeded before the operator picked their UI
// language must convert the wrong-language card IN PLACE on the next reseed —
// same thread row, retitled and re-localized, with the seeded message updated —
// never a duplicate pair of briefing cards.
func TestDirectorDemoSeedSelfHealsWrongLocaleCard(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := database.User{Email: "demo-820-heal@example.com", Role: "admin"}
	require.NoError(t, db.Create(&admin).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-820", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	// The seed ran with no operator language → the business language (es for
	// the AR demo venues) wins, so Spanish cards exist.
	var before []database.DirectorConsoleThread
	require.NoError(t, db.Where("title = ? AND pinned = ?", demoSeedTitleES, true).Find(&before).Error)
	require.NotEmpty(t, before)

	// Operator now selects English and the hourly ensure reseeds.
	require.NoError(t, db.Model(&database.User{}).Where("id = ?", admin.ID).
		Update("language_selected", "en").Error)
	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var leftoverES int64
	require.NoError(t, db.Model(&database.DirectorConsoleThread{}).
		Where("title = ?", demoSeedTitleES).Count(&leftoverES).Error)
	require.Zero(t, leftoverES, "the Spanish card must be converted, not left beside an English twin")

	for _, oldThread := range before {
		var healed database.DirectorConsoleThread
		require.NoError(t, db.First(&healed, oldThread.ID).Error, "healed card keeps the same thread row")
		require.Equal(t, demoSeedTitleEN, healed.Title)
		require.Equal(t, "en", healed.Locale)

		var msg database.DirectorConsoleMessage
		require.NoError(t, db.Where("thread_id = ? AND model_name = ?", healed.ID, "demo-seed").
			First(&msg).Error)
		require.Equal(t, "en", msg.Locale)
		require.Contains(t, msg.Content, "Push cocktails", "seeded message content follows the healed locale")
	}
}

// #820 (live thread 10): the card whose TITLE already matches the current copy
// but whose LOCALE drifted. Ask() used to relabel any thread it answered in,
// pinned seed card included, so the English "Demo Director Briefing" ended up
// stamped es-AR. The rename branch above only fires for a stale-TITLE card and
// FirstOrCreate/Attrs only writes on CREATE, so every reseed left the row
// wrong. The seed must converge the label (and the seeded message) in place.
func TestDirectorDemoSeedRepairsDriftedLocaleOnMatchingTitle(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := database.User{Email: "demo-820-drift@example.com", Role: "admin", LanguageSelected: "en"}
	require.NoError(t, db.Create(&admin).Error)

	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "test-820", BaselineDays: 3})
	_, err := svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var seeded database.DirectorConsoleThread
	require.NoError(t, db.Where("title = ? AND pinned = ?", demoSeedTitleEN, true).First(&seeded).Error)
	require.Equal(t, "en", seeded.Locale)

	// A Spanish follow-up asked inside the pinned card relabels it (the live
	// pre-fix behaviour that produced thread 10).
	require.NoError(t, db.Model(&database.DirectorConsoleThread{}).
		Where("id = ?", seeded.ID).Update("locale", "es-AR").Error)
	require.NoError(t, db.Model(&database.DirectorConsoleMessage{}).
		Where("thread_id = ? AND model_name = ?", seeded.ID, "demo-seed").
		Update("locale", "es-AR").Error)

	_, err = svc.EnsureForAdmin(context.Background(), admin.ID)
	require.NoError(t, err)

	var healed database.DirectorConsoleThread
	require.NoError(t, db.First(&healed, seeded.ID).Error, "the repair happens in place, same thread row")
	require.Equal(t, demoSeedTitleEN, healed.Title)
	require.Equal(t, "en", healed.Locale,
		"a reseed must converge the drifted locale, not leave the English briefing labelled es-AR")

	var msg database.DirectorConsoleMessage
	require.NoError(t, db.Where("thread_id = ? AND model_name = ?", healed.ID, "demo-seed").First(&msg).Error)
	require.Equal(t, "en", msg.Locale, "the seeded briefing message is realigned too")

	var pinned int64
	require.NoError(t, db.Model(&database.DirectorConsoleThread{}).
		Where("business_id = ? AND pinned = ?", healed.BusinessID, true).Count(&pinned).Error)
	require.EqualValues(t, 1, pinned, "the repair must not mint a second pinned card")
}

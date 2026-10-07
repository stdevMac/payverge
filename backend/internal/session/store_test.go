package session

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&UserSession{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE user_session_refresh_history (
			session_id integer NOT NULL,
			token_hash text NOT NULL,
			rotated_at datetime NOT NULL,
			expires_at datetime NOT NULL DEFAULT '2099-01-01 00:00:00',
			PRIMARY KEY (session_id, token_hash),
			UNIQUE (token_hash),
			FOREIGN KEY (session_id) REFERENCES user_sessions(id) ON DELETE CASCADE
		)
	`).Error)
	return NewStore(db)
}

func seedSession(t *testing.T, s *Store, userID *uint, address, refreshRaw string) *UserSession {
	return seedSessionWithProvider(t, s, userID, "email", refreshRaw, address)
}

func seedSessionWithProvider(t *testing.T, s *Store, userID *uint, provider, refreshRaw string, address ...string) *UserSession {
	t.Helper()
	addr := ""
	if len(address) > 0 {
		addr = address[0]
	}
	sess, err := s.Create(CreateInput{
		UserID:    userID,
		Address:   addr,
		TokenHash: HashToken("session-" + refreshRaw),
		Provider:  provider,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, s.RotateRefreshToken(sess.ID, HashToken(refreshRaw), time.Now().Add(7*24*time.Hour)))
	return sess
}

func TestCreateGeneratesUniqueProvisionalTokenHashesConcurrently(t *testing.T) {
	s := newTestStore(t)

	const sessionCount = 16
	start := make(chan struct{})
	results := make(chan struct {
		session *UserSession
		err     error
	}, sessionCount)

	var wg sync.WaitGroup
	for i := 0; i < sessionCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			sess, err := s.Create(CreateInput{
				Provider:  "email",
				ExpiresAt: time.Now().Add(time.Hour),
			})
			results <- struct {
				session *UserSession
				err     error
			}{session: sess, err: err}
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	seen := make(map[string]struct{}, sessionCount)
	for result := range results {
		require.NoError(t, result.err)
		require.NotNil(t, result.session)
		require.NotEmpty(t, result.session.SessionToken)
		require.NotEqual(t, "pending", result.session.SessionToken)
		seen[result.session.SessionToken] = struct{}{}
	}
	require.Len(t, seen, sessionCount)
}

func TestCreateReplacesLegacyPendingSentinel(t *testing.T) {
	s := newTestStore(t)

	first, err := s.Create(CreateInput{
		TokenHash: "pending",
		Provider:  "email",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	second, err := s.Create(CreateInput{
		TokenHash: "pending",
		Provider:  "google",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	require.NotEqual(t, "pending", first.SessionToken)
	require.NotEqual(t, "pending", second.SessionToken)
	require.NotEqual(t, first.SessionToken, second.SessionToken)
}

func requireRevokedWithReason(t *testing.T, s *Store, sessionID uint, reason RevocationReason) time.Time {
	t.Helper()
	var row UserSession
	require.NoError(t, s.db.First(&row, sessionID).Error)
	require.True(t, row.Revoked)
	require.NotNil(t, row.RevokedAt)
	require.Equal(t, string(reason), row.RevocationReason)
	return *row.RevokedAt
}

func TestRevokeWithReason(t *testing.T) {
	s := newTestStore(t)
	uid := uint(21)
	sess := seedSession(t, s, &uid, "", "single-reason")

	require.NoError(t, s.RevokeWithReason(sess.ID, RevocationReasonUserLogout))
	requireRevokedWithReason(t, s, sess.ID, RevocationReasonUserLogout)
}

func TestRevokeByTokenHashWithReason(t *testing.T) {
	s := newTestStore(t)
	uid := uint(22)
	sess := seedSession(t, s, &uid, "", "token-hash-reason")

	require.NoError(t, s.RevokeByTokenHashWithReason(sess.SessionToken, RevocationReasonInvalidSession))
	requireRevokedWithReason(t, s, sess.ID, RevocationReasonInvalidSession)
}

// SEC-6 / #303: logout with only a refresh cookie must revoke via refresh hash.
func TestRevokeByRefreshTokenHashWithReason(t *testing.T) {
	s := newTestStore(t)
	uid := uint(26)
	refreshRaw := "refresh-hash-reason"
	sess := seedSession(t, s, &uid, "", refreshRaw)

	_, err := s.ValidateRefreshToken(HashToken(refreshRaw))
	require.NoError(t, err)

	require.NoError(t, s.RevokeByRefreshTokenHashWithReason(HashToken(refreshRaw), RevocationReasonUserLogout))
	requireRevokedWithReason(t, s, sess.ID, RevocationReasonUserLogout)

	_, err = s.ValidateRefreshToken(HashToken(refreshRaw))
	require.Error(t, err, "refresh token must be invalid after revoke-by-refresh-hash")
}

func TestRevokeByRefreshTokenHashWithReason_EmptyNoOp(t *testing.T) {
	s := newTestStore(t)
	uid := uint(27)
	sess := seedSession(t, s, &uid, "", "refresh-empty-noop")

	require.NoError(t, s.RevokeByRefreshTokenHashWithReason("", RevocationReasonUserLogout))
	var row UserSession
	require.NoError(t, s.db.First(&row, sess.ID).Error)
	require.False(t, row.Revoked)
}

func TestRevokeAllForUserWithReason(t *testing.T) {
	s := newTestStore(t)
	uid := uint(23)
	otherUID := uint(24)
	first := seedSession(t, s, &uid, "", "user-reason-a")
	second := seedSessionWithProvider(t, s, &uid, "google", "user-reason-b")
	unrelated := seedSession(t, s, &otherUID, "", "user-reason-other")

	require.NoError(t, s.RevokeAllForScopedUserWithReason(uid, RevocationReasonSecurityReset))
	firstRevokedAt := requireRevokedWithReason(t, s, first.ID, RevocationReasonSecurityReset)
	secondRevokedAt := requireRevokedWithReason(t, s, second.ID, RevocationReasonSecurityReset)
	require.Equal(t, firstRevokedAt, secondRevokedAt, "one bulk revoke must use one timestamp")

	var unrelatedRow UserSession
	require.NoError(t, s.db.First(&unrelatedRow, unrelated.ID).Error)
	require.False(t, unrelatedRow.Revoked)
	require.Nil(t, unrelatedRow.RevokedAt)
	require.Empty(t, unrelatedRow.RevocationReason)
}

func TestRevokeAllLinkedOperatorSessions_AddressKeyed(t *testing.T) {
	s := newTestStore(t)
	uid := uint(41)
	email := seedSessionWithProvider(t, s, &uid, "email", "linked-email")
	google := seedSessionWithProvider(t, s, &uid, "google", "linked-google")
	staff := seedSessionWithProvider(t, s, &uid, "staff_code", "linked-staff")
	customer := seedSessionWithProvider(t, s, &uid, "customer", "linked-customer")

	const mixed = "0xAbCdEf0123456789AbCdEf0123456789AbCdEf01"
	addressSess, err := s.Create(CreateInput{
		Address:   mixed,
		TokenHash: HashToken("linked-address-only"),
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)
	require.Nil(t, addressSess.UserID)

	// Historical rows may have been stored before address normalization.
	legacy := UserSession{
		Address:      "0xFEEDFEEDFEEDFEEDFEEDFEEDFEEDFEEDFEEDFEED",
		SessionToken: HashToken("linked-legacy-mixed"),
		Provider:     "dynamic_web3",
		ExpiresAt:    time.Now().Add(time.Hour),
		LastUsedAt:   time.Now(),
	}
	require.NoError(t, s.db.Create(&legacy).Error)

	unrelated, err := s.Create(CreateInput{
		Address:   "0xdddddddddddddddddddddddddddddddddddddddd",
		TokenHash: HashToken("linked-unrelated"),
		Provider:  "web3",
		ExpiresAt: time.Now().Add(time.Hour),
	})
	require.NoError(t, err)

	require.NoError(t, s.RevokeAllLinkedOperatorSessions(
		uid,
		[]string{mixed, "0xFEEDFEEDFEEDFEEDFEEDFEEDFEEDFEEDFEEDFEED"},
		RevocationReasonSecurityReset,
	))

	requireRevokedWithReason(t, s, email.ID, RevocationReasonSecurityReset)
	requireRevokedWithReason(t, s, google.ID, RevocationReasonSecurityReset)
	requireRevokedWithReason(t, s, addressSess.ID, RevocationReasonSecurityReset)
	requireRevokedWithReason(t, s, legacy.ID, RevocationReasonSecurityReset)

	var staffRow UserSession
	require.NoError(t, s.db.First(&staffRow, staff.ID).Error)
	require.False(t, staffRow.Revoked)
	var customerRow UserSession
	require.NoError(t, s.db.First(&customerRow, customer.ID).Error)
	require.False(t, customerRow.Revoked)
	var other UserSession
	require.NoError(t, s.db.First(&other, unrelated.ID).Error)
	require.False(t, other.Revoked)
}

func TestRevokeAllForScopedUserWithReason(t *testing.T) {
	s := newTestStore(t)
	uid := uint(25)
	email := seedSessionWithProvider(t, s, &uid, "email", "scoped-reason-email")
	google := seedSessionWithProvider(t, s, &uid, "google", "scoped-reason-google")
	staff := seedSessionWithProvider(t, s, &uid, "staff", "scoped-reason-staff")

	require.NoError(t, s.RevokeAllForScopedUserWithReason(
		uid,
		RevocationReasonAdministrative,
		"email",
		"google",
	))
	emailRevokedAt := requireRevokedWithReason(t, s, email.ID, RevocationReasonAdministrative)
	googleRevokedAt := requireRevokedWithReason(t, s, google.ID, RevocationReasonAdministrative)
	require.Equal(t, emailRevokedAt, googleRevokedAt, "one scoped revoke must use one timestamp")

	var staffRow UserSession
	require.NoError(t, s.db.First(&staffRow, staff.ID).Error)
	require.False(t, staffRow.Revoked)
	require.Nil(t, staffRow.RevokedAt)
	require.Empty(t, staffRow.RevocationReason)
}

func TestRevokeFamilyWithReason(t *testing.T) {
	s := newTestStore(t)
	uid := uint(26)
	first := seedSession(t, s, &uid, "", "family-reason-a")
	second := seedSessionWithProvider(t, s, &uid, "google", "family-reason-b")
	unrelated := seedSessionWithProvider(t, s, &uid, "staff", "family-reason-staff")

	require.NoError(t, s.RevokeFamilyWithReason(first, RevocationReasonRefreshTokenReuse))
	firstRevokedAt := requireRevokedWithReason(t, s, first.ID, RevocationReasonRefreshTokenReuse)
	secondRevokedAt := requireRevokedWithReason(t, s, second.ID, RevocationReasonRefreshTokenReuse)
	require.Equal(t, firstRevokedAt, secondRevokedAt, "one family revoke must use one timestamp")

	var unrelatedRow UserSession
	require.NoError(t, s.db.First(&unrelatedRow, unrelated.ID).Error)
	require.False(t, unrelatedRow.Revoked, "provider-class scoping must remain unchanged")
}

func TestRevokeWithReasonPreservesOriginalAuditFields(t *testing.T) {
	s := newTestStore(t)
	uid := uint(27)
	sess := seedSession(t, s, &uid, "", "preserve-reason")

	require.NoError(t, s.RevokeWithReason(sess.ID, RevocationReasonAccountDisabled))
	originalRevokedAt := requireRevokedWithReason(t, s, sess.ID, RevocationReasonAccountDisabled)
	time.Sleep(time.Millisecond)

	require.NoError(t, s.RevokeWithReason(sess.ID, RevocationReasonAdministrative))
	secondRevokedAt := requireRevokedWithReason(t, s, sess.ID, RevocationReasonAccountDisabled)
	require.Equal(t, originalRevokedAt, secondRevokedAt, "an already-revoked row must not be rewritten")
}

func TestRevokeWithReasonNormalizesUnsafeReasons(t *testing.T) {
	tests := []struct {
		name   string
		reason RevocationReason
	}{
		{name: "blank", reason: ""},
		{name: "unknown", reason: "operator_typo"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			uid := uint(28)
			sess := seedSession(t, s, &uid, "", "normalize-"+tt.name)

			require.NoError(t, s.RevokeWithReason(sess.ID, tt.reason))
			requireRevokedWithReason(t, s, sess.ID, RevocationReasonUnspecified)
		})
	}
}

// TestRotateSession_CASMatchAndMiss locks AUTH-1: rotation is a transactional
// compare-and-swap on the old refresh hash. A correct old hash advances the
// token atomically (refresh + session hashes both move, old hash recorded as
// previous); a stale old hash matches nothing (a concurrent rotation already won).
func TestRotateSession_CASMatchAndMiss(t *testing.T) {
	s := newTestStore(t)
	uid := uint(7)
	sess := seedSession(t, s, &uid, "", "refresh-A")

	oldHash := HashToken("refresh-A")
	newRefresh := HashToken("refresh-B")
	newSession := HashToken("session-B")

	// Correct old hash -> CAS matches.
	matched, err := s.RotateSession(sess.ID, oldHash, newRefresh, newSession,
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched, "rotation with the correct old hash must match")

	var row UserSession
	require.NoError(t, s.db.First(&row, sess.ID).Error)
	require.Equal(t, newRefresh, row.RefreshToken, "refresh hash advanced")
	require.Equal(t, oldHash, row.PreviousRefreshToken, "old hash recorded as previous")
	require.Equal(t, newSession, row.SessionToken, "session hash advanced atomically")

	// Replaying the now-stale old hash -> CAS misses (no row updated).
	matched, err = s.RotateSession(sess.ID, oldHash, HashToken("refresh-C"), HashToken("session-C"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.False(t, matched, "a stale old hash must not match (lost CAS)")

	var after UserSession
	require.NoError(t, s.db.First(&after, sess.ID).Error)
	require.Equal(t, newRefresh, after.RefreshToken, "stale CAS must not mutate the row")
}

// TestStorePersistence_AcrossRecreation locks the deployment invariant that
// the database, rather than a Store instance, owns session state. A replacement
// process can construct a new Store over the same database, validate tokens
// issued before the restart, and continue the normal atomic rotation flow.
func TestStorePersistence_AcrossRecreation(t *testing.T) {
	firstStore := newTestStore(t)
	uid := uint(31)
	accessHash := HashToken("access-before-restart")
	refreshHash := HashToken("refresh-before-restart")

	sess, err := firstStore.Create(CreateInput{
		UserID:    &uid,
		TokenHash: accessHash,
		Provider:  "email",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, firstStore.RotateRefreshToken(
		sess.ID,
		refreshHash,
		time.Now().Add(7*24*time.Hour),
	))

	// Simulate the first process going away while its database remains.
	persistentDB := firstStore.db
	firstStore = nil
	restartedStore := NewStore(persistentDB)

	valid, err := restartedStore.Validate(sess.ID, accessHash)
	require.NoError(t, err)
	require.True(t, valid, "the pre-restart access hash remains valid")

	persisted, err := restartedStore.ValidateRefreshToken(refreshHash)
	require.NoError(t, err)
	require.Equal(t, sess.ID, persisted.ID, "the pre-restart refresh hash remains valid")

	newAccessHash := HashToken("access-after-restart")
	newRefreshHash := HashToken("refresh-after-restart")
	matched, err := restartedStore.RotateSession(
		sess.ID,
		refreshHash,
		newRefreshHash,
		newAccessHash,
		time.Now().Add(7*24*time.Hour),
		time.Now().Add(24*time.Hour),
	)
	require.NoError(t, err)
	require.True(t, matched, "the restarted store rotates the persisted session")

	valid, err = restartedStore.Validate(sess.ID, newAccessHash)
	require.NoError(t, err)
	require.True(t, valid, "the rotated access hash is persisted")
	valid, err = restartedStore.Validate(sess.ID, accessHash)
	require.NoError(t, err)
	require.False(t, valid, "the pre-restart access hash was replaced")

	rotated, err := restartedStore.ValidateRefreshToken(newRefreshHash)
	require.NoError(t, err)
	require.Equal(t, sess.ID, rotated.ID)
	require.Equal(t, refreshHash, rotated.PreviousRefreshToken)
	_, err = restartedStore.ValidateRefreshToken(refreshHash)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound, "the pre-restart refresh hash was replaced")
}

// TestFindByPreviousRefreshToken locks the reuse-detection lookup: after a
// rotation, the rotated-away hash is discoverable as a "previous" token so a
// replay of it can be recognized as reuse.
func TestFindByPreviousRefreshToken(t *testing.T) {
	s := newTestStore(t)
	uid := uint(9)
	sess := seedSession(t, s, &uid, "", "refresh-A")
	oldHash := HashToken("refresh-A")

	_, err := s.RotateSession(sess.ID, oldHash, HashToken("refresh-B"), HashToken("session-B"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)

	found, err := s.FindByPreviousRefreshToken(oldHash)
	require.NoError(t, err)
	require.NotNil(t, found)
	require.Equal(t, sess.ID, found.ID)

	missing, err := s.FindByPreviousRefreshToken(HashToken("never-issued"))
	require.NoError(t, err)
	require.Nil(t, missing, "an unknown hash returns (nil, nil), not an error")
}

func TestFindByPreviousRefreshTokenIgnoresExpiredHistory(t *testing.T) {
	s := newTestStore(t)
	uid := uint(29)
	sess := seedSession(t, s, &uid, "", "expired-history-old")
	oldHash := HashToken("expired-history-old")
	matched, err := s.RotateSession(sess.ID, oldHash, HashToken("expired-history-new"),
		HashToken("expired-history-session"), time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)
	require.NoError(t, s.db.Model(&RefreshTokenHistory{}).Where("token_hash = ?", oldHash).
		Update("expires_at", time.Now().Add(-time.Second)).Error)

	found, err := s.FindByPreviousRefreshToken(oldHash)
	require.NoError(t, err)
	require.Nil(t, found)
}

func TestRefreshHistoryFindsEveryRotatedGeneration(t *testing.T) {
	s := newTestStore(t)
	uid := uint(10)
	sess := seedSession(t, s, &uid, "", "refresh-R1")

	r1 := HashToken("refresh-R1")
	r2 := HashToken("refresh-R2")
	r3 := HashToken("refresh-R3")
	matched, err := s.RotateSession(sess.ID, r1, r2, HashToken("session-R2"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)
	matched, err = s.RotateSession(sess.ID, r2, r3, HashToken("session-R3"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)

	for _, hash := range []string{r1, r2} {
		found, findErr := s.FindByPreviousRefreshToken(hash)
		require.NoError(t, findErr)
		require.NotNil(t, found, "every rotated-away generation must remain discoverable")
		require.Equal(t, sess.ID, found.ID)
		require.Equal(t, hash, found.PreviousRefreshToken)
		require.NotNil(t, found.PreviousRotatedAt)
	}

	var historyCount int64
	require.NoError(t, s.db.Table("user_session_refresh_history").
		Where("session_id = ?", sess.ID).Count(&historyCount).Error)
	require.Equal(t, int64(2), historyCount)

	matched, err = s.RotateSession(sess.ID, r1, HashToken("refresh-stale"), HashToken("session-stale"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.False(t, matched, "a historical token must never pass the CAS")
	require.NoError(t, s.db.Table("user_session_refresh_history").
		Where("session_id = ?", sess.ID).Count(&historyCount).Error)
	require.Equal(t, int64(2), historyCount, "a stale CAS must not append history")
}

func TestRotateSessionSetsHistoryDetectionExpiry(t *testing.T) {
	s := newTestStore(t)
	uid := uint(19)
	sess := seedSession(t, s, &uid, "", "refresh-expiry-source")
	expectedExpiry := time.Now().Add(7 * 24 * time.Hour)

	matched, err := s.RotateSession(sess.ID, HashToken("refresh-expiry-source"), HashToken("refresh-expiry-next"),
		HashToken("session-expiry-next"), expectedExpiry, time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)

	var expiresAt time.Time
	require.NoError(t, s.db.Table("user_session_refresh_history").
		Select("expires_at").Where("session_id = ?", sess.ID).Scan(&expiresAt).Error)
	require.WithinDuration(t, expectedExpiry, expiresAt, time.Second)
}

func TestRotateSessionRejectsCrossSessionHistoryTokenCollision(t *testing.T) {
	s := newTestStore(t)
	firstUser := uint(30)
	secondUser := uint(31)
	sharedRaw := "cross-session-history-collision"
	first := seedSession(t, s, &firstUser, "", sharedRaw)
	second := seedSession(t, s, &secondUser, "", "cross-session-second-seed")
	oldHash := HashToken(sharedRaw)
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", second.ID).
		Update("refresh_token", oldHash).Error)

	matched, err := s.RotateSession(first.ID, oldHash, HashToken("collision-first-new"),
		HashToken("collision-first-session"), time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)

	matched, err = s.RotateSession(second.ID, oldHash, HashToken("collision-second-new"),
		HashToken("collision-second-session"), time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.Error(t, err)
	require.False(t, matched)

	var persisted UserSession
	require.NoError(t, s.db.First(&persisted, second.ID).Error)
	require.Equal(t, oldHash, persisted.RefreshToken, "collision must roll back the losing session update")
	var owner RefreshTokenHistory
	require.NoError(t, s.db.Where("token_hash = ?", oldHash).First(&owner).Error)
	require.Equal(t, first.ID, owner.SessionID)
}

func TestRotateSessionConcurrentCASRecordsOnlyWinner(t *testing.T) {
	s := newTestStore(t)
	uid := uint(11)
	sess := seedSession(t, s, &uid, "", "refresh-race-old")
	oldHash := HashToken("refresh-race-old")

	type result struct {
		matched bool
		err     error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			matched, err := s.RotateSession(
				sess.ID,
				oldHash,
				HashToken(fmt.Sprintf("refresh-race-%d", i)),
				HashToken(fmt.Sprintf("session-race-%d", i)),
				time.Now().Add(7*24*time.Hour),
				time.Now().Add(24*time.Hour),
			)
			results <- result{matched: matched, err: err}
		}()
	}
	close(start)
	wg.Wait()
	close(results)

	winners := 0
	for got := range results {
		require.NoError(t, got.err)
		if got.matched {
			winners++
		}
	}
	require.Equal(t, 1, winners, "exactly one concurrent refresh may win the CAS")

	var historyCount int64
	require.NoError(t, s.db.Table("user_session_refresh_history").
		Where("session_id = ? AND token_hash = ?", sess.ID, oldHash).Count(&historyCount).Error)
	require.Equal(t, int64(1), historyCount, "the CAS winner records one rotated-away generation")
}

func TestRotateSessionHistoryFailureRollsBackCredentials(t *testing.T) {
	s := newTestStore(t)
	uid := uint(13)
	sess := seedSession(t, s, &uid, "", "refresh-atomic-old")
	oldHash := HashToken("refresh-atomic-old")
	require.NoError(t, s.db.Exec("DROP TABLE user_session_refresh_history").Error)

	matched, err := s.RotateSession(sess.ID, oldHash, HashToken("refresh-atomic-new"), HashToken("session-atomic-new"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.Error(t, err)
	require.False(t, matched, "a rolled-back rotation must not report a committed CAS match")

	var row UserSession
	require.NoError(t, s.db.First(&row, sess.ID).Error)
	require.Equal(t, oldHash, row.RefreshToken, "transaction rollback must preserve the old refresh credential")
	require.Equal(t, HashToken("session-refresh-atomic-old"), row.SessionToken,
		"transaction rollback must preserve the old access credential")
	require.Empty(t, row.PreviousRefreshToken)
}

func TestCleanExpiredCascadesRefreshHistory(t *testing.T) {
	s := newTestStore(t)
	uid := uint(12)
	sess := seedSession(t, s, &uid, "", "refresh-expired")
	matched, err := s.RotateSession(sess.ID, HashToken("refresh-expired"), HashToken("refresh-next"), HashToken("session-next"),
		time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)

	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sess.ID).
		Updates(map[string]interface{}{
			"expires_at":         time.Now().Add(-25 * time.Hour),
			"refresh_expires_at": time.Now().Add(-25 * time.Hour),
		}).Error)
	require.NoError(t, s.CleanExpired())

	var historyCount int64
	require.NoError(t, s.db.Table("user_session_refresh_history").
		Where("session_id = ?", sess.ID).Count(&historyCount).Error)
	require.Zero(t, historyCount, "session lifecycle cleanup must bound refresh history")
}

func TestCleanExpiredPreservesSessionWithValidRefreshToken(t *testing.T) {
	s := newTestStore(t)
	uid := uint(14)
	sess := seedSession(t, s, &uid, "", "refresh-still-valid")
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sess.ID).
		Update("expires_at", time.Now().Add(-25*time.Hour)).Error)

	require.NoError(t, s.CleanExpired())
	var count int64
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sess.ID).Count(&count).Error)
	require.Equal(t, int64(1), count)
}

func TestCleanExpiredRemovesLegacySessionsWithoutRefreshExpiry(t *testing.T) {
	s := newTestStore(t)
	uid := uint(18)

	active := seedSession(t, s, &uid, "", "refresh-legacy-active")
	revoked := seedSession(t, s, &uid, "", "refresh-legacy-revoked")
	for _, sessionID := range []uint{active.ID, revoked.ID} {
		require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sessionID).
			Updates(map[string]interface{}{
				"expires_at":         time.Now().Add(-25 * time.Hour),
				"refresh_expires_at": nil,
			}).Error)
	}
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", revoked.ID).
		Updates(map[string]interface{}{
			"revoked":    true,
			"revoked_at": nil,
		}).Error)

	require.NoError(t, s.CleanExpired())
	var count int64
	require.NoError(t, s.db.Model(&UserSession{}).
		Where("id IN ?", []uint{active.ID, revoked.ID}).Count(&count).Error)
	require.Zero(t, count, "legacy sessions without refresh credentials must fall back to access expiry")
}

func TestCleanExpiredPrunesHistoryAtDetectionExpiryWhileParentLives(t *testing.T) {
	s := newTestStore(t)
	uid := uint(15)
	sess := seedSession(t, s, &uid, "", "refresh-history-expiry")
	matched, err := s.RotateSession(sess.ID, HashToken("refresh-history-expiry"), HashToken("refresh-history-next"),
		HashToken("session-history-next"), time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)
	require.NoError(t, s.db.Table("user_session_refresh_history").Where("session_id = ?", sess.ID).
		Update("expires_at", time.Now().Add(-time.Second)).Error)

	require.NoError(t, s.CleanExpired())
	var historyCount, sessionCount int64
	require.NoError(t, s.db.Table("user_session_refresh_history").Where("session_id = ?", sess.ID).Count(&historyCount).Error)
	require.Zero(t, historyCount)
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sess.ID).Count(&sessionCount).Error)
	require.Equal(t, int64(1), sessionCount)
}

func TestCleanExpiredRetainsHistoryUntilDetectionExpiry(t *testing.T) {
	s := newTestStore(t)
	uid := uint(16)
	sess := seedSession(t, s, &uid, "", "refresh-history-live")
	matched, err := s.RotateSession(sess.ID, HashToken("refresh-history-live"), HashToken("refresh-history-live-next"),
		HashToken("session-history-live-next"), time.Now().Add(7*24*time.Hour), time.Now().Add(24*time.Hour))
	require.NoError(t, err)
	require.True(t, matched)
	require.NoError(t, s.CleanExpired())
	var historyCount int64
	require.NoError(t, s.db.Table("user_session_refresh_history").Where("session_id = ?", sess.ID).Count(&historyCount).Error)
	require.Equal(t, int64(1), historyCount)
}

func TestCleanExpiredUsesRevocationAuditGrace(t *testing.T) {
	s := newTestStore(t)
	uid := uint(17)
	sess := seedSession(t, s, &uid, "", "refresh-revoked-cleanup")
	require.NoError(t, s.RevokeWithReason(sess.ID, RevocationReasonAdministrative))
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sess.ID).
		Update("revoked_at", time.Now().Add(-25*time.Hour)).Error)
	require.NoError(t, s.CleanExpired())
	var count int64
	require.NoError(t, s.db.Model(&UserSession{}).Where("id = ?", sess.ID).Count(&count).Error)
	require.Zero(t, count)
}

// TestRevokeFamily locks family revocation on a detected reuse: every live
// session for the offending identity is revoked. Keyed on user_id when present,
// else on address (web3 sessions carry a nil user_id).
func TestRevokeFamily(t *testing.T) {
	s := newTestStore(t)

	// user-id family
	uid := uint(11)
	a := seedSession(t, s, &uid, "", "ra")
	b := seedSession(t, s, &uid, "", "rb")
	require.NoError(t, s.RevokeFamilyWithReason(a, RevocationReasonUnspecified))
	for _, id := range []uint{a.ID, b.ID} {
		var row UserSession
		require.NoError(t, s.db.First(&row, id).Error)
		require.True(t, row.Revoked, "all sessions for the user must be revoked")
	}

	// address family (nil user id, e.g. web3)
	addr := "0xabc"
	c := seedSession(t, s, nil, addr, "rc")
	d := seedSession(t, s, nil, addr, "rd")
	require.NoError(t, s.RevokeFamilyWithReason(c, RevocationReasonUnspecified))
	for _, id := range []uint{c.ID, d.ID} {
		var row UserSession
		require.NoError(t, s.db.First(&row, id).Error)
		require.True(t, row.Revoked, "all sessions for the address must be revoked")
	}
}

// TestRevokeFamily_ScopedByProviderClass locks the fix for a cross-identity
// over-revoke: user_sessions.user_id is drawn from THREE distinct PK namespaces
// (User.ID, Staff.ID, Customer.ID), so a bare user_id revoke would log out
// unrelated identities that happen to share the numeric id. RevokeFamilyWithReason must
// stay within the offending session's provider class.
func TestRevokeFamily_ScopedByProviderClass(t *testing.T) {
	s := newTestStore(t)
	shared := uint(5)

	// Three sessions, same numeric id, different identity namespaces.
	customerSess := seedSessionWithProvider(t, s, &shared, "customer", "cust-r")
	staffSess := seedSessionWithProvider(t, s, &shared, "staff_code", "staff-r")
	userSess := seedSessionWithProvider(t, s, &shared, "email", "user-r")

	// A customer reuse event must revoke ONLY the customer session.
	require.NoError(t, s.RevokeFamilyWithReason(customerSess, RevocationReasonUnspecified))

	var cRow, sRow, uRow UserSession
	require.NoError(t, s.db.First(&cRow, customerSess.ID).Error)
	require.NoError(t, s.db.First(&sRow, staffSess.ID).Error)
	require.NoError(t, s.db.First(&uRow, userSess.ID).Error)
	require.True(t, cRow.Revoked, "the customer session is revoked")
	require.False(t, sRow.Revoked, "the unrelated staff session (same id) must NOT be revoked")
	require.False(t, uRow.Revoked, "the unrelated user session (same id) must NOT be revoked")
}

func TestRevokeFamily_GenericStaffProviderStaysInStaffNamespace(t *testing.T) {
	s := newTestStore(t)
	shared := uint(17)

	genericStaff := seedSessionWithProvider(t, s, &shared, "staff", "staff-generic")
	codeStaff := seedSessionWithProvider(t, s, &shared, "staff_code", "staff-code")
	userSess := seedSessionWithProvider(t, s, &shared, "email", "user-email")

	require.NoError(t, s.RevokeFamilyWithReason(genericStaff, RevocationReasonUnspecified))

	var genericRow, codeRow, userRow UserSession
	require.NoError(t, s.db.First(&genericRow, genericStaff.ID).Error)
	require.NoError(t, s.db.First(&codeRow, codeStaff.ID).Error)
	require.NoError(t, s.db.First(&userRow, userSess.ID).Error)
	require.True(t, genericRow.Revoked)
	require.True(t, codeRow.Revoked, "all staff-provider sessions belong to one revocation family")
	require.False(t, userRow.Revoked, "same numeric user id in the user namespace must remain active")
}

func TestRevokeFamily_UnknownProviderDoesNotEnterUserNamespace(t *testing.T) {
	s := newTestStore(t)
	shared := uint(18)
	unknown := seedSessionWithProvider(t, s, &shared, "future_provider", "future-provider")
	unknownSibling := seedSessionWithProvider(t, s, &shared, "future_provider", "future-provider-sibling")
	userSess := seedSessionWithProvider(t, s, &shared, "email", "known-user-provider")
	require.NoError(t, s.RevokeFamilyWithReason(unknown, RevocationReasonRefreshTokenReuse))
	var unknownRow, siblingRow, userRow UserSession
	require.NoError(t, s.db.First(&unknownRow, unknown.ID).Error)
	require.NoError(t, s.db.First(&siblingRow, unknownSibling.ID).Error)
	require.NoError(t, s.db.First(&userRow, userSess.ID).Error)
	require.True(t, unknownRow.Revoked)
	require.True(t, siblingRow.Revoked)
	require.False(t, userRow.Revoked)
}

// TestRecentlyRotated locks the reuse grace window: a previous-token match whose
// rotation happened moments ago is a benign concurrent double-refresh, not a
// stolen-token replay, so it must not trigger family revocation.
func TestRecentlyRotated(t *testing.T) {
	now := time.Date(2026, 6, 24, 12, 0, 0, 0, time.UTC)

	require.False(t, RecentlyRotated(nil, now))
	require.False(t, RecentlyRotated(&UserSession{}, now), "no rotation timestamp -> not recent")

	tests := []struct {
		name   string
		age    time.Duration
		recent bool
	}{
		{name: "89 seconds is benign", age: 89 * time.Second, recent: true},
		{name: "90 second boundary is reuse", age: 90 * time.Second, recent: false},
		{name: "91 seconds is reuse", age: 91 * time.Second, recent: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rotatedAt := now.Add(-tt.age)
			require.Equal(t, tt.recent, RecentlyRotated(&UserSession{PreviousRotatedAt: &rotatedAt}, now))
		})
	}
}

// TestStorePersistenceAcrossRecreation proves sessions live in the database,
// not in Store state: a session created through one Store survives that Store
// being discarded, and a brand-new Store over the same database validates and
// rotates it (deployment-restart semantics).
func TestStorePersistenceAcrossRecreation(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&UserSession{}))
	require.NoError(t, db.Exec(`
		CREATE TABLE user_session_refresh_history (
			session_id integer NOT NULL,
			token_hash text NOT NULL,
			rotated_at datetime NOT NULL,
			expires_at datetime NOT NULL DEFAULT '2099-01-01 00:00:00',
			PRIMARY KEY (session_id, token_hash),
			UNIQUE (token_hash),
			FOREIGN KEY (session_id) REFERENCES user_sessions(id) ON DELETE CASCADE
		)
	`).Error)

	first := NewStore(db)
	uid := uint(77)
	sess, err := first.Create(CreateInput{
		UserID:    &uid,
		TokenHash: HashToken("persist-session"),
		Provider:  "email",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	require.NoError(t, err)
	require.NoError(t, first.RotateRefreshToken(sess.ID, HashToken("persist-refresh-1"), time.Now().Add(7*24*time.Hour)))

	// Simulate process recreation: discard the first Store and build a new one
	// over the same database.
	first = nil
	second := NewStore(db)

	ok, err := second.Validate(sess.ID, HashToken("persist-session"))
	require.NoError(t, err)
	require.True(t, ok, "pre-restart session token must validate on the new store")

	// CAS rotation (the real refresh path) records previous for reuse detection.
	matched, err := second.RotateSession(
		sess.ID,
		HashToken("persist-refresh-1"),
		HashToken("persist-refresh-2"),
		HashToken("persist-session-2"),
		time.Now().Add(7*24*time.Hour),
		time.Now().Add(24*time.Hour),
	)
	require.NoError(t, err)
	require.True(t, matched)
	var row UserSession
	require.NoError(t, db.First(&row, sess.ID).Error)
	require.Equal(t, HashToken("persist-refresh-2"), row.RefreshToken, "rotation works on the recreated store")
	require.Equal(t, HashToken("persist-refresh-1"), row.PreviousRefreshToken, "reuse detection state survived recreation")
	require.False(t, row.Revoked)
}

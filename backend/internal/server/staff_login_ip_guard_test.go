package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestVerifyLoginCode_StrangerCannotLockOutStaff: wrong codes from one client
// earn that client a 429 for that email, but never trip the identity-wide
// lockout, so the real staff member on another network still signs in.
func TestVerifyLoginCode_StrangerCannotLockOutStaff(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	orig := staffLoginGuard
	staffLoginGuard = newStaffLoginClientGuard(time.Minute, staffLoginPerClientMaxFailures)
	t.Cleanup(func() { staffLoginGuard = orig })

	business := createOwnedBusiness(t, "0xOwnerGuard", "Guard Biz")
	staff := createStaffMember(t, business.ID, "guarded@example.com", "Guarded")
	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID: staff.ID, Code: "424242", ExpiresAt: time.Now().Add(10 * time.Minute),
	}).Error)

	verify := func(ip, code string) int {
		body, err := json.Marshal(map[string]string{"email": staff.Email, "code": code})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.RemoteAddr = ip + ":1234"
		VerifyLoginCode(c)
		return w.Code
	}

	for i := 0; i < staffLoginPerClientMaxFailures; i++ {
		assert.Equal(t, http.StatusBadRequest, verify("198.51.100.7", "000000"), "attempt %d", i+1)
	}
	// The stranger's client is now blocked, even with the right code...
	assert.Equal(t, http.StatusTooManyRequests, verify("198.51.100.7", "424242"))

	// ...but the staff identity is not locked out.
	locked, _, err := database.GetDBWrapper().StaffService.IsLoginLocked(staff.ID)
	require.NoError(t, err)
	assert.False(t, locked, "a single client must not trip the identity-wide lockout")
	assert.Less(t, staffLoginPerClientMaxFailures, database.StaffLoginMaxFailedAttempts)

	// The real staff member, on another network, signs in.
	assert.Equal(t, http.StatusOK, verify("203.0.113.9", "424242"))
}

// TestVerifyLoginCode_CaseVariantsShareClientBucket: the account lookup
// normalizes the email, so the per-client guard must too; otherwise rotating
// the letter case gives one client 5 fresh failures per spelling and still
// trips the identity-wide lockout.
func TestVerifyLoginCode_CaseVariantsShareClientBucket(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	orig := staffLoginGuard
	staffLoginGuard = newStaffLoginClientGuard(time.Minute, staffLoginPerClientMaxFailures)
	t.Cleanup(func() { staffLoginGuard = orig })

	business := createOwnedBusiness(t, "0xOwnerGuardCase", "Guard Case Biz")
	staff := createStaffMember(t, business.ID, "cased@example.com", "Cased")
	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID: staff.ID, Code: "424242", ExpiresAt: time.Now().Add(10 * time.Minute),
	}).Error)

	verify := func(email, code string) int {
		body, err := json.Marshal(map[string]string{"email": email, "code": code})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.RemoteAddr = "198.51.100.8:1234"
		VerifyLoginCode(c)
		return w.Code
	}

	variants := []string{"Cased@example.com", "cASED@example.com", " CASED@EXAMPLE.COM ", "cased@Example.com"}
	blocked := 0
	for round := 0; round < database.StaffLoginMaxFailedAttempts; round++ {
		code := verify(variants[round%len(variants)], "000000")
		if code == http.StatusTooManyRequests {
			blocked++
		}
	}
	assert.Equal(t, database.StaffLoginMaxFailedAttempts-staffLoginPerClientMaxFailures, blocked,
		"case/whitespace variants must share one per-client bucket")

	locked, _, err := database.GetDBWrapper().StaffService.IsLoginLocked(staff.ID)
	require.NoError(t, err)
	assert.False(t, locked, "rotating email case from one client must not trip the identity-wide lockout")
}

func newStaffLoginRaceFixture(t *testing.T, ownerAddr, bizName, email string) (*database.Staff, func(ip, code string) int) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	setupStaffHandlerTestDB(t)
	orig := staffLoginGuard
	staffLoginGuard = newStaffLoginClientGuard(time.Minute, staffLoginPerClientMaxFailures)
	t.Cleanup(func() { staffLoginGuard = orig })

	business := createOwnedBusiness(t, ownerAddr, bizName)
	staff := createStaffMember(t, business.ID, email, "Raced")
	require.NoError(t, database.GetDB().Create(&database.StaffLoginCode{
		StaffID: staff.ID, Code: "424242", ExpiresAt: time.Now().Add(10 * time.Minute),
	}).Error)
	verify := func(ip, code string) int {
		body, err := json.Marshal(map[string]string{"email": staff.Email, "code": code})
		require.NoError(t, err)
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Request.RemoteAddr = ip + ":1234"
		VerifyLoginCode(c)
		return w.Code
	}
	return staff, verify
}

// TestVerifyLoginCode_RotatingClientsHitIdentityCeiling (10a): an attacker
// rotating source addresses gets a fresh per-client budget each time, but the
// per-email ceiling still stops guessing at StaffLoginMaxFailedAttempts and
// burns the outstanding code, so even the right code no longer works.
func TestVerifyLoginCode_RotatingClientsHitIdentityCeiling(t *testing.T) {
	staff, verify := newStaffLoginRaceFixture(t, "0xOwnerRotate", "Rotate Biz", "rotate@example.com")

	compared := 0
	for i := 0; i < database.StaffLoginMaxFailedAttempts*2; i++ {
		ip := fmt.Sprintf("198.51.100.%d", 10+i) // a new client every attempt
		if verify(ip, "000000") == http.StatusBadRequest {
			compared++
		}
	}
	assert.Equal(t, database.StaffLoginMaxFailedAttempts, compared,
		"rotating clients must not get more guesses than the per-email ceiling")
	locked, _, err := database.GetDBWrapper().StaffService.IsLoginLocked(staff.ID)
	require.NoError(t, err)
	assert.True(t, locked)
	assert.NotEqual(t, http.StatusOK, verify("203.0.113.50", "424242"), "the code must be burned after the ceiling")
}

// TestVerifyLoginCode_ConcurrentGuessesRespectCeilings (10b): parallel wrong
// codes cannot slip past either budget, because both the per-client and the
// per-email attempt are claimed atomically before the code is compared.
func TestVerifyLoginCode_ConcurrentGuessesRespectCeilings(t *testing.T) {
	t.Run("one client", func(t *testing.T) {
		_, verify := newStaffLoginRaceFixture(t, "0xOwnerRaceA", "Race Biz A", "race-a@example.com")
		got := runConcurrentVerifies(t, 40, func(int) int { return verify("198.51.100.77", "000000") })
		assert.Equal(t, staffLoginPerClientMaxFailures, got[http.StatusBadRequest])
		assert.Equal(t, 40-staffLoginPerClientMaxFailures, got[http.StatusTooManyRequests])
	})
	t.Run("many clients", func(t *testing.T) {
		_, verify := newStaffLoginRaceFixture(t, "0xOwnerRaceB", "Race Biz B", "race-b@example.com")
		const n = 60
		got := runConcurrentVerifies(t, n, func(i int) int { return verify(fmt.Sprintf("203.0.113.%d", 1+i), "000000") })
		assert.Equal(t, database.StaffLoginMaxFailedAttempts, got[http.StatusBadRequest])
		assert.Equal(t, n-database.StaffLoginMaxFailedAttempts, got[http.StatusTooManyRequests])
	})
}

func runConcurrentVerifies(t *testing.T, n int, call func(i int) int) map[int]int {
	t.Helper()
	var (
		wg  sync.WaitGroup
		mu  sync.Mutex
		got = map[int]int{}
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			code := call(i)
			mu.Lock()
			got[code]++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	return got
}

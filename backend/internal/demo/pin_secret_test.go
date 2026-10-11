package demo

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/stdevmac/payverge/backend/internal/database"
)

func resetDemoPINKeyForTest(t *testing.T) {
	t.Helper()
	demoPINKeyOnce = sync.Once{}
	demoPINKeyVal = nil
	t.Cleanup(func() {
		demoPINKeyOnce = sync.Once{}
		demoPINKeyVal = nil
	})
}

// legacyDemoStaffPIN is the pre-M-pin derivation: sha256 over a public
// constant and the staff email, computable by anyone who knows the email.
func legacyDemoStaffPIN(email string) string {
	sum := sha256.Sum256([]byte("demo-pin-v5:" + strings.ToLower(strings.TrimSpace(email))))
	return fmt.Sprintf("%04d", binary.BigEndian.Uint32(sum[:4])%9000+1000)
}

func demoRosterEmails(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("demo+admin1-business%d-staff%d@demo.example", i/5+1, i%5+1)
	}
	return out
}

// M-pin: the PIN must depend on the server secret, not only on the email.
func TestDemoStaffPIN_DependsOnServerSecret(t *testing.T) {
	emails := demoRosterEmails(40)
	keyA := []byte("instance-a-secret-key-material-0")
	keyB := []byte("instance-b-secret-key-material-0")

	differsAcrossKeys, differsFromLegacy := 0, 0
	for _, email := range emails {
		a := keyedDemoPIN(keyA, email, 0)
		require.Len(t, a, 4)
		assert.Equal(t, a, keyedDemoPIN(keyA, email, 0), "derivation must be stable for one key")
		if a != keyedDemoPIN(keyB, email, 0) {
			differsAcrossKeys++
		}
		if a != legacyDemoStaffPIN(email) {
			differsFromLegacy++
		}
	}
	// With 9000 possible PINs a handful of chance collisions is fine; a
	// key-independent derivation would differ for none.
	assert.GreaterOrEqual(t, differsAcrossKeys, 35, "PINs must change with the server key")
	assert.GreaterOrEqual(t, differsFromLegacy, 35, "PINs must not be the public sha256(email) derivation")
}

func TestDemoPINKey_DerivedFromPluginSecretKey(t *testing.T) {
	resetDemoPINKeyForTest(t)
	t.Setenv("PLUGIN_SECRET_KEY", "payverge-hermetic-test-key-00000")

	mac := hmac.New(sha256.New, []byte("payverge-hermetic-test-key-00000"))
	mac.Write([]byte(demoPINKeyLabel))
	assert.Equal(t, mac.Sum(nil), demoPINKey())
	assert.NotEqual(t, []byte("payverge-hermetic-test-key-00000"), demoPINKey(), "the raw plugin key must not be reused")
}

func TestDemoPINKey_RandomPerProcessWithoutSecret(t *testing.T) {
	t.Setenv("PLUGIN_SECRET_KEY", "")

	resetDemoPINKeyForTest(t)
	first := append([]byte(nil), demoPINKey()...)
	require.Len(t, first, 32)
	assert.Equal(t, first, demoPINKey(), "stable within a process")

	resetDemoPINKeyForTest(t)
	assert.NotEqual(t, first, demoPINKey(), "no secret means a fresh random key, never a constant")
}

// The Demo Center must never surface a PIN hint that no longer opens the
// staff login (for example after the PIN key changed).
func TestSummaryOmitsPINHintThatNoLongerMatchesHash(t *testing.T) {
	db := newDemoServiceTestDB(t)
	admin := seedAdmin(t, db, "pin-key-admin@example.com")
	svc := NewService(db, Options{Now: fixedNow, SeedVersion: "pin-key-seed", BaselineDays: 5})

	summary, err := svc.SummaryForAdmin(context.Background(), admin.ID, true)
	require.NoError(t, err)
	require.NotEmpty(t, summary.Access)
	target := summary.Access[0]
	require.NotEmpty(t, target.PINHint)

	otherHash, err := bcrypt.GenerateFromPassword([]byte("0000"), bcrypt.MinCost)
	require.NoError(t, err)
	require.NoError(t, db.Model(&database.Staff{}).
		Where("business_id = ? AND email = ?", target.BusinessID, target.Email).
		Update("pin_hash", string(otherHash)).Error)

	again, err := svc.SummaryForAdmin(context.Background(), admin.ID, false)
	require.NoError(t, err)
	found := false
	for _, identity := range again.Access {
		if identity.BusinessID == target.BusinessID && identity.Email == target.Email {
			found = true
			assert.Empty(t, identity.PINHint, "a hint that does not match pin_hash must be withheld")
		}
	}
	require.True(t, found)
}

// BenchmarkDemoPINMatchesHash measures the per-staff hint check the Demo
// Center summary now runs (seeded PIN hashes use bcrypt.MinCost).
func BenchmarkDemoPINMatchesHash(b *testing.B) {
	hash, err := bcrypt.GenerateFromPassword([]byte("4821"), bcrypt.MinCost)
	require.NoError(b, err)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !demoPINMatchesHash(string(hash), "4821") {
			b.Fatal("hash must match")
		}
	}
}

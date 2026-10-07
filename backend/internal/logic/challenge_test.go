package logic

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testAddress = "0x742d35cc6635c0532925a3b8d400e4c3f2c0c1c1"

func TestChallengeStore_IssueStoresNothing(t *testing.T) {
	store := NewChallengeStore()
	for i := 0; i < 10_000; i++ {
		_, err := store.Issue(fmt.Sprintf("0x%040x", i), time.Minute)
		require.NoError(t, err)
	}
	assert.Zero(t, store.Len())

	challenge, err := store.Issue(testAddress, time.Minute)
	require.NoError(t, err)
	_, ok := store.Verify(challenge.Value, testAddress)
	assert.True(t, ok, "a fresh client still gets a usable challenge")
}

func TestChallengeStore_NonceIsAlphanumeric(t *testing.T) {
	challenge, err := NewChallengeStore().Issue(testAddress, time.Minute)
	require.NoError(t, err)
	// EIP-4361: nonce = 8*( ALPHA / DIGIT ).
	assert.Regexp(t, regexp.MustCompile(`^[0-9a-f]{8,}$`), challenge.Value)
	assert.Equal(t, testAddress, challenge.Address)
}

func TestChallengeStore_VerifyBindsAddressKeyAndExpiry(t *testing.T) {
	store := NewChallengeStore()
	challenge, err := store.Issue(testAddress, time.Minute)
	require.NoError(t, err)

	_, ok := store.Verify(challenge.Value, strings.ToUpper(testAddress[:2])+strings.ToUpper(testAddress[2:]))
	assert.True(t, ok, "address comparison is case-insensitive")

	_, ok = store.Verify(challenge.Value, "0x0000000000000000000000000000000000000001")
	assert.False(t, ok, "bound to the address it was issued for")

	_, ok = NewChallengeStore().Verify(challenge.Value, testAddress)
	assert.False(t, ok, "another key (another process) did not issue it")

	tampered := []byte(challenge.Value)
	tampered[3] ^= 1 // flip a bit in the expiry
	_, ok = store.Verify(string(tampered), testAddress)
	assert.False(t, ok, "the expiry is covered by the MAC")

	for _, junk := range []string{"", "abc", challenge.Value[:len(challenge.Value)-2], challenge.Value + "00", strings.Repeat("zz", 40)} {
		_, ok = store.Verify(junk, testAddress)
		assert.False(t, ok, junk)
	}

	expired, err := store.Issue(testAddress, -time.Second)
	require.NoError(t, err)
	_, ok = store.Verify(expired.Value, testAddress)
	assert.False(t, ok, "expired")
}

func TestChallengeStore_RedeemIsSingleUse(t *testing.T) {
	store := NewChallengeStore()
	challenge, err := store.Issue(testAddress, time.Minute)
	require.NoError(t, err)
	verified, ok := store.Verify(challenge.Value, testAddress)
	require.True(t, ok)

	var wins atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if store.Redeem(verified) {
				wins.Add(1)
			}
		}()
	}
	wg.Wait()
	assert.Equal(t, int32(1), wins.Load(), "exactly one concurrent redeem wins")

	_, ok = store.Verify(challenge.Value, testAddress)
	assert.False(t, ok, "spent")
	_, ok = store.Verify(strings.ToUpper(challenge.Value), testAddress)
	assert.False(t, ok, "an upper-cased copy of a spent nonce is the same nonce")
}

func TestChallengeStore_CleanupForgetsOnlyExpiredRedemptions(t *testing.T) {
	store := NewChallengeStore()
	live, err := store.Issue(testAddress, time.Minute)
	require.NoError(t, err)
	require.True(t, store.Redeem(live))
	require.True(t, store.Redeem(Challenge{Value: "lapsed", ExpiresAt: time.Now().Add(20 * time.Millisecond)}))
	require.Equal(t, 2, store.Len())

	time.Sleep(30 * time.Millisecond)
	store.Cleanup()
	assert.Equal(t, 1, store.Len())
	assert.False(t, store.Redeem(live), "a live redemption is still remembered")
	assert.False(t, store.Redeem(Challenge{Value: "lapsed", ExpiresAt: time.Now().Add(-time.Second)}), "expired challenges never redeem")
}

// TestChallengeStore_RedeemedSetIsCapped: the redeemed set never exceeds its
// cap. Expired entries are purged first; if the set is still full of live
// entries, the oldest are evicted behind an expiry floor, so a flood of
// redemptions cannot refuse every later sign-in and an evicted nonce can never
// be replayed.
func TestChallengeStore_RedeemedSetIsCapped(t *testing.T) {
	store := NewChallengeStore()
	store.SetMaxRedeemed(2)
	soon := time.Now().Add(20 * time.Millisecond)
	later := time.Now().Add(time.Minute)
	latest := time.Now().Add(2 * time.Minute)

	require.NoError(t, store.TryRedeem(Challenge{Value: "a", ExpiresAt: soon}))
	require.NoError(t, store.TryRedeem(Challenge{Value: "b", ExpiresAt: later}))
	time.Sleep(30 * time.Millisecond)
	require.NoError(t, store.TryRedeem(Challenge{Value: "c", ExpiresAt: later}),
		"expired entries are purged on demand when the cap is reached")
	require.Equal(t, 2, store.Len())
	require.ErrorIs(t, store.TryRedeem(Challenge{Value: "b", ExpiresAt: later}), ErrChallengeReplayed,
		"live redemptions survive the purge")

	// Full of live entries: a newer challenge still redeems by evicting the
	// oldest, and the evicted nonces stay unusable.
	require.NoError(t, store.TryRedeem(Challenge{Value: "d", ExpiresAt: latest}))
	require.LessOrEqual(t, store.Len(), 2)
	require.ErrorIs(t, store.TryRedeem(Challenge{Value: "b", ExpiresAt: later}), ErrChallengeReplayed,
		"an evicted nonce is below the floor and can never be replayed")
	require.ErrorIs(t, store.TryRedeem(Challenge{Value: "e", ExpiresAt: later}), ErrChallengeReplayed,
		"an unseen nonce expiring at or before the floor is treated as expired")
	require.ErrorIs(t, store.TryRedeem(Challenge{Value: "d", ExpiresAt: latest}), ErrChallengeReplayed)
}

func TestChallengeStore_DefaultCap(t *testing.T) {
	store := NewChallengeStore()
	require.Equal(t, DefaultMaxRedeemedChallenges, store.maxRedeemed)
	store.SetMaxRedeemed(0)
	require.Equal(t, DefaultMaxRedeemedChallenges, store.maxRedeemed)
}

// BenchmarkChallengeIssueVerifyRedeem is one wallet sign-in's challenge work:
// mint, check the signed nonce, mark it spent.
func BenchmarkChallengeIssueVerifyRedeem(b *testing.B) {
	store := NewChallengeStore()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		challenge, err := store.Issue(testAddress, time.Minute)
		if err != nil {
			b.Fatal(err)
		}
		verified, ok := store.Verify(challenge.Value, testAddress)
		if !ok || !store.Redeem(verified) {
			b.Fatal("challenge must verify and redeem once")
		}
	}
}

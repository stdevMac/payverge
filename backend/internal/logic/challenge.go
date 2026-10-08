package logic

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Wallet sign-in challenges are stateless. The nonce handed to the wallet is
//
//	hex( expiry(8, big-endian unix seconds) || random(16) || mac(16) )
//
// where mac = HMAC-SHA256(key, "wallet-challenge:v1" || address || expiry ||
// random), truncated to 16 bytes. Issuing a challenge stores nothing, so no
// number of /auth/challenge calls can fill server memory or crowd out another
// client: an earlier design kept every pending challenge in a capped map and
// refused all wallet sign-ins once a botnet had filled it.
//
// Single use is enforced by recording a nonce only when it is redeemed, which
// callers do after the wallet signature has verified. A record lives until the
// nonce itself expires, so the redeemed set grows only with successful
// signatures (behind the per-client auth limiter) and drains within one TTL.
// The set is also hard-capped (DefaultMaxRedeemedChallenges): a distributed
// attacker can mint unlimited throwaway keys, so without a cap its size would
// be bounded only by the sign-in rate times the TTL. When the cap is reached,
// expired entries are purged inline; if the set is still full, the entries
// with the earliest expiry are evicted and an expiry floor is raised to the
// latest evicted expiry. Verify and Redeem treat any nonce expiring at or
// before the floor as expired, so an evicted nonce can never be replayed.
// Under a flood this only shortens the effective challenge lifetime (a client
// whose challenge predates the floor asks for a new one) instead of refusing
// every wallet sign-in until the flood's entries expire.
//
// The MAC key is random per process: a restart invalidates outstanding
// challenges, exactly as the in-memory map did, and no configured secret is
// needed. Like the rest of the in-process auth state this assumes a single
// backend replica.
const (
	challengeExpiryBytes = 8
	challengeRandomBytes = 16
	challengeMACBytes    = 16
	challengeNonceBytes  = challengeExpiryBytes + challengeRandomBytes + challengeMACBytes
	challengeMACDomain   = "wallet-challenge:v1"

	// DefaultMaxRedeemedChallenges caps the redeemed-nonce set. One entry is
	// a 80-byte key plus a time.Time, so the cap bounds the set to a few tens
	// of MB; with the 5-minute wallet challenge TTL it allows well over 1,000
	// successful wallet sign-ins per second before it starts evicting.
	DefaultMaxRedeemedChallenges = 500_000
)

// ErrChallengeReplayed means the challenge was already redeemed or has
// expired since Verify.
var ErrChallengeReplayed = errors.New("wallet challenge already redeemed or expired")

type Challenge struct {
	// Value is the nonce the wallet signs (hex, so it satisfies the EIP-4361
	// alphanumeric nonce rule).
	Value string
	// Address is the lower-case wallet address the nonce was issued for.
	Address   string
	ExpiresAt time.Time
}

type ChallengeStore struct {
	key         []byte
	mu          sync.Mutex
	redeemed    map[string]time.Time // nonce -> its expiry
	maxRedeemed int
	// floor: nonces expiring at or before it are rejected (see eviction above).
	floor time.Time
}

// NewChallengeStore returns a store with a fresh random MAC key.
func NewChallengeStore() *ChallengeStore {
	key := make([]byte, sha256.Size)
	if _, err := rand.Read(key); err != nil {
		// crypto/rand does not fail on supported platforms (Go 1.24+ crashes
		// instead of returning an error); a store without a key would accept
		// forged nonces, so never continue.
		panic(fmt.Sprintf("wallet challenge key: %v", err))
	}
	return &ChallengeStore{key: key, redeemed: make(map[string]time.Time), maxRedeemed: DefaultMaxRedeemedChallenges}
}

// SetMaxRedeemed overrides the redeemed-nonce cap (tests). n <= 0 restores
// the default.
func (cs *ChallengeStore) SetMaxRedeemed(n int) {
	if n <= 0 {
		n = DefaultMaxRedeemedChallenges
	}
	cs.mu.Lock()
	cs.maxRedeemed = n
	cs.mu.Unlock()
}

func (cs *ChallengeStore) mac(address string, payload []byte) []byte {
	m := hmac.New(sha256.New, cs.key)
	m.Write([]byte(challengeMACDomain))
	m.Write([]byte{0})
	m.Write([]byte(address))
	m.Write([]byte{0})
	m.Write(payload)
	return m.Sum(nil)[:challengeMACBytes]
}

// Issue mints a challenge for address that expires after ttl. Nothing is
// stored.
func (cs *ChallengeStore) Issue(address string, ttl time.Duration) (Challenge, error) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return Challenge{}, fmt.Errorf("challenge address is empty")
	}
	expiresAt := time.Unix(time.Now().Add(ttl).Unix(), 0)

	var raw [challengeNonceBytes]byte
	binary.BigEndian.PutUint64(raw[:challengeExpiryBytes], uint64(expiresAt.Unix()))
	if _, err := rand.Read(raw[challengeExpiryBytes : challengeExpiryBytes+challengeRandomBytes]); err != nil {
		return Challenge{}, fmt.Errorf("challenge nonce: %w", err)
	}
	copy(raw[challengeExpiryBytes+challengeRandomBytes:], cs.mac(address, raw[:challengeExpiryBytes+challengeRandomBytes]))

	return Challenge{Value: hex.EncodeToString(raw[:]), Address: address, ExpiresAt: expiresAt}, nil
}

// Verify reports whether nonce was issued by this store for address (compared
// case-insensitively), has not expired and has not been redeemed. It records
// nothing: call Redeem once the wallet signature over the message has been
// checked.
func (cs *ChallengeStore) Verify(nonce, address string) (Challenge, bool) {
	if len(nonce) != hex.EncodedLen(challengeNonceBytes) {
		return Challenge{}, false
	}
	raw, err := hex.DecodeString(nonce)
	if err != nil {
		return Challenge{}, false
	}
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return Challenge{}, false
	}
	payload := raw[:challengeExpiryBytes+challengeRandomBytes]
	if !hmac.Equal(raw[challengeExpiryBytes+challengeRandomBytes:], cs.mac(address, payload)) {
		return Challenge{}, false
	}
	expiresAt := time.Unix(int64(binary.BigEndian.Uint64(raw[:challengeExpiryBytes])), 0)
	if !time.Now().Before(expiresAt) {
		return Challenge{}, false
	}
	// Normalise to the canonical (lower-case hex) spelling so an upper-cased
	// copy of a redeemed nonce cannot pass as a new one.
	canonical := hex.EncodeToString(raw)
	cs.mu.Lock()
	_, used := cs.redeemed[canonical]
	evicted := !expiresAt.After(cs.floor)
	cs.mu.Unlock()
	if used || evicted {
		return Challenge{}, false
	}
	return Challenge{Value: canonical, Address: address, ExpiresAt: expiresAt}, true
}

// Redeem marks a challenge returned by Verify as spent. It reports false when
// the challenge was already redeemed (a concurrent or replayed sign-in) or has
// expired since Verify; exactly one of several concurrent callers wins.
func (cs *ChallengeStore) Redeem(challenge Challenge) bool {
	return cs.TryRedeem(challenge) == nil
}

// TryRedeem is Redeem with a reason: ErrChallengeReplayed for a spent or
// expired challenge. A redeemed set at its cap evicts its oldest entries and
// raises the expiry floor, so it never refuses a fresh challenge.
func (cs *ChallengeStore) TryRedeem(challenge Challenge) error {
	now := time.Now()
	if challenge.Value == "" || !now.Before(challenge.ExpiresAt) {
		return ErrChallengeReplayed
	}
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if _, used := cs.redeemed[challenge.Value]; used {
		return ErrChallengeReplayed
	}
	if !challenge.ExpiresAt.After(cs.floor) {
		return ErrChallengeReplayed
	}
	limit := cs.maxRedeemed
	if limit <= 0 {
		limit = DefaultMaxRedeemedChallenges
	}
	if len(cs.redeemed) >= limit {
		cs.cleanupLocked(now)
		if len(cs.redeemed) >= limit {
			cs.evictOldestLocked(limit)
			if !challenge.ExpiresAt.After(cs.floor) {
				return ErrChallengeReplayed
			}
		}
	}
	cs.redeemed[challenge.Value] = challenge.ExpiresAt
	return nil
}

// evictOldestLocked drops the redeemed entries with the earliest expiry until
// the set holds at most 90% of limit (amortising the sort), and raises the
// floor to the latest evicted expiry so none of them can be replayed. Entries
// that share the floor's expiry are evicted together, so the floor never
// rejects a nonce that is still remembered.
func (cs *ChallengeStore) evictOldestLocked(limit int) {
	target := limit - limit/10
	if target >= limit {
		target = limit - 1
	}
	expiries := make([]time.Time, 0, len(cs.redeemed))
	for _, exp := range cs.redeemed {
		expiries = append(expiries, exp)
	}
	sort.Slice(expiries, func(i, j int) bool { return expiries[i].Before(expiries[j]) })
	drop := len(expiries) - target
	if drop <= 0 {
		return
	}
	floor := expiries[drop-1]
	if floor.After(cs.floor) {
		cs.floor = floor
	}
	for nonce, exp := range cs.redeemed {
		if !exp.After(cs.floor) {
			delete(cs.redeemed, nonce)
		}
	}
}

// Len reports the number of redeemed nonces still remembered.
func (cs *ChallengeStore) Len() int {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	return len(cs.redeemed)
}

// Cleanup forgets redeemed nonces whose expiry has passed; Verify rejects them
// on expiry alone from then on.
func (cs *ChallengeStore) Cleanup() {
	now := time.Now()
	cs.mu.Lock()
	defer cs.mu.Unlock()
	cs.cleanupLocked(now)
}

func (cs *ChallengeStore) cleanupLocked(now time.Time) {
	for nonce, expiresAt := range cs.redeemed {
		if !now.Before(expiresAt) {
			delete(cs.redeemed, nonce)
		}
	}
}

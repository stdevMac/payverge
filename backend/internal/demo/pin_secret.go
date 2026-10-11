package demo

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/bcrypt"

	"github.com/stdevmac/payverge/backend/internal/logger"
)

// demoPINKeyLabel domain-separates the demo staff PIN key from every other
// use of PLUGIN_SECRET_KEY. Bump the version to rotate every demo PIN.
const demoPINKeyLabel = "payverge-demo-staff-pin-v6"

var (
	demoPINKeyOnce sync.Once
	demoPINKeyVal  []byte
)

// demoPINKey returns the server-side key demo staff PINs are derived with
// (M-pin). PINs used to be sha256("demo-pin-v5:"+email): anyone who knew a
// demo staff email (they are shown in the Demo Center and follow a fixed
// pattern) could compute the PIN of every demo staff member on every
// instance, including a public demo.
//
// The key is HMAC-SHA256(PLUGIN_SECRET_KEY, label), so it is stable across
// restarts and never shared between instances. Without PLUGIN_SECRET_KEY (a
// local dev instance; production refuses to start without it) a random
// per-process key is used: PINs then change on restart, and the Demo Center
// only shows a PIN hint that still matches the stored bcrypt hash.
func demoPINKey() []byte {
	demoPINKeyOnce.Do(func() {
		secret := strings.TrimSpace(os.Getenv("PLUGIN_SECRET_KEY"))
		if secret != "" {
			mac := hmac.New(sha256.New, []byte(secret))
			mac.Write([]byte(demoPINKeyLabel))
			demoPINKeyVal = mac.Sum(nil)
			return
		}
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			panic(fmt.Sprintf("demo: cannot generate staff PIN key: %v", err))
		}
		demoPINKeyVal = key
		logger.Logger.Warn("demo: PLUGIN_SECRET_KEY is not set; demo staff PINs use a per-process random key and change on restart")
	})
	return demoPINKeyVal
}

// keyedDemoPIN maps (email, attempt) to a 4-digit PIN in 1000..9999 with the
// server key. attempt 0 is the primary PIN; collisions within a roster walk
// attempt 1, 2, ...
func keyedDemoPIN(key []byte, email string, attempt int) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte("pin:" + strings.ToLower(strings.TrimSpace(email))))
	if attempt > 0 {
		mac.Write([]byte(fmt.Sprintf(":%d", attempt)))
	}
	sum := mac.Sum(nil)
	return fmt.Sprintf("%04d", binary.BigEndian.Uint32(sum[:4])%9000+1000)
}

// demoPINMatchesHash reports whether pin is the PIN stored as pinHash. The
// Demo Center uses it so a re-derived hint is only surfaced when it still
// opens the staff login (seeded PINs use bcrypt.MinCost, so this is cheap).
func demoPINMatchesHash(pinHash, pin string) bool {
	if pinHash == "" || pin == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(pinHash), []byte(pin)) == nil
}

// Package auththrottle is a DB-backed failed-attempt counter with lockout and
// exponential backoff, keyed by (principal, kind). State lives in auth_attempts
// so it survives restarts and is shared across replicas —
// unlike the in-memory auth.RateLimiter.
package auththrottle

import (
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// Config tunes the throttle. MaxAttempts failures within Window trip a lock for
// BaseLockout; each additional failure past the lock doubles the lockout up to
// MaxLockout.
type Config struct {
	MaxAttempts int
	Window      time.Duration
	BaseLockout time.Duration
	MaxLockout  time.Duration
}

func (c Config) withDefaults() Config {
	if c.MaxAttempts <= 0 {
		c.MaxAttempts = 5
	}
	if c.Window <= 0 {
		c.Window = 15 * time.Minute
	}
	if c.BaseLockout <= 0 {
		c.BaseLockout = time.Minute
	}
	if c.MaxLockout <= 0 {
		c.MaxLockout = time.Hour
	}
	return c
}

// Throttle records failures and reports lockout state.
type Throttle struct {
	db  *gorm.DB
	cfg Config
}

// New binds a Throttle to db with cfg (zero fields fall back to safe defaults).
func New(db *gorm.DB, cfg Config) *Throttle {
	return &Throttle{db: db, cfg: cfg.withDefaults()}
}

func norm(principal string) string { return strings.ToLower(strings.TrimSpace(principal)) }

// Record registers one failed attempt for (principal, kind). If the rolling
// window has expired the counter resets; once Count reaches MaxAttempts a lock
// is set with exponential backoff based on how far Count exceeds MaxAttempts.
func (t *Throttle) Record(principal, kind string) error {
	principal = norm(principal)
	now := time.Now()
	return t.db.Transaction(func(tx *gorm.DB) error {
		var a database.AuthAttempt
		err := tx.Where("principal = ? AND kind = ?", principal, kind).First(&a).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			a = database.AuthAttempt{Principal: principal, Kind: kind, Count: 0, WindowStartedAt: now}
		case err != nil:
			return err
		}
		if now.Sub(a.WindowStartedAt) > t.cfg.Window {
			a.Count = 0
			a.WindowStartedAt = now
			a.LockedUntil = nil
		}
		a.Count++
		if a.Count >= t.cfg.MaxAttempts {
			over := a.Count - t.cfg.MaxAttempts // 0 on first lock, grows after
			lockout := t.cfg.BaseLockout << uint(over)
			if lockout <= 0 || lockout > t.cfg.MaxLockout {
				lockout = t.cfg.MaxLockout
			}
			until := now.Add(lockout)
			a.LockedUntil = &until
		}
		if a.ID == 0 {
			return tx.Create(&a).Error
		}
		return tx.Save(&a).Error
	})
}

// IsLocked reports whether (principal, kind) is currently locked and, if so,
// the time the lock lifts.
func (t *Throttle) IsLocked(principal, kind string) (bool, time.Time, error) {
	principal = norm(principal)
	var a database.AuthAttempt
	err := t.db.Where("principal = ? AND kind = ?", principal, kind).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, time.Time{}, nil
	}
	if err != nil {
		return false, time.Time{}, err
	}
	if a.LockedUntil != nil && a.LockedUntil.After(time.Now()) {
		return true, *a.LockedUntil, nil
	}
	return false, time.Time{}, nil
}

// Clear removes the counter for (principal, kind). Call on a successful login.
func (t *Throttle) Clear(principal, kind string) error {
	principal = norm(principal)
	return t.db.Where("principal = ? AND kind = ?", principal, kind).
		Delete(&database.AuthAttempt{}).Error
}

package verification

import (
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// captureQuerySQL records the SQL of every query-callback statement (Count,
// Find, First, ...) run on db after it is called.
func captureQuerySQL(t *testing.T, db *gorm.DB) func() []string {
	t.Helper()
	var (
		mu   sync.Mutex
		seen []string
	)
	if err := db.Callback().Query().After("gorm:query").Register("test:capture_sql", func(tx *gorm.DB) {
		mu.Lock()
		defer mu.Unlock()
		seen = append(seen, tx.Statement.SQL.String())
	}); err != nil {
		t.Fatal(err)
	}
	return func() []string {
		mu.Lock()
		defer mu.Unlock()
		out := append([]string(nil), seen...)
		seen = nil
		return out
	}
}

// IsOperatorVerified runs on every authenticated email/register request
// (verifyPersistedOperatorSession). Pin its access shape: one COUNT over the
// user_auths/users join, with the verified predicates only when verification
// is required and the email binding always.
func TestIsOperatorVerifiedAccessShape(t *testing.T) {
	for _, tc := range []struct {
		mode            string
		wantVerifiedSQL bool
	}{
		{mode: "required", wantVerifiedSQL: true},
		{mode: "off", wantVerifiedSQL: false},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			t.Setenv("EMAIL_VERIFICATION", tc.mode)
			db := newVerificationTestDB(t)
			user := database.User{Email: "shape@example.com", AuthMethod: "email", Role: "user", EmailVerified: true}
			if err := db.Create(&user).Error; err != nil {
				t.Fatal(err)
			}
			if err := db.Create(&emailAuthState{UserID: user.ID, Provider: "email", ProviderUserID: "shape@example.com", EmailVerified: true, CreatedAt: time.Now()}).Error; err != nil {
				t.Fatal(err)
			}
			drain := captureQuerySQL(t, db)

			ok, err := NewService(db).IsOperatorVerified(user.ID, "email")
			if err != nil || !ok {
				t.Fatalf("IsOperatorVerified = %v, %v; want true, nil", ok, err)
			}
			statements := drain()
			if len(statements) != 1 {
				t.Fatalf("want exactly one statement, got %d: %q", len(statements), statements)
			}
			sql := statements[0]
			if !strings.Contains(sql, "count(") && !strings.Contains(sql, "COUNT(") {
				t.Fatalf("want a COUNT, got %q", sql)
			}
			if strings.Contains(sql, "SELECT *") {
				t.Fatalf("must not hydrate rows: %q", sql)
			}
			if !strings.Contains(sql, "LOWER(u.email) = LOWER(ua.provider_user_id)") {
				t.Fatalf("email binding predicate missing: %q", sql)
			}
			if got := strings.Contains(sql, "email_verified"); got != tc.wantVerifiedSQL {
				t.Fatalf("verified predicates present=%v, want %v: %q", got, tc.wantVerifiedSQL, sql)
			}
		})
	}
}

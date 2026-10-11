package verification

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
)

// BenchmarkIsOperatorVerified measures the live session check that
// verifyPersistedOperatorSession runs on every authenticated email/register
// request. The required sub-benchmark is the pre-existing query shape; the off
// sub-benchmark is the EMAIL_VERIFICATION=off shape (binding only, no verified
// flags), which reads the verification mode from env on each call.
func BenchmarkIsOperatorVerified(b *testing.B) {
	for _, tc := range []struct {
		name     string
		mode     string
		verified bool
	}{
		{name: "required_verified", mode: "required", verified: true},
		{name: "off_unverified", mode: "off", verified: false},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.Setenv("EMAIL_VERIFICATION", tc.mode)
			db := newVerificationTestDB(b)
			user := database.User{Email: "bench@example.com", AuthMethod: "email", Role: "user", EmailVerified: tc.verified}
			if err := db.Create(&user).Error; err != nil {
				b.Fatal(err)
			}
			auth := emailAuthState{UserID: user.ID, Provider: "email", ProviderUserID: "bench@example.com", EmailVerified: tc.verified, CreatedAt: time.Now()}
			if err := db.Create(&auth).Error; err != nil {
				b.Fatal(err)
			}
			svc := NewService(db)
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := svc.IsOperatorVerified(user.ID, "email"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

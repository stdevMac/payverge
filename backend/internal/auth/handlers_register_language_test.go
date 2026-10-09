package auth

import (
	"context"
	"net/http"
	"sync"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/structs"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// subjectCapturingProvider records dispatched messages so tests can assert
// which localized template family actually rendered.
type subjectCapturingProvider struct {
	mu       sync.Mutex
	messages []emails.EmailMessage
}

func (p *subjectCapturingProvider) Send(_ context.Context, m emails.EmailMessage) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.messages = append(p.messages, m)
	return nil
}

func (p *subjectCapturingProvider) LastSubject() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.messages) == 0 {
		return ""
	}
	return p.messages[len(p.messages)-1].Subject
}

func newRegisterLanguageHandler(t *testing.T) (*AuthHandler, *gorm.DB, *subjectCapturingProvider) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	prevSecret := structs.SecretKey
	t.Cleanup(func() { structs.SecretKey = prevSecret })
	structs.SecretKey = []byte("test-secret-key-for-register-language-tests")

	dsn := "file:" + t.Name() + "?mode=memory&cache=shared"
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	require.NoError(t, db.AutoMigrate(&database.User{}, &UserAuth{}))

	provider := &subjectCapturingProvider{}
	es, err := emails.NewEmailServer(provider, "noreply@example.com", "updates@example.com", "../../email/templates")
	require.NoError(t, err)

	return NewAuthHandler(db, es), db, provider
}

// TestRegisterStoresOperatorLanguageAndLocalizesVerification: a signup that
// carries the active operator locale must (a) persist it as
// User.LanguageSelected and (b) send the verification email in that language
// — the first touchpoint of a Spanish funnel must not be English.
func TestRegisterStoresOperatorLanguageAndLocalizesVerification(t *testing.T) {
	h, db, provider := newRegisterLanguageHandler(t)

	w, c := postJSON(t, map[string]string{
		"email":    "es-owner@example.com",
		"password": "password123",
		"name":     "Dueña",
		"language": "es",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code)

	var user database.User
	require.NoError(t, db.Where("email = ?", "es-owner@example.com").First(&user).Error)
	assert.Equal(t, "es", user.LanguageSelected)

	// es email_verification subject (backend/email/templates/es/email_verification.html).
	assert.Equal(t, "Verifica tu correo", provider.LastSubject())
}

// TestRegisterRejectsNonOperatorLanguage: unknown or guest-only codes must not
// be persisted — the user falls back to the English default exactly as today.
func TestRegisterRejectsNonOperatorLanguage(t *testing.T) {
	h, db, provider := newRegisterLanguageHandler(t)

	w, c := postJSON(t, map[string]string{
		"email":    "xx-owner@example.com",
		"password": "password123",
		"name":     "Owner",
		"language": "xx",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code)

	var user database.User
	require.NoError(t, db.Where("email = ?", "xx-owner@example.com").First(&user).Error)
	assert.Equal(t, "", user.LanguageSelected)
	assert.Equal(t, "Verify your email", provider.LastSubject())
}

// TestRegisterWithoutLanguageKeepsEnglishDefault pins today's behavior for
// old clients that send no language field.
func TestRegisterWithoutLanguageKeepsEnglishDefault(t *testing.T) {
	h, db, provider := newRegisterLanguageHandler(t)

	w, c := postJSON(t, map[string]string{
		"email":    "plain@example.com",
		"password": "password123",
		"name":     "Owner",
	})
	h.Register(c)
	require.Equal(t, http.StatusCreated, w.Code)

	var user database.User
	require.NoError(t, db.Where("email = ?", "plain@example.com").First(&user).Error)
	assert.Equal(t, "", user.LanguageSelected)
	assert.Equal(t, "Verify your email", provider.LastSubject())
}

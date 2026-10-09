package server

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/services"
)

func TestNoShowEmailUsesDedicatedTemplateNotCancellation(t *testing.T) {
	// Source-level guard: sendReservationNoShowEmail must call
	// SendReservationNoShowEmail, not SendReservationCancelledEmail.
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	src := filepath.Join(filepath.Dir(thisFile), "reservation_emails.go")
	body, err := os.ReadFile(src)
	require.NoError(t, err)
	text := string(body)

	// Isolate the no-show function body roughly.
	idx := strings.Index(text, "func sendReservationNoShowEmail")
	require.Greater(t, idx, -1)
	// Next function after no-show
	rest := text[idx:]
	next := strings.Index(rest[1:], "\nfunc ")
	if next > 0 {
		rest = rest[:next+1]
	}
	require.Contains(t, rest, "SendReservationNoShowEmail")
	require.NotContains(t, rest, "SendReservationCancelledEmail")
}

func TestNoShowLanguageResolvesFromReservation(t *testing.T) {
	res := &database.TableReservation{Language: "es-AR"}
	biz := &database.Business{DefaultLanguage: "en"}
	require.Equal(t, "es-AR", services.ResolveReservationEmailLanguage(res, biz))
}

func TestNoShowTemplatesExistInAllFamilies(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	// server/ -> internal/ -> backend/
	backendRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	for _, fam := range []string{"eng", "es", "es_ar"} {
		path := filepath.Join(backendRoot, "email", "templates", fam, "reservation_noshow.html")
		body, err := os.ReadFile(path)
		require.NoError(t, err, "missing template %s", path)
		require.Contains(t, string(body), `define "subject"`)
		require.Contains(t, string(body), `define "content"`)
		// Must not look like the cancellation template.
		require.NotContains(t, strings.ToLower(string(body)), "has been cancelled")
		require.NotContains(t, strings.ToLower(string(body)), "fue cancelada")
	}
}

package handlers

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"

	"github.com/stdevmac/payverge/backend/internal/models"
)

// TestTruncate_BoundsWithoutCorruptingUTF8 verifies the shared truncate helper
// is rune-safe: values under the cap pass through untouched, oversized values
// are bounded to the rune count, and multibyte input is never split into invalid
// UTF-8 (which Postgres text columns would reject — a 500 on the public ingests).
func TestTruncate_BoundsWithoutCorruptingUTF8(t *testing.T) {
	assert.Equal(t, "hello", truncate("hello", 100), "under cap is unchanged")
	assert.Equal(t, "abcde", truncate("abcdefghij", 5), "ascii bounded by rune count")

	// 10 multibyte runes (é = 2 bytes each); cap to 4 runes.
	multibyte := strings.Repeat("é", 10)
	got := truncate(multibyte, 4)
	assert.Equal(t, 4, utf8.RuneCountInString(got), "bounded to rune count")
	assert.True(t, utf8.ValidString(got), "must remain valid UTF-8 (no mid-rune split)")
	assert.Equal(t, strings.Repeat("é", 4), got)
}

// TestClampPageViewStrings_BoundsAttackerControlledFields ensures the public
// page-view ingest bounds every caller-supplied string field, matching the
// hardening the sibling error-log and missing-translation ingests already apply.
func TestClampPageViewStrings_BoundsAttackerControlledFields(t *testing.T) {
	big := strings.Repeat("A", 5000)
	pv := &models.PageView{
		SessionID: big, Page: big, Referrer: big, UserAgent: big,
		Country: big, City: big, DeviceType: big, Browser: big, OS: big, Locale: big,
	}

	clampPageViewStrings(pv)

	assert.LessOrEqual(t, len(pv.SessionID), 100)
	assert.LessOrEqual(t, len(pv.Page), 1024)
	assert.LessOrEqual(t, len(pv.Referrer), 1024)
	assert.LessOrEqual(t, len(pv.UserAgent), 512)
	assert.LessOrEqual(t, len(pv.Country), 100)
	assert.LessOrEqual(t, len(pv.City), 200)
	assert.LessOrEqual(t, len(pv.DeviceType), 50)
	assert.LessOrEqual(t, len(pv.Browser), 100)
	assert.LessOrEqual(t, len(pv.OS), 100)
	assert.LessOrEqual(t, len(pv.Locale), 32)

	// A normal-sized page view is left untouched.
	normal := &models.PageView{SessionID: "sess-123", Page: "/menu", Locale: "es-AR"}
	clampPageViewStrings(normal)
	assert.Equal(t, "sess-123", normal.SessionID)
	assert.Equal(t, "/menu", normal.Page)
	assert.Equal(t, "es-AR", normal.Locale)
}

func TestClampInteractionStrings_BoundsFields(t *testing.T) {
	big := strings.Repeat("B", 5000)
	in := &models.UserInteraction{
		SessionID: big, Page: big, EventType: big,
		EventCategory: big, EventLabel: big, EventValue: big,
	}
	clampInteractionStrings(in)
	assert.LessOrEqual(t, len(in.SessionID), 100)
	assert.LessOrEqual(t, len(in.Page), 1024)
	assert.LessOrEqual(t, len(in.EventType), 100)
	assert.LessOrEqual(t, len(in.EventCategory), 100)
	assert.LessOrEqual(t, len(in.EventLabel), 255)
	assert.LessOrEqual(t, len(in.EventValue), 2048)
}

func TestClampConversionStrings_BoundsFields(t *testing.T) {
	big := strings.Repeat("C", 5000)
	ev := &models.ConversionEvent{SessionID: big, ConversionType: big, Metadata: big}
	clampConversionStrings(ev)
	assert.LessOrEqual(t, len(ev.SessionID), 100)
	assert.LessOrEqual(t, len(ev.ConversionType), 100)
	assert.LessOrEqual(t, len(ev.Metadata), 2048)
}

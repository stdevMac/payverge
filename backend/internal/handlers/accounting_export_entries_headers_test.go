package handlers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEntriesCSVHeaders_IncludesNotesAndLocalizes(t *testing.T) {
	en := entriesCSVHeaders("en")
	assert.Contains(t, en, "notes")
	assert.Equal(t, "notes", en[len(en)-1])

	es := entriesCSVHeaders("es")
	assert.Contains(t, es, "notas")
	assert.Equal(t, "notas", es[len(es)-1])
	// Not raw English keys for labels operators see
	assert.NotContains(t, es, "occurred_at")
}

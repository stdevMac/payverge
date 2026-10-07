package aicontract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEffectRecorder_Match(t *testing.T) {
	r := NewEffectRecorder()
	r.Record("support_escalation")
	r.Record("support_escalation")
	r.Record("whatsapp_send")
	require.NoError(t, r.Match(map[string]int{
		"support_escalation": 2,
		"whatsapp_send":      1,
	}))
	require.Error(t, r.Match(map[string]int{"support_escalation": 1}))
}

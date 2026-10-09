//go:build !whatsapp

package aicontract

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Default builds compile out the whatsmeow integration, so the WhatsApp
// scenarios have no production path to exercise. Mark them as tag-gated
// instead of registering a fake-passing runner.
func registerWhatsAppRunners() {
	tagGatedScenarioIDs["whatsapp-restores-after-process-restart"] = "requires -tags whatsapp"
}

// Default builds gate exactly the WhatsApp scenarios; anything else missing a
// runner must still fail TestHermeticMatrix_AllScenariosRegistered.
func TestWhatsAppScenariosTagGatedInDefaultBuild(t *testing.T) {
	require.Equal(t, map[string]string{
		"whatsapp-restores-after-process-restart": "requires -tags whatsapp",
	}, tagGatedScenarioIDs)
}

package agents

import (
	"os"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpsAssistantHasNoOperationalMutationCapability(t *testing.T) {
	source, err := os.ReadFile("ops_finalizer.go")
	require.NoError(t, err)
	text := string(source)

	assert.Contains(t, text, "guideDestinationHref")
	assert.NotContains(t, text, "in.Model.Actions")

	allowedActionTypes := map[string]bool{
		"navigate":               true,
		"create_support_request": true,
	}
	typePattern := regexp.MustCompile(`(?s)assistantcontract\.Action\{.*?Type:\s*"([a-z_]+)"`)
	for _, match := range typePattern.FindAllStringSubmatch(text, -1) {
		assert.True(t, allowedActionTypes[match[1]], "Ops finalizer action type %q is not allowlisted", match[1])
	}

	for _, forbidden := range []string{
		`Type: "external_link"`, `Type: "director_handoff"`, `Type: "add_cart_item"`,
		`Type: "capture_lead"`,
	} {
		assert.NotContains(t, text, forbidden)
	}
}

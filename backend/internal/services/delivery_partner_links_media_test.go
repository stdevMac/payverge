package services

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/stretchr/testify/require"
)

func TestSanitizeExternalPartnerLinksKeepsOwnMediaIcons(t *testing.T) {
	t.Cleanup(s3.SetPublicURL(""))

	raw := json.RawMessage(`[
		{"name":"Local","url":"https://partners.example.com/a","icon_url":"/media/businesses/7/partner-icons/logo.png"},
		{"name":"Bad icon","url":"https://partners.example.com/b","icon_url":"javascript:alert(1)"},
		{"name":"Traversal icon","url":"https://partners.example.com/c","icon_url":"/media/../etc/passwd"},
		{"name":"Relative link","url":"/media/businesses/7/partner-icons/logo.png"}
	]`)

	var links []map[string]any
	require.NoError(t, json.Unmarshal(SanitizeExternalPartnerLinks(raw), &links))
	require.Len(t, links, 3, "a relative /media link URL is not a partner link")
	require.Equal(t, "/media/businesses/7/partner-icons/logo.png", links[0]["icon_url"])
	require.NotContains(t, links[1], "icon_url")
	require.NotContains(t, links[2], "icon_url")
}

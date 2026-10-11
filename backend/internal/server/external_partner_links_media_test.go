package server

import (
	"encoding/json"
	"testing"

	"github.com/stdevmac/payverge/backend/internal/s3"

	"github.com/stretchr/testify/require"
)

// Partner icons uploaded on the local storage driver come back as same-origin
// "/media/<key>" URLs (relative when PUBLIC_URL is unset). They must be
// accepted as icon_url; the partner link itself stays http/https only.
func TestNormalizeAndValidateExternalPartnerLinks_AcceptsOwnMediaIconURL(t *testing.T) {
	t.Cleanup(s3.SetPublicURL(""))

	icon := "/media/businesses/7/partner-icons/0123456789abcdef_logo.png"
	normalized, err := NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
		{"name": "Local Partner", "url": "https://partners.example.com/order", "icon_url": icon},
	})
	require.NoError(t, err)

	var links []ExternalPartnerLink
	require.NoError(t, json.Unmarshal(normalized, &links))
	require.Len(t, links, 1)
	require.Equal(t, icon, links[0].IconURL)

	for _, bad := range []string{
		"javascript:alert(1)",
		"/media/../etc/passwd",
		"//evil.test/media/a.png",
		"/uploads/a.png",
	} {
		_, err := NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
			{"name": "P", "url": "https://partners.example.com/order", "icon_url": bad},
		})
		require.Error(t, err, bad)
	}

	// A relative /media URL is never valid for the partner link itself.
	_, err = NormalizeAndValidateExternalPartnerLinks([]map[string]interface{}{
		{"name": "P", "url": icon},
	})
	require.Error(t, err)
}

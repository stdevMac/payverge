package telegram

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// hardCodedMoneyCopy matches a currency symbol or ISO code glued to a figure
// ("$100", "USD 100", "100 USD"). Plugin config copy is rendered verbatim on
// every venue regardless of `businesses.display_currency`, so any such literal
// is wrong on an ARS/EUR carta.
var hardCodedMoneyCopy = regexp.MustCompile(`(?i)(\$\s*\d|\d\s*(usd|ars|eur|gbp)\b|\b(usd|ars|eur|gbp)\s*\d)`)

// collectSchemaCopy walks the decoded JSON-Schema and returns every human-facing
// string (title / description) keyed by its dotted path.
func collectSchemaCopy(t *testing.T, node interface{}, path string, out map[string]string) {
	t.Helper()
	switch typed := node.(type) {
	case map[string]interface{}:
		for _, key := range []string{"title", "description"} {
			if value, ok := typed[key].(string); ok {
				out[path+"."+key] = value
			}
		}
		for key, value := range typed {
			if key == "title" || key == "description" {
				continue
			}
			collectSchemaCopy(t, value, path+"."+key, out)
		}
	case []interface{}:
		for _, value := range typed {
			collectSchemaCopy(t, value, path, out)
		}
	}
}

// TestTelegramConfigSchema_CopyIsCurrencyNeutral is the #901 gate: the Telegram
// plugin's config schema is served to every tenant, so none of its operator copy
// may hard-code a USD amount.
func TestTelegramConfigSchema_CopyIsCurrencyNeutral(t *testing.T) {
	plugin := &TelegramPlugin{}

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(plugin.GetConfigSchema()), &schema))

	copyByPath := map[string]string{}
	collectSchemaCopy(t, schema, "", copyByPath)
	require.NotEmpty(t, copyByPath, "schema should expose operator-facing copy")

	for path, text := range copyByPath {
		require.False(t, hardCodedMoneyCopy.MatchString(text),
			"telegram config schema copy at %s hard-codes a currency amount (%q); "+
				"the venue currency is per-business, so this is wrong on an ARS carta", path, text)
	}
}

// TestTelegramConfigSchema_HighValueOrdersStillDescribed guards the fix from
// degenerating into an empty description.
func TestTelegramConfigSchema_HighValueOrdersStillDescribed(t *testing.T) {
	plugin := &TelegramPlugin{}

	var schema map[string]interface{}
	require.NoError(t, json.Unmarshal([]byte(plugin.GetConfigSchema()), &schema))

	properties, ok := schema["properties"].(map[string]interface{})
	require.True(t, ok, "schema must expose properties")
	settings, ok := properties["notification_settings"].(map[string]interface{})
	require.True(t, ok, "schema must expose notification_settings")
	settingProperties, ok := settings["properties"].(map[string]interface{})
	require.True(t, ok, "notification_settings must expose properties")
	highValue, ok := settingProperties["high_value_orders"].(map[string]interface{})
	require.True(t, ok, "notification_settings must expose high_value_orders")

	description, _ := highValue["description"].(string)
	require.NotEmpty(t, strings.TrimSpace(description),
		"high_value_orders must keep an operator-readable description")
}

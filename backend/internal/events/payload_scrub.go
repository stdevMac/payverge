package events

import (
	"bytes"
	"encoding/json"
	"strings"
)

// scrubBusinessEventData strips capability fields from bill and order payloads
// before they enter the replay ring or the subscriber fan-out. Other types pass
// through unchanged, including their original bytes.
func scrubBusinessEventData(eventType string, data json.RawMessage) json.RawMessage {
	if strings.HasPrefix(eventType, "bill.") || strings.HasPrefix(eventType, "order.") {
		return scrubCapabilityFields(data)
	}
	return data
}

// scrubCapabilityFields removes guest bearer capabilities and settlement fields
// from a payload. Numbers are preserved exactly. Undecodable input fails closed
// to an empty object.
func scrubCapabilityFields(raw json.RawMessage) json.RawMessage {
	if !containsCapabilityKeyFragment(raw) {
		return raw
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return json.RawMessage("{}")
	}
	scrubCapabilityValue(v)
	out, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("{}")
	}
	return out
}

func containsCapabilityKeyFragment(raw []byte) bool {
	return bytes.Contains(raw, []byte(`"public_token"`)) ||
		bytes.Contains(raw, []byte(`"settlement_address"`)) ||
		bytes.Contains(raw, []byte(`"tipping_address"`)) ||
		bytes.Contains(raw, []byte(`"confirmed_by"`)) ||
		bytes.Contains(raw, []byte(`"fiscal_`))
}

func scrubCapabilityValue(v any) {
	switch n := v.(type) {
	case map[string]any:
		for k, child := range n {
			if capabilityKey(k) {
				delete(n, k)
				continue
			}
			scrubCapabilityValue(child)
		}
	case []any:
		for _, child := range n {
			scrubCapabilityValue(child)
		}
	}
}

func capabilityKey(k string) bool {
	switch k {
	case "public_token", "settlement_address", "tipping_address", "confirmed_by":
		return true
	default:
		return strings.HasPrefix(k, "fiscal_")
	}
}

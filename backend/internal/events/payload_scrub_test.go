package events

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestPublishScrubsBillAndOrderCapabilityFields(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}
	ch, _, cancel := hub.SubscribeWithReplayTopic(42, 0)
	defer cancel()

	payload := json.RawMessage(`{"bill":{"id":1,"public_token":"tok_secret","settlement_address":"0xabc","tipping_address":"0xdef","fiscal_cae":"123","fiscal_status":"ok","confirmed_by":7,"total":12.5,"sequence":9007199254740993,"items":[{"public_token":"x"}]}}`)
	for _, eventType := range []string{"bill.updated", "order.created"} {
		hub.Publish(BusinessEvent{
			BusinessID: 42,
			Type:       eventType,
			Data:       append(json.RawMessage(nil), payload...),
			Timestamp:  time.Now(),
		})
	}

	for _, eventType := range []string{"bill.updated", "order.created"} {
		select {
		case got := <-ch:
			require.Equal(t, eventType, got.Type)
			assertCapabilityFieldsScrubbed(t, got.Data)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %s", eventType)
		}
	}

	replayed := hub.ReplayAfter(42, 0)
	require.Len(t, replayed, 2)
	require.Equal(t, "bill.updated", replayed[0].Type)
	require.Equal(t, "order.created", replayed[1].Type)
	for _, event := range replayed {
		assertCapabilityFieldsScrubbed(t, event.Data)
	}
}

func TestPublishLeavesNonBillOrderPayloadUntouched(t *testing.T) {
	hub := &Hub{subscribers: make(map[uint][]*subscriber)}
	ch, _, cancel := hub.SubscribeWithReplayTopic(7, 0)
	defer cancel()

	raw := json.RawMessage(`{"id":9,"public_token":"tok_secret","fiscal_cae":"123","total":12.5}`)
	hub.Publish(BusinessEvent{
		BusinessID: 7,
		Type:       "reservation.new",
		Data:       raw,
		Timestamp:  time.Now(),
	})

	select {
	case got := <-ch:
		require.Equal(t, "reservation.new", got.Type)
		require.True(t, bytes.Equal(raw, got.Data), "delivered payload = %s", got.Data)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for reservation.new")
	}

	replayed := hub.ReplayAfter(7, 0)
	require.Len(t, replayed, 1)
	require.True(t, bytes.Equal(raw, replayed[0].Data), "replay payload = %s", replayed[0].Data)
}

func TestScrubCapabilityFieldsFastPathReturnsIdenticalBytes(t *testing.T) {
	raw := json.RawMessage(" {\"id\": 1, \"total\": 12.50, \"items\": [{\"name\": \"soup\"}] } ")
	out := scrubCapabilityFields(raw)
	require.True(t, bytes.Equal(raw, out), "fast path rewrote payload: %s", out)
}

func TestScrubCapabilityFieldsFailClosed(t *testing.T) {
	out := scrubCapabilityFields(json.RawMessage(`{"public_token":`))
	require.True(t, bytes.Equal([]byte("{}"), out), "got %s", out)
}

func assertCapabilityFieldsScrubbed(t *testing.T, raw json.RawMessage) {
	t.Helper()
	text := string(raw)
	for _, key := range []string{`"public_token"`, `"settlement_address"`, `"tipping_address"`, `"confirmed_by"`, `"fiscal_`} {
		require.NotContains(t, text, key)
	}
	for _, secret := range []string{"tok_secret", "0xabc", "0xdef"} {
		require.NotContains(t, text, secret)
	}
	require.Contains(t, text, `"id"`)
	require.Contains(t, text, `"total"`)
	require.Contains(t, text, "12.5")
	require.Contains(t, text, "9007199254740993")
	require.NotContains(t, text, `"12.5"`)
}

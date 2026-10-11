package aiwaiterevents

import (
	"testing"
	"time"

	"github.com/stdevmac/payverge/backend/internal/assistantcontract"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHubPreservesAIWaiterV2Payload(t *testing.T) {
	hub := NewHub()
	ch, cancel, err := hub.Subscribe(9)
	require.NoError(t, err)
	defer cancel()

	response := assistantcontract.NewResponse("waiter-response-v2", "Grounded answer")
	response.Answer.Format = assistantcontract.FormatPlainText
	hub.PublishMessage(9, MessageEvent{
		ID: 9, Role: "assistant", Content: response.Answer.Content, ResponseV2: &response,
	})

	select {
	case got := <-ch:
		require.NotNil(t, got.ResponseV2)
		assert.Equal(t, response, *got.ResponseV2)
	case <-time.After(time.Second):
		t.Fatal("subscriber should receive the structured waiter response")
	}
}

func TestHubScopesMatchingSessionTokensByResolvedConversationID(t *testing.T) {
	hub := NewHub()
	// The authenticated HTTP layer may resolve the same opaque session token in
	// two different business scopes. Their conversation IDs remain distinct.
	conversationA, cancelA, err := hub.Subscribe(101)
	require.NoError(t, err)
	defer cancelA()
	conversationB, cancelB, err := hub.Subscribe(202)
	require.NoError(t, err)
	defer cancelB()

	hub.PublishMessage(101, MessageEvent{ID: 11, Role: "assistant", Content: "business A only"})
	require.Len(t, conversationA, 1)
	assert.Empty(t, conversationB, "a different business/conversation with the same token must not receive the event")
}

func TestHubDeepCopiesStructuredResponsePerSubscriber(t *testing.T) {
	hub := NewHub()
	first, cancelFirst, err := hub.Subscribe(303)
	require.NoError(t, err)
	defer cancelFirst()
	second, cancelSecond, err := hub.Subscribe(303)
	require.NoError(t, err)
	defer cancelSecond()

	response := assistantcontract.NewResponse("copy-response", "Original answer")
	response.Notices = []assistantcontract.Notice{{ID: "notice", Kind: "safety", Message: "Original notice"}}
	hub.PublishMessage(303, MessageEvent{ID: 12, Role: "assistant", Content: response.Answer.Content, ResponseV2: &response})
	response.Answer.Content = "mutated original"
	response.Notices[0].Message = "mutated original notice"

	firstEvent := <-first
	firstEvent.ResponseV2.Answer.Content = "mutated subscriber"
	firstEvent.ResponseV2.Notices[0].Message = "mutated subscriber notice"
	secondEvent := <-second
	require.NotNil(t, secondEvent.ResponseV2)
	assert.Equal(t, "Original answer", secondEvent.ResponseV2.Answer.Content)
	assert.Equal(t, "Original notice", secondEvent.ResponseV2.Notices[0].Message)
}

func TestHubDropsStructuredResponseForNonAssistantRoles(t *testing.T) {
	hub := NewHub()
	ch, cancel, err := hub.Subscribe(404)
	require.NoError(t, err)
	defer cancel()
	response := assistantcontract.NewResponse("not-an-assistant-response", "must not attach")
	hub.PublishMessage(404, MessageEvent{ID: 13, Role: "user", Content: "guest", ResponseV2: &response})
	got := <-ch
	assert.Nil(t, got.ResponseV2)
}

func TestHubValidatesAssistantV2AndUsesItsCanonicalContent(t *testing.T) {
	hub := NewHub()
	ch, cancel, err := hub.Subscribe(405)
	require.NoError(t, err)
	defer cancel()
	valid := assistantcontract.NewResponse("canonical-event", "Canonical answer")
	hub.PublishMessage(405, MessageEvent{ID: 14, Role: "assistant", Content: "stale answer", ResponseV2: &valid})
	got := <-ch
	require.NotNil(t, got.ResponseV2)
	assert.Equal(t, "Canonical answer", got.Content)

	invalid := valid
	invalid.Version = 1
	hub.PublishMessage(405, MessageEvent{ID: 15, Role: "assistant", Content: "safe legacy", ResponseV2: &invalid})
	got = <-ch
	assert.Nil(t, got.ResponseV2)
	assert.Equal(t, "safe legacy", got.Content)
}

func TestHubDeliversToMatchingSessionOnly(t *testing.T) {
	hub := NewHub()

	chA, cancelA, err := hub.Subscribe(1)
	require.NoError(t, err)
	defer cancelA()
	chB, cancelB, err := hub.Subscribe(2)
	require.NoError(t, err)
	defer cancelB()

	hub.PublishMessage(1, MessageEvent{ID: 7, Role: "assistant", Content: "hi"})

	select {
	case got := <-chA:
		assert.Equal(t, uint(7), got.ID)
		assert.Equal(t, "assistant", got.Role)
		assert.Equal(t, "hi", got.Content)
	default:
		t.Fatal("subscriber A should have received the event")
	}

	select {
	case <-chB:
		t.Fatal("subscriber B must NOT receive another session's event")
	default:
	}
}

func TestHubNonBlockingDropOnFullBuffer(t *testing.T) {
	hub := NewHub()
	ch, cancel, err := hub.Subscribe(3)
	require.NoError(t, err)
	defer cancel()

	for i := 0; i < subscriberBufferSize+5; i++ {
		hub.PublishMessage(3, MessageEvent{ID: uint(i)})
	}
	require.Len(t, ch, subscriberBufferSize)
}

func TestHubCancelStopsDelivery(t *testing.T) {
	hub := NewHub()
	ch, cancel, err := hub.Subscribe(4)
	require.NoError(t, err)
	cancel()

	hub.PublishMessage(4, MessageEvent{ID: 1})

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("cancelled subscriber must not receive events")
		}
	default:
	}
}

func TestSubscribeLimitedRejectsFifthSubscriberOnConversation(t *testing.T) {
	hub := NewHubWithLimits(4, 10, 100)
	cancels := make([]func(), 0, 5)
	t.Cleanup(func() {
		for _, cancel := range cancels {
			cancel()
		}
	})
	for i := 0; i < 4; i++ {
		_, cancel, ok := hub.SubscribeLimited(42, "203.0.113.1")
		require.True(t, ok)
		cancels = append(cancels, cancel)
	}
	_, _, ok := hub.SubscribeLimited(42, "203.0.113.2")
	assert.False(t, ok, "5th subscriber on one conversation must be rejected")

	_, cancel, ok := hub.SubscribeLimited(43, "203.0.113.2")
	require.True(t, ok, "a different conversation still has room")
	cancels = append(cancels, cancel)
}

func TestSubscribeLimitedPerIPCapAcrossConversations(t *testing.T) {
	hub := NewHubWithLimits(4, 10, 100)
	cancels := make([]func(), 0, 12)
	t.Cleanup(func() {
		for _, cancel := range cancels {
			cancel()
		}
	})
	for i := 0; i < 10; i++ {
		_, cancel, ok := hub.SubscribeLimited(uint(i+1), "203.0.113.9")
		require.True(t, ok)
		cancels = append(cancels, cancel)
	}
	_, _, ok := hub.SubscribeLimited(11, "203.0.113.9")
	assert.False(t, ok, "11th subscriber from the same IP must be rejected")

	_, cancel, ok := hub.SubscribeLimited(11, "203.0.113.10")
	require.True(t, ok, "a different IP is a different bucket")
	cancels = append(cancels, cancel)

	_, cancel, ok = hub.SubscribeLimited(12, "")
	require.True(t, ok, "an empty client IP skips the per-IP cap")
	cancels = append(cancels, cancel)
}

func TestSubscribeLimitedCancelFreesSlot(t *testing.T) {
	hub := NewHubWithLimits(4, 10, 100)
	cancels := make([]func(), 0, 4)
	for i := 0; i < 4; i++ {
		_, cancel, ok := hub.SubscribeLimited(7, "198.51.100.7")
		require.True(t, ok)
		cancels = append(cancels, cancel)
	}
	_, _, ok := hub.SubscribeLimited(7, "198.51.100.8")
	require.False(t, ok)

	cancels[0]()
	_, cancel, ok := hub.SubscribeLimited(7, "198.51.100.8")
	require.True(t, ok, "cancel must free the conversation slot")
	cancel()
	for _, c := range cancels[1:] {
		c()
	}

	hub.mu.Lock()
	assert.Equal(t, 0, hub.total)
	assert.Equal(t, 0, hub.perIP["198.51.100.7"])
	hub.mu.Unlock()
}

func TestSubscribeLimitedCancelTwiceDoesNotDoubleDecrement(t *testing.T) {
	hub := NewHubWithLimits(4, 10, 100)
	_, cancel, ok := hub.SubscribeLimited(8, "203.0.113.8")
	require.True(t, ok)
	cancel()
	cancel()

	hub.mu.Lock()
	total := hub.total
	ipCount := hub.perIP["203.0.113.8"]
	hub.mu.Unlock()
	assert.Equal(t, 0, total)
	assert.Equal(t, 0, ipCount)

	cancels := make([]func(), 0, 4)
	t.Cleanup(func() {
		for _, c := range cancels {
			c()
		}
	})
	for i := 0; i < 4; i++ {
		_, c, ok := hub.SubscribeLimited(8, "")
		require.True(t, ok, "slot %d should be free after a single cancel", i)
		cancels = append(cancels, c)
	}
	_, _, ok = hub.SubscribeLimited(8, "")
	assert.False(t, ok, "double cancel must not free an extra slot")
}

func TestNewHubProductionCapsAllowVenueNAT(t *testing.T) {
	hub := NewHub()
	if hub.maxPerConversation != 4 || hub.maxPerIP != 60 || hub.maxTotal != 2000 {
		t.Fatalf("caps = %d/%d/%d, want 4/60/2000", hub.maxPerConversation, hub.maxPerIP, hub.maxTotal)
	}
	// 15 guests on one venue IP, each with 4 tabs open, all fit.
	for conv := uint(1); conv <= 15; conv++ {
		for i := 0; i < 4; i++ {
			if _, _, ok := hub.SubscribeLimited(conv, "203.0.113.7"); !ok {
				t.Fatalf("conversation %d stream %d rejected under venue NAT", conv, i)
			}
		}
	}
	if _, _, ok := hub.SubscribeLimited(16, "203.0.113.7"); ok {
		t.Fatal("61st stream from one IP was accepted")
	}
}

func TestHubPerConversationLimit(t *testing.T) {
	hub := NewHubWithLimits(2, 3, 3)
	_, cancel1, err := hub.Subscribe(1)
	require.NoError(t, err)
	_, cancel2, err := hub.Subscribe(1)
	require.NoError(t, err)
	defer cancel2()

	_, _, err = hub.Subscribe(1)
	require.ErrorIs(t, err, ErrLimit)

	cancel1()
	_, cancel3, err := hub.Subscribe(1)
	require.NoError(t, err)
	defer cancel3()

	// A second cancel must not free another slot.
	cancel1()
	_, _, err = hub.Subscribe(1)
	require.ErrorIs(t, err, ErrLimit)
}

func TestHubGlobalLimit(t *testing.T) {
	hub := NewHubWithLimits(2, 3, 3)
	_, cancel1, err := hub.Subscribe(1)
	require.NoError(t, err)
	_, cancel2, err := hub.Subscribe(2)
	require.NoError(t, err)
	defer cancel2()
	_, cancel3, err := hub.Subscribe(3)
	require.NoError(t, err)
	defer cancel3()

	_, _, err = hub.Subscribe(4)
	require.ErrorIs(t, err, ErrLimit)

	cancel1()
	_, cancel4, err := hub.Subscribe(4)
	require.NoError(t, err)
	defer cancel4()
}

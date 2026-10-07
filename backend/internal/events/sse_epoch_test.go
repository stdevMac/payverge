package events

import (
	"context"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func serveSSEWithResume(t *testing.T, businessID uint, lastEventID string) string {
	t.Helper()
	req := httptest.NewRequest("GET", "/businesses/"+strconv.Itoa(int(businessID))+"/events", nil)
	if lastEventID != "" {
		req.Header.Set("Last-Event-ID", lastEventID)
	}
	ctx, cancel := context.WithCancel(req.Context())
	cancel() // return after replay + connected
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: strconv.Itoa(int(businessID))}}
	c.Request = req
	c.Set("token_type", "staff")
	c.Set("staff_business_id", businessID)

	SSEHandler(c)
	return w.Body.String()
}

func TestParseResumeTokenSplitsEpochAndSeq(t *testing.T) {
	epoch, seq := parseResumeToken("abc123:42")
	assert.Equal(t, "abc123", epoch)
	assert.Equal(t, uint64(42), seq)

	// Legacy bare-seq format (old client / pre-epoch backend).
	epoch, seq = parseResumeToken("42")
	assert.Equal(t, "", epoch)
	assert.Equal(t, uint64(42), seq)

	epoch, seq = parseResumeToken("")
	assert.Equal(t, "", epoch)
	assert.Equal(t, uint64(0), seq)
}

// resumeFromNow returns a resume token at the current live head for the given
// epoch, so any events published AFTER this call replay on resume. It bumps the
// global hub sequence via a throwaway publish on a sentinel business (the seq is
// never 0 — that value is the fresh-connection sentinel and cannot be resumed
// from) and reads its assigned id. The singleton hub accumulates events across
// tests, so anchoring on "now" is the only stable way to isolate a test's own
// events from prior-test noise on the same business id.
func resumeFromNow(t *testing.T, epoch string) string {
	t.Helper()
	const sentinelBiz = uint(990001)
	GetHub().Publish(BusinessEvent{BusinessID: sentinelBiz, Type: "order.updated", Timestamp: time.Now()})
	buf := GetHub().ReplayAfter(sentinelBiz, 0)
	require.NotEmpty(t, buf)
	return epoch + ":" + strconv.FormatUint(buf[len(buf)-1].ID, 10)
}

func TestHubIDsCarryEpochPrefix(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)

	resume := resumeFromNow(t, GetHub().Epoch())
	GetHub().Publish(BusinessEvent{BusinessID: business.ID, Type: "order.updated", Timestamp: time.Now()})

	body := serveSSEWithResume(t, business.ID, resume)
	require.Contains(t, body, "id: "+GetHub().Epoch()+":")
	assert.NotContains(t, body, "event: sync.reset")
	assert.Contains(t, body, "event: order.updated")
}

func TestSSEHandlerStaleEpochSendsSyncReset(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)

	GetHub().Publish(BusinessEvent{BusinessID: business.ID, Type: "order.updated", Timestamp: time.Now()})

	// A client resuming with an epoch from a previous process must be told to
	// resync — the ring it references no longer exists.
	body := serveSSEWithResume(t, business.ID, "deadbeefstaleepoch:3")
	assert.Contains(t, body, "event: sync.reset")
	assert.Contains(t, body, "event: connected")
}

// TestSSEHandlerScopedFreshConnectDoesNotReplayRing pins X-9 for the
// permission-scoped branch: a fresh connection (no resume token) starts at the
// live head. Without a lastEventID guard, SubscribeWithReplayTopic(.., 0, ..)
// replays the entire retained ring to every new scoped tab.
func TestSSEHandlerScopedFreshConnectDoesNotReplayRing(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)

	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions(hostPerms), false
	})

	GetHub().Publish(BusinessEvent{BusinessID: business.ID, Type: "order.updated", Timestamp: time.Now()})

	body := serveSSEWithResume(t, business.ID, "")
	assert.Contains(t, body, "event: connected")
	assert.NotContains(t, body, "event: order.updated",
		"fresh scoped connection must start at the live head, not replay the ring")
}

// TestSSEHandlerScopedResumeStillReplaysMissedEvents pins that the guard above
// only suppresses the FRESH-connect replay — a scoped client resuming with a
// real token still gets bounded replay of genuinely-missed allowed events.
func TestSSEHandlerScopedResumeStillReplaysMissedEvents(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)

	restorePermissionResolver(t)
	SetPermissionResolver(func(_ *gin.Context) (PermissionChecker, bool) {
		return checkerWithPermissions(hostPerms), false
	})

	resume := resumeFromNow(t, GetHub().Epoch())
	GetHub().Publish(BusinessEvent{BusinessID: business.ID, Type: "order.updated", Timestamp: time.Now()})

	body := serveSSEWithResume(t, business.ID, resume)
	assert.NotContains(t, body, "event: sync.reset")
	assert.Contains(t, body, "event: order.updated")
}

func TestSSEHandlerSameEpochWithinRingReplaysNoReset(t *testing.T) {
	setupSSEHandlerTestDB(t)
	business := createSSETestBusiness(t)

	resume := resumeFromNow(t, GetHub().Epoch())
	GetHub().Publish(BusinessEvent{BusinessID: business.ID, Type: "order.updated", Timestamp: time.Now()})
	GetHub().Publish(BusinessEvent{BusinessID: business.ID, Type: "bill.updated", Timestamp: time.Now()})

	// Resume with the current epoch at the live head before both events: normal
	// replay, no reset.
	body := serveSSEWithResume(t, business.ID, resume)
	assert.NotContains(t, body, "event: sync.reset")
	assert.Contains(t, body, "event: order.updated")
	assert.Contains(t, body, "event: bill.updated")
}

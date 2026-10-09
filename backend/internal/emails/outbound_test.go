package emails

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type receiptProviderStub struct {
	id  string
	got EmailMessage
}

func (p *receiptProviderStub) Send(ctx context.Context, msg EmailMessage) error {
	_, err := p.SendWithReceipt(ctx, msg)
	return err
}

func (p *receiptProviderStub) SendWithReceipt(_ context.Context, msg EmailMessage) (SendResult, error) {
	p.got = msg
	return SendResult{ProviderMessageID: p.id}, nil
}

func (p *receiptProviderStub) ProviderName() string { return "resend" }

func TestEmailServerRecordsOutboundSendWithProviderMessageID(t *testing.T) {
	db := suppressionTestDB(t)
	store := NewGormOutboundStore(db)
	provider := &receiptProviderStub{id: "re_msg_123"}
	server, err := NewEmailServer(provider, "noreply@payverge.io", "updates@payverge.io", resolveTemplatesRoot(t))
	require.NoError(t, err)
	server.SetOutboundStore(store)

	require.NoError(t, server.SendPasswordResetEmail([]string{"guest@example.test"}, "https://payverge.io/reset", "en"))

	var rec EmailOutboundSend
	require.NoError(t, db.Where("provider_message_id = ?", "re_msg_123").First(&rec).Error)
	require.Equal(t, "resend", rec.Provider)
	require.Equal(t, OutboundStatusSent, rec.Status)
	require.Equal(t, "password_reset", rec.TemplateName)
	require.Equal(t, "g***@example.test", rec.RecipientRedacted)
	require.NotContains(t, rec.RecipientRedacted, "guest@example.test")
	require.Equal(t, "password_reset", provider.got.TemplateName)
}

func TestResendWebhookAppliesBounceToOutboundSend(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := suppressionTestDB(t)
	require.NoError(t, NewGormOutboundStore(db).RecordSend(context.Background(), EmailOutboundSend{
		Provider:          "resend",
		ProviderMessageID: "email-bounce-join",
		RecipientRedacted: "g***@example.test",
		TemplateName:      "reservation_confirmation",
		Status:            OutboundStatusSent,
		SentAt:            time.Now().UTC(),
	}))

	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("webhook-secret"))
	handler := NewResendWebhookHandler(db, secret)
	payload, err := json.Marshal(map[string]interface{}{
		"type":       "email.bounced",
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]interface{}{
			"email_id": "email-bounce-join",
			"to":       []string{"guest@example.test"},
			"bounce":   map[string]interface{}{"type": "Permanent"},
		},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = signedResendWebhookRequest(t, secret, "evt-bounce-join", payload)
	handler.Handle(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())

	var rec EmailOutboundSend
	require.NoError(t, db.Where("provider_message_id = ?", "email-bounce-join").First(&rec).Error)
	require.Equal(t, OutboundStatusBounced, rec.Status)
	require.Equal(t, "email.bounced", rec.LastEventType)
	require.NotNil(t, rec.LastEventAt)
}

func TestResendWebhookMissingOutboundSendIsNotFatal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db := suppressionTestDB(t)
	secret := "whsec_" + base64.StdEncoding.EncodeToString([]byte("webhook-secret"))
	handler := NewResendWebhookHandler(db, secret)
	payload, err := json.Marshal(map[string]interface{}{
		"type":       "email.delivered",
		"created_at": time.Now().UTC().Format(time.RFC3339Nano),
		"data": map[string]interface{}{
			"email_id": "email-unknown",
			"to":       []string{"ok@example.test"},
		},
	})
	require.NoError(t, err)

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = signedResendWebhookRequest(t, secret, "evt-delivered-unknown", payload)
	handler.Handle(ctx)
	require.Equal(t, http.StatusOK, recorder.Code, recorder.Body.String())
}

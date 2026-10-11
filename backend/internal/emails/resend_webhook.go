package emails

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/resend/resend-go/v3"
	"github.com/sirupsen/logrus"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/stdevmac/payverge/backend/internal/metrics"
)

const maxResendWebhookBodyBytes = 1 << 20

type resendWebhookVerifier interface {
	Verify(options *resend.VerifyWebhookOptions) error
}

type ResendWebhookHandler struct {
	db       *gorm.DB
	secret   string
	verifier resendWebhookVerifier
}

func NewResendWebhookHandler(db *gorm.DB, secret string) *ResendWebhookHandler {
	client := resend.NewClient("")
	return &ResendWebhookHandler{
		db:       db,
		secret:   strings.TrimSpace(secret),
		verifier: client.Webhooks,
	}
}

type resendWebhookPayload struct {
	Type      string    `json:"type"`
	CreatedAt time.Time `json:"created_at"`
	Data      struct {
		EmailID string   `json:"email_id"`
		To      []string `json:"to"`
		Bounce  struct {
			Type    string `json:"type"`
			SubType string `json:"subType"`
		} `json:"bounce"`
		Suppressed struct {
			Type string `json:"type"`
		} `json:"suppressed"`
	} `json:"data"`
}

func knownDeliveryEvent(eventType string) bool {
	switch eventType {
	case resend.EventEmailDelivered,
		resend.EventEmailDeliveryDelayed,
		resend.EventEmailBounced,
		resend.EventEmailComplained,
		resend.EventEmailFailed,
		resend.EventEmailSuppressed:
		return true
	default:
		return false
	}
}

func suppressionReason(payload resendWebhookPayload) (SuppressionReason, bool) {
	switch payload.Type {
	case resend.EventEmailComplained:
		return SuppressionReasonComplaint, true
	case resend.EventEmailSuppressed:
		return SuppressionReasonProvider, true
	case resend.EventEmailBounced:
		// Resend documents email.bounced as a permanent rejection. Preserve a
		// defensive exception for an explicitly temporary payload.
		return SuppressionReasonBounce, !strings.EqualFold(payload.Data.Bounce.Type, "temporary")
	default:
		return "", false
	}
}

// deliveryEventDetail is the human-readable provider reason recorded on the
// attributed entity (e.g. "Permanent:General" for a hard bounce).
func deliveryEventDetail(payload resendWebhookPayload) string {
	if detail := suppressionDetail(payload); detail != "" {
		return detail
	}
	return payload.Type
}

func suppressionDetail(payload resendWebhookPayload) string {
	switch payload.Type {
	case resend.EventEmailBounced:
		return strings.Trim(strings.Join([]string{payload.Data.Bounce.Type, payload.Data.Bounce.SubType}, ":"), ":")
	case resend.EventEmailSuppressed:
		return payload.Data.Suppressed.Type
	default:
		return ""
	}
}

func (h *ResendWebhookHandler) Handle(c *gin.Context) {
	if h == nil || h.db == nil || h.verifier == nil || h.secret == "" {
		c.AbortWithStatus(http.StatusServiceUnavailable)
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxResendWebhookBodyBytes)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || len(body) == 0 {
		c.AbortWithStatus(http.StatusBadRequest)
		return
	}
	webhookID := strings.TrimSpace(c.GetHeader("svix-id"))
	verifyErr := h.verifier.Verify(&resend.VerifyWebhookOptions{
		Payload: string(body),
		Headers: resend.WebhookHeaders{
			Id:        webhookID,
			Timestamp: strings.TrimSpace(c.GetHeader("svix-timestamp")),
			Signature: strings.TrimSpace(c.GetHeader("svix-signature")),
		},
		WebhookSecret: h.secret,
	})
	if verifyErr != nil {
		c.AbortWithStatus(http.StatusUnauthorized)
		return
	}

	var payload resendWebhookPayload
	if err := json.Unmarshal(body, &payload); err != nil || !knownDeliveryEvent(payload.Type) {
		c.AbortWithStatus(http.StatusNoContent)
		return
	}

	recipient := ""
	if len(payload.Data.To) > 0 {
		recipient, err = canonicalEmail(payload.Data.To[0])
		if err != nil {
			c.AbortWithStatus(http.StatusUnprocessableEntity)
			return
		}
	}
	if _, shouldSuppress := suppressionReason(payload); shouldSuppress && recipient == "" {
		c.AbortWithStatus(http.StatusUnprocessableEntity)
		return
	}

	now := time.Now().UTC()
	var occurredAt *time.Time
	if !payload.CreatedAt.IsZero() {
		occurred := payload.CreatedAt.UTC()
		occurredAt = &occurred
	}
	deliveryEvent := EmailDeliveryEvent{
		Provider:        "resend",
		WebhookID:       webhookID,
		EventType:       payload.Type,
		ProviderEmailID: strings.TrimSpace(payload.Data.EmailID),
		RecipientEmail:  recipient,
		OccurredAt:      occurredAt,
		ReceivedAt:      now,
	}
	duplicate := false
	err = h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		result := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "provider"}, {Name: "webhook_id"}},
			DoNothing: true,
		}).Create(&deliveryEvent)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			duplicate = true
			return nil
		}
		if err := applyOutboundDeliveryEvent(tx, "resend", strings.TrimSpace(payload.Data.EmailID), payload.Type, now); err != nil {
			return err
		}
		// #562: attach the verdict to whatever triggered the send. An email ID
		// we never queued (pre-outbox mail, another sender on the domain) is a
		// no-op, so the event above is still recorded either way.
		eventAt := now
		if occurredAt != nil {
			eventAt = *occurredAt
		}
		if err := applyOutboxDeliveryEvent(tx, "resend", strings.TrimSpace(payload.Data.EmailID),
			payload.Type, recipient, deliveryEventDetail(payload), eventAt); err != nil {
			return err
		}
		reason, shouldSuppress := suppressionReason(payload)
		if !shouldSuppress {
			return nil
		}
		return NewGormSuppressionStore(tx).Suppress(c.Request.Context(), EmailSuppression{
			Email:           recipient,
			Reason:          reason,
			Provider:        "resend",
			ProviderEventID: webhookID,
			ProviderEmailID: strings.TrimSpace(payload.Data.EmailID),
			Detail:          suppressionDetail(payload),
			SuppressedAt:    now,
			LastEventAt:     now,
		})
	})
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"provider":   "resend",
			"webhook_id": webhookID,
			"event_type": payload.Type,
		}).WithError(err).Error("email delivery webhook persistence failed")
		c.AbortWithStatus(http.StatusInternalServerError)
		return
	}
	if !duplicate {
		metrics.EmailDeliveryEvents.WithLabelValues(payload.Type).Inc()
		if _, suppressed := suppressionReason(payload); suppressed {
			logrus.WithFields(logrus.Fields{
				"provider":          "resend",
				"webhook_id":        webhookID,
				"provider_email_id": payload.Data.EmailID,
				"event_type":        payload.Type,
			}).Warn("email recipient suppressed after delivery event")
		}
	}
	c.Status(http.StatusOK)
}

package services

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"gorm.io/gorm"
)

// WebPushService sends Web Push API notifications to subscribed devices.
type WebPushService struct {
	db        *gorm.DB
	vapidPub  string
	vapidPriv string
	vapidSubj string
	// httpClient sends deliveries without following redirects.
	httpClient *http.Client
	// validateEndpoint gates every stored endpoint before a send; tests swap
	// it to reach an httptest server.
	validateEndpoint func(string) error
}

// NewWebPushService creates a WebPushService. If vapidPub or vapidPriv are
// empty, SendPush is a no-op (web push not configured).
func NewWebPushService(db *gorm.DB, vapidPub, vapidPriv, vapidSubj string) *WebPushService {
	return &WebPushService{
		db: db, vapidPub: vapidPub, vapidPriv: vapidPriv, vapidSubj: vapidSubj,
		httpClient:       newPushHTTPClient(),
		validateEndpoint: ValidatePushEndpoint,
	}
}

// SendPush delivers a push notification to all subscribed endpoints for the
// given user. Endpoints that respond with 410 Gone are automatically pruned.
func (s *WebPushService) SendPush(userID uint, title, body, url string) error {
	if s.vapidPub == "" || s.vapidPriv == "" {
		return nil
	}

	var subs []database.PushSubscription
	if err := s.db.Where("user_id = ?", userID).Find(&subs).Error; err != nil {
		return err
	}

	return s.deliverToSubscriptions(subs, title, body, url)
}

// SendLocalizedPushToBusiness delivers a push notification to all staff
// subscribed to a business, with the title/body localized to the business
// owner's language. This is the localized seam for operator browser push:
// callers pass a notification key + interpolation args instead of a hard-coded
// English title/body, and the owner's language (resolved via
// DetermineBusinessOwnerLanguage, the same source the Telegram and email
// channels use) selects the localized copy from pushMessagesByLocale.
//
// Owner-language resolution happens here, not at the call site, because it needs
// the full *database.Business (owner relations); call sites only have the
// businessID. If the business can't be loaded we fall back to English copy so a
// lookup miss never silences the operator.
func (s *WebPushService) SendLocalizedPushToBusiness(businessID uint, key PushKey, args PushArgs, url string) error {
	if s.vapidPub == "" || s.vapidPriv == "" {
		return nil
	}

	language := emails.LanguageEnglish
	if business, err := database.GetBusinessByID(businessID); err == nil {
		language = DetermineBusinessOwnerLanguage(business)
	}

	title, body := LocalizePush(language, key, args)

	subs, err := s.loadActiveBusinessPushSubscriptions(businessID)
	if err != nil {
		return err
	}

	return s.deliverToSubscriptions(subs, title, body, url)
}

// loadActiveBusinessPushSubscriptions returns push rows for a business whose
// principal is still an active member: active staff rows for principal_type=staff,
// or the business owner for principal_type=owner_user. Single query — no N+1.
func (s *WebPushService) loadActiveBusinessPushSubscriptions(businessID uint) ([]database.PushSubscription, error) {
	var subs []database.PushSubscription
	// SQLite-compatible EXISTS form (also works on Postgres).
	err := s.db.Where("business_id = ?", businessID).
		Where(`(
			(principal_type = ? AND EXISTS (
				SELECT 1 FROM staff
				WHERE staff.id = push_subscriptions.principal_id
				  AND staff.business_id = push_subscriptions.business_id
				  AND staff.is_active = ?
			))
			OR
			(principal_type = ? AND EXISTS (
				SELECT 1 FROM businesses
				WHERE businesses.id = push_subscriptions.business_id
				  AND businesses.user_id = push_subscriptions.principal_id
			))
		)`, database.PushPrincipalStaff, true, database.PushPrincipalOwnerUser).
		Find(&subs).Error
	return subs, err
}

func (s *WebPushService) deliverToSubscriptions(subs []database.PushSubscription, title, body, url string) error {

	payload, _ := json.Marshal(map[string]string{
		"title": title,
		"body":  body,
		"url":   url,
	})

	for _, sub := range subs {
		// Rows stored before the endpoint allowlist existed are skipped, not
		// sent: the sender must never POST to an arbitrary URL (M-push).
		if s.validateEndpoint != nil {
			if err := s.validateEndpoint(sub.Endpoint); err != nil {
				log.Printf("[WebPush] skipped subscription %d: endpoint host %s is not an allowed push service", sub.ID, endpointHost(sub.Endpoint))
				continue
			}
		}
		ws := &webpush.Subscription{
			Endpoint: sub.Endpoint,
			Keys: webpush.Keys{
				P256dh: sub.P256dhKey,
				Auth:   sub.AuthKey,
			},
		}

		client := s.httpClient
		if client == nil {
			client = newPushHTTPClient()
		}
		resp, err := webpush.SendNotification(payload, ws, &webpush.Options{
			HTTPClient:      client,
			Subscriber:      s.vapidSubj,
			VAPIDPublicKey:  s.vapidPub,
			VAPIDPrivateKey: s.vapidPriv,
			TTL:             60,
		})
		if err != nil {
			log.Printf("[WebPush] send error for endpoint %s: %v", endpointHost(sub.Endpoint), err)
			continue
		}
		resp.Body.Close()

		switch {
		case resp.StatusCode == http.StatusGone || resp.StatusCode == http.StatusNotFound:
			// 410 Gone and 404 Not Found both mean the subscription no longer
			// exists at the push service — prune it so we stop paying for the
			// send on every fan-out.
			s.db.Delete(&sub)
			log.Printf("[WebPush] removed expired subscription %d (status %d)", sub.ID, resp.StatusCode)
		case resp.StatusCode < 200 || resp.StatusCode >= 300:
			// Any other non-2xx was previously invisible (only err!=nil logged).
			// One warn line per failure, endpoint host only (endpoints embed
			// per-device tokens; don't log them).
			log.Printf("[WebPush] push rejected: subscription=%d host=%s status=%d", sub.ID, endpointHost(sub.Endpoint), resp.StatusCode)
		}
	}
	return nil
}

// endpointHost extracts just the host from a push endpoint URL for logging;
// the path carries a per-device secret token and must stay out of logs.
func endpointHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" {
		return "invalid"
	}
	return u.Host
}

// Package stripetest provides deterministic Stripe webhook fixtures for local
// and CI tests. It never calls Stripe and never logs the signing secret.
package stripetest

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

// Simulator creates reproducible payloads signed like Stripe test webhooks.
type Simulator struct {
	secret string
	now    time.Time
}

// New creates a simulator with a fixed signing clock.
func New(secret string, now time.Time) *Simulator {
	return &Simulator{secret: secret, now: now.UTC()}
}

// Event builds a signed Stripe event. Map keys are encoded deterministically
// by encoding/json, making the payload suitable for checked CI fixtures.
func (s *Simulator) Event(id, eventType string, object map[string]interface{}) ([]byte, string, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"id":   id,
		"type": eventType,
		"data": map[string]interface{}{"object": object},
	})
	if err != nil {
		return nil, "", err
	}
	timestamp := s.now.Unix()
	signed := fmt.Sprintf("%d.%s", timestamp, payload)
	mac := hmac.New(sha256.New, []byte(s.secret))
	_, _ = mac.Write([]byte(signed))
	signature := hex.EncodeToString(mac.Sum(nil))
	return payload, fmt.Sprintf("t=%d,v1=%s", timestamp, signature), nil
}

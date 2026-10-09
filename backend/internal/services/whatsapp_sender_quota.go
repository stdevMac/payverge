package services

// Build-tag-neutral (see whatsapp_helpers.go): must not import go.mau.fi/*.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"strconv"
	"strings"

	"github.com/stdevmac/payverge/backend/internal/dailyquota"
)

// WhatsApp per-sender daily ceiling (SEC H-ai-cost). The per-sender sliding
// window (10 per minute) still lets one number send ~14k messages a day,
// enough to spend a venue's whole guest message and USD budget by itself and
// lock every other guest out. A WhatsApp sender is the channel's equivalent of
// the HTTP waiter's signed device cookie, so it shares that ceiling's env
// knob and default. The key is a hash of the sender without its device part
// (all linked devices of one number count together), and it spans businesses
// like the device ceiling does. In-memory and per replica, like the other
// dailyquota ceilings; the durable USD scopes stay the hard backstop.
const (
	envWhatsAppDailyPerSender     = "AI_WAITER_DAILY_MESSAGES_PER_DEVICE"
	defaultWhatsAppDailyPerSender = 150
)

var whatsAppSenderQuota = dailyquota.New(100_000)

func whatsAppDailyPerSender() int64 {
	if v := strings.TrimSpace(os.Getenv(envWhatsAppDailyPerSender)); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	return defaultWhatsAppDailyPerSender
}

// takeWhatsAppSenderQuota counts one inbound turn against the sender's daily
// ceiling; false means the ceiling is exhausted. sender is the JID without
// its device part. An empty sender is refused: it cannot be attributed.
func takeWhatsAppSenderQuota(sender string) bool {
	key, ok := whatsAppSenderKey(sender)
	if !ok {
		return false
	}
	took, _ := whatsAppSenderQuota.Take(dailyquota.Limit{
		Key: "wa-sender:" + key,
		Max: whatsAppDailyPerSender(),
	})
	return took
}

// takeWhatsAppSenderQuotaNotice reports whether an over-ceiling sender should
// still get the "budget reached" reply: true once per sender per UTC day,
// false afterwards, so a flooding number gets one outbound message and then
// its turns are dropped silently instead of each one costing an outbound send.
// An empty sender never gets a reply.
func takeWhatsAppSenderQuotaNotice(sender string) bool {
	key, ok := whatsAppSenderKey(sender)
	if !ok {
		return false
	}
	took, _ := whatsAppSenderQuota.Take(dailyquota.Limit{Key: "wa-sender-notified:" + key, Max: 1})
	return took
}

// whatsAppSenderKey hashes the trimmed sender so raw phone numbers never sit
// in the counter's keys.
func whatsAppSenderKey(sender string) (string, bool) {
	sender = strings.TrimSpace(sender)
	if sender == "" {
		return "", false
	}
	sum := sha256.Sum256([]byte(sender))
	return hex.EncodeToString(sum[:16]), true
}

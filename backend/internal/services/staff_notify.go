package services

import (
	"log"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/events"
	"github.com/stdevmac/payverge/backend/internal/logger"
)

// NotifyStaff is the shared staff-awareness fan-out: durable inbox row + live SSE
// + best-effort web push, each recipient's copy localized in their own language.
// It lives in the services layer (rather than only in handlers) so BOTH the HTTP
// handlers AND background schedulers (e.g. the shift-reminder scheduler) route
// through ONE chokepoint — keeping localization, dedupe, and channel fan-out
// identical no matter what triggers the notification.
//
// FULLY best-effort by contract: it runs AFTER the triggering mutation has
// already succeeded, so any failure (resolve, inbox insert, SSE, push) is logged
// and never propagated. Durability comes from the inbox INSERT being synchronous,
// not from being allowed to fail the caller. `wp` may be nil (push simply skipped
// — inbox + SSE still land). NO money ever crosses this wire.
func NotifyStaff(db *database.DB, wp *WebPushService, businessID uint, targets []uint, kind string, key PushKey, args PushArgs, url string) {
	ids := dedupeNonZeroIDs(targets)
	if len(ids) == 0 {
		return
	}
	resolved, err := db.ResolveStaffNotifyTargets(businessID, ids)
	if err != nil {
		log.Printf("[NotifyStaff] resolve targets failed: business_id=%d kind=%s err=%v", businessID, kind, err)
		return
	}
	if len(resolved) == 0 {
		return
	}

	// Business default language fallback (resolved once). LocalizePush treats ""
	// as English, so a lookup miss never blanks the copy.
	fallbackLang := ""
	if biz, err := database.GetBusinessByID(businessID); err == nil {
		fallbackLang = DetermineBusinessOwnerLanguage(biz)
	}

	rows := make([]database.StaffNotification, 0, len(resolved))
	type pushTarget struct {
		userID      uint
		title, body string
	}
	pushes := make([]pushTarget, 0, len(resolved))
	staffIDs := make([]uint, 0, len(resolved))
	for _, t := range resolved {
		lang := t.Language
		if lang == "" {
			lang = fallbackLang
		}
		title, body := LocalizePush(lang, key, args)
		rows = append(rows, database.StaffNotification{
			BusinessID: businessID, StaffID: t.StaffID,
			Kind: kind, Title: title, Body: body, URL: url,
		})
		staffIDs = append(staffIDs, t.StaffID)
		if t.UserID != 0 {
			pushes = append(pushes, pushTarget{userID: t.UserID, title: title, body: body})
		}
	}

	if err := db.CreateStaffNotifications(rows); err != nil {
		// Log, do NOT fail: the triggering mutation already succeeded. Fall through
		// to SSE/push so a live staffer is not silenced by an inbox write blip.
		log.Printf("[NotifyStaff] inbox insert failed: business_id=%d kind=%s err=%v", businessID, kind, err)
	}

	events.GetHub().PublishJSON(businessID, "notification.new", map[string]interface{}{"staff_ids": staffIDs})

	if wp != nil && len(pushes) > 0 {
		logger.SafeGo(func() {
			for _, p := range pushes {
				if err := wp.SendPush(p.userID, p.title, p.body, url); err != nil {
					log.Printf("[NotifyStaff] push failed: user_id=%d kind=%s err=%v", p.userID, kind, err)
				}
			}
		})
	}
}

// dedupeNonZeroIDs returns the input ids with duplicates and zero removed, order
// preserved. (0 = an owner/non-staff caller with no staff row.)
func dedupeNonZeroIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	out := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

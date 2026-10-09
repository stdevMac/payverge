package auth

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/gin-gonic/gin"
)

type StaffOAuthState struct {
	ExpiresAt time.Time
	Redirect  string
}

type StaffOAuthMembershipChoice struct {
	BusinessID   uint               `json:"business_id"`
	BusinessName string             `json:"business_name"`
	Role         database.StaffRole `json:"role"`
}

type StaffOAuthMembershipSelectionPayload struct {
	SelectionToken string                       `json:"selection_token"`
	Memberships    []StaffOAuthMembershipChoice `json:"memberships"`
}

// staffOAuthMembershipSelectionRedirect carries the already-authenticated
// identity into the existing membership-selection exchange without placing
// the short-lived signed proof in a query string. URL fragments are not sent to
// servers or in HTTP referrers, and the frontend removes the fragment as soon
// as it has parsed the handoff.
func staffOAuthMembershipSelectionRedirect(base string, payload StaffOAuthMembershipSelectionPayload) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	base = strings.SplitN(base, "#", 2)[0]
	return base + "#staff_membership_selection=" + url.QueryEscape(string(raw)), nil
}

// Sentinels returned by resolveStaffForGoogleLogin, mapped to staff_error
// redirect codes by GoogleStaffCallback.
var (
	// ErrStaffOAuthUnverifiedEmail is returned when Google has not verified the
	// email on the identity attempting staff login. Staff accounts are matched
	// purely by email, so trusting an unverified address is an account-takeover
	// vector; the login is rejected before the email match can authenticate.
	ErrStaffOAuthUnverifiedEmail = errors.New("google email is not verified")
	// ErrStaffNotFound is returned when no staff row matches the (verified) email.
	ErrStaffNotFound = errors.New("staff not found for email")
	// ErrStaffInactive is returned when the matched staff row is deactivated.
	ErrStaffInactive = errors.New("staff is inactive")
	// ErrStaffMembershipSelectionNeeded requires the authenticated identity to
	// use the email-code selection exchange before any business-scoped token is
	// minted. Google OAuth never guesses between memberships.
	ErrStaffMembershipSelectionNeeded = errors.New("staff membership selection is required")
)

// resolveStaffForGoogleLogin decides whether a Google-authenticated identity may
// log in as an existing staff member. It mirrors the primary user flow's
// account-takeover guard (resolveGoogleCallbackUserWithInvite / ErrUnverifiedOAuthEmail):
// because a staff account is matched only by email, the email MUST be one Google
// has verified — otherwise anyone who registers an unverified Google account
// bearing a staff member's address could authenticate as them. The verified-email
// gate is checked BEFORE the lookup so a matching row can never authenticate an
// unverified identity. lookup is injected (db.StaffService.GetByEmail in
// production) to keep this security decision unit-testable without the global DB.
func resolveStaffForGoogleLogin(userInfo *GoogleUserInfo, lookup func(email string) (*database.Staff, error)) (*database.Staff, error) {
	if userInfo == nil || !userInfo.VerifiedEmail {
		return nil, ErrStaffOAuthUnverifiedEmail
	}
	staff, err := lookup(strings.ToLower(userInfo.Email))
	if errors.Is(err, database.ErrStaffMembershipSelectionNeeded) {
		return nil, ErrStaffMembershipSelectionNeeded
	}
	if err != nil || staff == nil {
		return nil, ErrStaffNotFound
	}
	if !staff.IsActive {
		return nil, ErrStaffInactive
	}
	return staff, nil
}

func (h *AuthHandler) startStaffStateCleanup(interval time.Duration) {
	ticker := time.NewTicker(interval)
	// Raw goroutine (no logger.SafeTick) is intentional: the loop body only does
	// in-memory map cleanup under a mutex with no external data or I/O, so it has
	// no realistic panic surface. Data/IO-processing background loops (schedulers,
	// notification workers) use SafeTick to stay panic-isolated.
	go func() {
		for range ticker.C {
			now := time.Now()
			h.staffStateMu.Lock()
			for key, state := range h.staffStateStore {
				if now.After(state.ExpiresAt) {
					delete(h.staffStateStore, key)
				}
			}
			h.staffStateMu.Unlock()
		}
	}()
}

// consumeStaffState atomically fetches and removes a staff OAuth state token,
// reporting whether the returned value is valid (present and not expired). The
// check-and-delete happens under a single write lock so two concurrent callbacks
// carrying the same captured state token cannot both pass validation — closing
// the TOCTOU replay window that a read-lock/release/write-lock-delete sequence
// leaves open.
func (h *AuthHandler) consumeStaffState(state string) (StaffOAuthState, bool) {
	h.staffStateMu.Lock()
	defer h.staffStateMu.Unlock()
	stateData, ok := h.staffStateStore[state]
	if !ok {
		return StaffOAuthState{}, false
	}
	delete(h.staffStateStore, state)
	if time.Now().After(stateData.ExpiresAt) {
		return StaffOAuthState{}, false
	}
	return stateData, true
}

// isStaffState reports whether the given OAuth state was issued by the staff
// login flow, WITHOUT consuming it. Staff Google login reuses the regular
// /api/v1/auth/google/callback redirect_uri but tracks its state in a separate
// store, so GoogleCallback uses this to route staff states to GoogleStaffCallback
// instead of rejecting them as unknown user states (which 400'd every staff
// Google login). The actual single-use consume still happens atomically inside
// GoogleStaffCallback via consumeStaffState.
func (h *AuthHandler) isStaffState(state string) bool {
	if state == "" {
		return false
	}
	h.staffStateMu.RLock()
	defer h.staffStateMu.RUnlock()
	_, ok := h.staffStateStore[state]
	return ok
}

// emailDomainForLog returns just the domain portion of an email for log
// correlation without leaking the local part (PII). Returns "unknown" if the
// input can't be parsed.
func emailDomainForLog(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 || at == len(email)-1 {
		return "unknown"
	}
	return strings.ToLower(email[at+1:])
}

func sanitizeStaffRedirect(rawRedirect string) string {
	frontendBaseURL := getFrontendBaseURL()
	fallback := frontendBaseURL + "/staff/login"

	if rawRedirect == "" {
		return fallback
	}

	// Allow relative paths — they are safe because the frontend base is prepended.
	if strings.HasPrefix(rawRedirect, "/") {
		return frontendBaseURL + rawRedirect
	}

	// For absolute URLs, perform host-exact comparison to prevent open redirects
	// via evil subdomains (e.g. example.com.evil.com) that pass a naive prefix check.
	parsed, err := url.Parse(rawRedirect)
	if err != nil {
		return fallback
	}

	baseParsed, err := url.Parse(frontendBaseURL)
	if err != nil {
		return fallback
	}

	if parsed.Scheme == baseParsed.Scheme && parsed.Host == baseParsed.Host {
		return rawRedirect
	}

	return fallback
}

// GoogleStaffAuthURL returns the Google OAuth URL for staff login
func (h *AuthHandler) GoogleStaffAuthURL(c *gin.Context) {
	state, err := GenerateOAuthState()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate state"})
		return
	}

	redirect := sanitizeStaffRedirect(c.Query("redirect"))
	h.staffStateMu.Lock()
	h.staffStateStore[state] = StaffOAuthState{
		ExpiresAt: time.Now().Add(oauthStateTTL),
		Redirect:  redirect,
	}
	h.staffStateMu.Unlock()
	// Same browser-bound state cookie as the operator Google flow (SEC-1). Staff
	// login does not account-link, but unbound state still enables login CSRF.
	setOAuthStateCookie(c, state)

	url := h.oauthConfig.GetGoogleAuthURL(state)
	if url == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Google OAuth not configured"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url, "state": state})
}

// GoogleStaffCallback handles Google OAuth callback for staff login
func (h *AuthHandler) GoogleStaffCallback(c *gin.Context) {
	code := c.Query("code")
	state := c.Query("state")

	if !oauthStateCookieMatches(c, state) {
		clearOAuthStateCookie(c)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired state"})
		return
	}

	stateData, ok := h.consumeStaffState(state)
	clearOAuthStateCookie(c)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired state"})
		return
	}

	ctx := context.Background()
	token, err := h.exchangeGoogleCode(ctx, code)
	if err != nil {
		log.Printf("[StaffAuth] Google token exchange failed: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Failed to exchange authorization code"})
		return
	}

	userInfo, err := h.lookupGoogleUserInfo(ctx, token.AccessToken)
	if err != nil {
		log.Printf("[StaffAuth] Failed to get Google user info: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get user info"})
		return
	}

	db := database.GetDBWrapper()
	staff, err := resolveStaffForGoogleLogin(userInfo, db.StaffService.GetByEmail)
	if err != nil {
		switch {
		case errors.Is(err, ErrStaffOAuthUnverifiedEmail):
			// Google did not verify this email — refuse to match a staff account
			// by an address the caller has not proven they own.
			log.Printf("[StaffAuth] Rejected unverified Google email (email_domain=%s)", emailDomainForLog(userInfo.Email))
			c.Redirect(http.StatusTemporaryRedirect, stateData.Redirect+"?staff_error=unverified_email")
		case errors.Is(err, ErrStaffInactive):
			c.Redirect(http.StatusTemporaryRedirect, stateData.Redirect+"?staff_error=inactive")
		case errors.Is(err, ErrStaffMembershipSelectionNeeded):
			memberships, membershipErr := db.StaffService.GetActiveByEmail(strings.ToLower(userInfo.Email))
			selectionToken, tokenErr := server.GenerateStaffMembershipSelectionToken(userInfo.Email)
			if membershipErr != nil || len(memberships) < 2 || tokenErr != nil {
				log.Printf("[StaffAuth] Failed to prepare membership selection (email_domain=%s)", emailDomainForLog(userInfo.Email))
				c.Redirect(http.StatusTemporaryRedirect, stateData.Redirect+"?staff_error=membership_selection_required")
				return
			}
			choices := make([]StaffOAuthMembershipChoice, 0, len(memberships))
			for i := range memberships {
				businessName := ""
				if business, businessErr := db.BusinessService.GetByID(memberships[i].BusinessID); businessErr == nil && business != nil {
					businessName = business.Name
				}
				choices = append(choices, StaffOAuthMembershipChoice{
					BusinessID: memberships[i].BusinessID, BusinessName: businessName, Role: memberships[i].Role,
				})
			}
			selectionRedirect, redirectErr := staffOAuthMembershipSelectionRedirect(
				stateData.Redirect,
				StaffOAuthMembershipSelectionPayload{SelectionToken: selectionToken, Memberships: choices},
			)
			if redirectErr != nil {
				c.Redirect(http.StatusTemporaryRedirect, stateData.Redirect+"?staff_error=membership_selection_required")
				return
			}
			c.Redirect(http.StatusTemporaryRedirect, selectionRedirect)
		default:
			// ErrStaffNotFound (or any lookup error). Avoid logging the raw email —
			// it enables log-reader enumeration of staff accounts. Domain is enough
			// to debug misrouted SSO attempts.
			log.Printf("[StaffAuth] Staff not found (email_domain=%s)", emailDomainForLog(userInfo.Email))
			c.Redirect(http.StatusTemporaryRedirect, stateData.Redirect+"?staff_error=not_found")
		}
		return
	}

	var staffToken string
	var refreshToken string
	if session.GlobalStore != nil {
		staffID := staff.ID
		sess, sessErr := session.GlobalStore.Create(CreateSessionInput{
			UserID:    &staffID,
			Provider:  "google_staff",
			IPAddress: c.ClientIP(),
			UserAgent: c.GetHeader("User-Agent"),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		if sessErr != nil {
			log.Printf("[StaffAuth] Failed to create session: %v", sessErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
			return
		}
		staffToken, err = server.GenerateStaffToken(staff, sess.ID)
		if err != nil {
			log.Printf("[StaffAuth] Failed to generate staff token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate authentication token"})
			return
		}
		if err := session.GlobalStore.UpdateTokenHash(sess.ID, HashToken(staffToken)); err != nil {
			log.Printf("[StaffAuth] Failed to persist token hash for session %v: %v", sess.ID, err)
		}
		refreshToken, _ = session.GenerateRefreshToken()
		if err := session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshToken), time.Now().Add(7*24*time.Hour)); err != nil {
			log.Printf("[StaffAuth] Failed to rotate refresh token for session %v: %v", sess.ID, err)
		}
	} else {
		staffToken, err = server.GenerateStaffToken(staff)
		if err != nil {
			log.Printf("[StaffAuth] Failed to generate staff token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate authentication token"})
			return
		}
	}

	// Evict leftover owner/customer cookies so Hybrid/session-info resolve
	// the just-authenticated staff principal instead of a shadowed owner.
	utils.ClearAllAuthCookies(c)
	if refreshToken != "" {
		utils.SetSessionCookie(c, "refresh_token", refreshToken, 7*24*3600)
	}

	// Set staff session cookie
	utils.SetSessionCookie(c, "staff_token", staffToken, 15*60)

	redirectURL := stateData.Redirect
	if strings.Contains(redirectURL, "?") {
		redirectURL = redirectURL + "&staff_auth=success"
	} else {
		redirectURL = redirectURL + "?staff_auth=success"
	}

	c.Redirect(http.StatusTemporaryRedirect, redirectURL)
}

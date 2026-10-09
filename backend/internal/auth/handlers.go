package auth

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/stdevmac/payverge/backend/internal/auththrottle"
	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/database"
	"github.com/stdevmac/payverge/backend/internal/emails"
	"github.com/stdevmac/payverge/backend/internal/locales"
	"github.com/stdevmac/payverge/backend/internal/logger"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/middleware"
	"github.com/stdevmac/payverge/backend/internal/runtimecontrol"
	"github.com/stdevmac/payverge/backend/internal/server"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/utils"
	"github.com/stdevmac/payverge/backend/internal/verification"

	"github.com/gin-gonic/gin"
	"golang.org/x/oauth2"
	"gorm.io/gorm"
)

// Helper function for setting session cookies
func setSessionCookie(c *gin.Context, token string) {
	utils.SetSessionCookie(c, "session_token", token, 15*60) // 15 min — matches JWT TTL
}

func getFrontendBaseURL() string {
	return config.FrontendBaseURL()
}

// clientIPDebugEnabled gates the client-IP attribution log. With
// CLIENT_IP_DEBUG=1 (or true) the backend logs one [ClientIPDebug] line per
// sign-up attempt (POST /api/v1/auth/register only, by design: one route is
// enough to check the proxy chain, and it logs before the body is parsed, so
// an empty POST works). It is a setup aid: docs/self-hosting/reverse-proxies.md
// has the check, and the variable should be removed afterwards.
func clientIPDebugEnabled() bool {
	v := strings.TrimSpace(os.Getenv("CLIENT_IP_DEBUG"))
	return v == "1" || strings.EqualFold(v, "true")
}

func isAllowedRedirectPath(p string) bool {
	// Reject double-slash paths that could be interpreted as protocol-relative URLs.
	if strings.HasPrefix(p, "//") {
		return false
	}

	// URL-decode to catch percent-encoded traversal sequences like %2e%2e.
	decoded, err := url.PathUnescape(p)
	if err != nil {
		return false
	}

	// Normalize the path to resolve any . and .. segments.
	cleaned := path.Clean(decoded)

	// Reject if the cleaned path still contains a traversal segment.
	if strings.Contains(cleaned, "..") {
		return false
	}

	switch {
	case cleaned == "/dashboard":
		return true
	case strings.HasPrefix(cleaned, "/business/"):
		return true
	case strings.HasPrefix(cleaned, "/staff/"):
		return true
	default:
		return false
	}
}

func resolveSafeFrontendRedirect(c *gin.Context, fallbackPath string) string {
	baseURL := getFrontendBaseURL()
	if fallbackPath == "" {
		fallbackPath = "/dashboard"
	}
	if !strings.HasPrefix(fallbackPath, "/") {
		fallbackPath = "/" + fallbackPath
	}
	fallbackURL := baseURL + fallbackPath

	redirectParam := strings.TrimSpace(c.Query("redirect"))
	if redirectParam == "" {
		return fallbackURL
	}

	redirectURL, err := url.Parse(redirectParam)
	if err != nil {
		return fallbackURL
	}

	// Relative path redirects must be explicitly allowlisted.
	if !redirectURL.IsAbs() {
		if isAllowedRedirectPath(redirectURL.Path) {
			return baseURL + redirectURL.String()
		}
		return fallbackURL
	}

	// Absolute redirects must match configured frontend origin and allowlisted paths.
	baseParsed, err := url.Parse(baseURL)
	if err != nil {
		return fallbackURL
	}
	if redirectURL.Scheme == baseParsed.Scheme &&
		redirectURL.Host == baseParsed.Host &&
		isAllowedRedirectPath(redirectURL.Path) {
		return redirectURL.String()
	}

	return fallbackURL
}

// oauthStateCookieName binds the OAuth CSRF state to the browser that started
// the flow (SEC-1). SameSite=Lax so the cookie is present on the top-level
// Google → callback redirect, but not on cross-site subresource requests.
const oauthStateCookieName = "oauth_state"

// oauthStateTTL is the single-use state lifetime for Google OAuth start/callback.
const oauthStateTTL = 10 * time.Minute

// oauthStateStore is a thread-safe store for CSRF state tokens with automatic
// expiry cleanup. Multi-instance note: this map is process-local. Cookie binding
// closes the account-linking CSRF (SEC-1) even across instances, but the state
// itself must still be consumed on the same instance that minted it (or a shared
// store). For multi-instance deployments, replace the map with Redis/DB while
// keeping the HttpOnly cookie binding and linkUserID intent fields.
type oauthStateStore struct {
	mu     sync.Mutex
	states map[string]oauthStateEntry
	ticker *time.Ticker
	done   chan struct{}
}

type oauthStateEntry struct {
	expiresAt  time.Time
	inviteCode string
	// linkUserID is non-zero only when an authenticated operator started OAuth
	// with explicit link intent. Callback must never infer linking from an
	// ambient session_token alone (SEC-1).
	linkUserID uint
}

func newOAuthStateStore() *oauthStateStore {
	s := &oauthStateStore{
		states: make(map[string]oauthStateEntry),
		ticker: time.NewTicker(5 * time.Minute),
		done:   make(chan struct{}),
	}
	go s.cleanupLoop()
	return s
}

func (s *oauthStateStore) Set(state string, expiry time.Time, inviteCode string, linkUserID uint) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.states[state] = oauthStateEntry{
		expiresAt:  expiry,
		inviteCode: strings.TrimSpace(inviteCode),
		linkUserID: linkUserID,
	}
}

func (s *oauthStateStore) Consume(state string) (oauthStateEntry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry, ok := s.states[state]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(s.states, state)
		return oauthStateEntry{}, false
	}
	delete(s.states, state) // single-use
	return entry, true
}

func setOAuthStateCookie(c *gin.Context, state string) {
	utils.SetSessionCookie(c, oauthStateCookieName, state, int(oauthStateTTL.Seconds()))
}

func clearOAuthStateCookie(c *gin.Context) {
	utils.ClearSessionCookie(c, oauthStateCookieName)
}

// oauthStateCookieMatches reports whether the browser-bound oauth_state cookie
// equals the query state. Length mismatch fails closed without panicking
// subtle.ConstantTimeCompare.
func oauthStateCookieMatches(c *gin.Context, state string) bool {
	if state == "" {
		return false
	}
	cookie, err := c.Cookie(oauthStateCookieName)
	if err != nil || cookie == "" || len(cookie) != len(state) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(cookie), []byte(state)) == 1
}

// sessionUserIDFromCookie returns the operator user_id from session_token only
// when the cookie is a live user session bound to that user. Zero means fail
// closed: missing cookie, bad JWT, non-user token, missing/invalid session id,
// a nil session store, a revoked or mismatched token hash, or a persisted
// session whose user does not match the claim.
func sessionUserIDFromCookie(c *gin.Context) uint {
	cookie, err := c.Cookie("session_token")
	if err != nil || cookie == "" {
		return 0
	}
	claims, err := server.VerifyToken(cookie)
	if err != nil {
		return 0
	}
	if claims["type"] != "user" {
		return 0
	}
	sid, ok := claims["session_id"].(float64)
	if !ok || sid <= 0 {
		return 0
	}
	userIDDouble, ok := claims["user_id"].(float64)
	if !ok || userIDDouble <= 0 {
		return 0
	}
	if session.GlobalStore == nil {
		return 0
	}
	valid, err := session.GlobalStore.Validate(uint(sid), session.HashToken(cookie))
	if err != nil || !valid {
		return 0
	}
	persisted, err := session.GlobalStore.Get(uint(sid))
	if err != nil || persisted == nil || persisted.UserID == nil || *persisted.UserID != uint(userIDDouble) {
		return 0
	}
	return uint(userIDDouble)
}

// googleCallbackLinkIntent decides whether a Google callback may attach a new
// Google identity to an existing Payverge user (SEC-1).
//
// Linking requires ALL of:
//  1. link-mode encoded into the consumed OAuth state at start time (authenticated
//     start with explicit link=1 intent), and
//  2. an ambient session whose user_id matches that encoded link user.
//
// An attacker-started (anonymous) state never carries linkUserID, so a logged-in
// victim visiting the callback cannot be linked — even if they present a valid
// session_token on the GET.
func googleCallbackLinkIntent(stateLinkUserID, sessionUserID uint) (linkingUserID uint, isLinking bool) {
	if stateLinkUserID == 0 || sessionUserID == 0 || sessionUserID != stateLinkUserID {
		return 0, false
	}
	return stateLinkUserID, true
}

// wantsGoogleAccountLink reports explicit link intent on the OAuth start request.
func wantsGoogleAccountLink(c *gin.Context) bool {
	v := strings.TrimSpace(strings.ToLower(c.Query("link")))
	return v == "1" || v == "true" || v == "yes"
}

func (s *oauthStateStore) cleanupLoop() {
	for {
		select {
		case <-s.ticker.C:
			s.mu.Lock()
			now := time.Now()
			for k, entry := range s.states {
				if now.After(entry.expiresAt) {
					delete(s.states, k)
				}
			}
			s.mu.Unlock()
		case <-s.done:
			s.ticker.Stop()
			return
		}
	}
}

// AuthHandler handles authentication endpoints
type AuthHandler struct {
	db              *gorm.DB
	authService     *AuthService
	oauthConfig     *OAuthConfig
	stateStore      *oauthStateStore
	staffStateMu    sync.RWMutex
	staffStateStore map[string]StaffOAuthState
	emailServer     *emails.EmailServer
	loginThrottle   *auththrottle.Throttle // P2: per-account login lockout (nil = disabled)
	// registrationLimiter is a durable per-IP hourly cap on account creation
	// (same sliding-window limiter the resend-verification flow uses). The
	// per-minute authLimiter middleware still runs in front of it.
	registrationLimiter   *RateLimiter
	registrationAdmission *runtimecontrol.Service

	// Optional test stubs. Production leaves these nil so the real Google
	// OAuth client and userinfo fetch are used.
	exchangeGoogleCodeFn func(ctx context.Context, code string) (*oauth2.Token, error)
	getGoogleUserInfoFn  func(ctx context.Context, accessToken string) (*GoogleUserInfo, error)
}

// SetLoginThrottle installs the DB-backed per-account login lockout.
func (h *AuthHandler) SetLoginThrottle(t *auththrottle.Throttle) { h.loginThrottle = t }

// SetRegistrationRateLimit configures the per-IP hourly account-creation cap.
// A non-positive limit disables this defense-in-depth limiter for isolated
// harnesses only; production should keep the default or an explicit positive cap.
func (h *AuthHandler) SetRegistrationRateLimit(limit int) {
	if limit <= 0 {
		h.registrationLimiter = nil
		return
	}
	h.registrationLimiter = NewRateLimiter(time.Hour, limit)
}

// SetRegistrationAdmission installs the durable invite/cohort gate. The
// application entrypoint always installs it; direct unit harnesses may omit it.
func (h *AuthHandler) SetRegistrationAdmission(service *runtimecontrol.Service) {
	h.registrationAdmission = service
}

// registrationClosed reports whether REGISTRATION_MODE=closed is in force for
// this handler (the admission service's mode when wired, else the env).
func (h *AuthHandler) registrationClosed() bool {
	if h.registrationAdmission != nil {
		return h.registrationAdmission.RegistrationMode() == config.RegistrationModeClosed
	}
	return config.RegistrationMode() == config.RegistrationModeClosed
}

// respondRegistrationClosed is the shared 403 for every signup path when
// REGISTRATION_MODE=closed. params.reason lets the UI show a localized
// "registration is closed" message instead of the generic forbidden copy.
func respondRegistrationClosed(c *gin.Context) {
	server.RespondWithErrorParams(c, http.StatusForbidden, server.ErrCodeForbidden,
		"Registration is closed on this instance", map[string]interface{}{"reason": "registration_closed"})
}

// NewAuthHandler creates a new auth handler
func NewAuthHandler(db *gorm.DB, emailServer *emails.EmailServer) *AuthHandler {
	handler := &AuthHandler{
		db:                  db,
		authService:         NewAuthService(db),
		oauthConfig:         NewOAuthConfig(),
		stateStore:          newOAuthStateStore(),
		staffStateStore:     make(map[string]StaffOAuthState),
		emailServer:         emailServer,
		registrationLimiter: NewRateLimiter(time.Hour, 5),
	}
	handler.startStaffStateCleanup(5 * time.Minute)
	return handler
}

// generateJWT generates a JWT token for a user (Deprecated: Use server.GenerateUserToken)
func (h *AuthHandler) generateJWT(userID uint, email, role string, sessionID ...uint) (string, error) {
	sid := uint(0)
	if len(sessionID) > 0 {
		sid = sessionID[0]
	}
	return server.GenerateUserToken(userID, email, "", role, sid)
}

// issueEmailOperatorSession is the sole normal-session issuance path for
// email/password identities. When email verification is required,
// registration never calls it: the first operator credential is issued only
// after verification, and subsequent logins reuse exactly the same
// persistence and cookie behavior. When EMAIL_VERIFICATION resolves to "off"
// an unverified identity is signed in as it is — off mode never writes
// email_verified=true, so the flag keeps meaning "this address was proven" —
// provided its email auth row is still bound to the account's current email
// (the same rule the session middleware's live check applies in off mode).
func (h *AuthHandler) issueEmailOperatorSession(c *gin.Context, user *database.User) (string, error) {
	if user == nil || user.ID == 0 {
		return "", ErrEmailNotVerified
	}
	if !user.EmailVerified {
		if config.EmailVerificationRequired() {
			return "", ErrEmailNotVerified
		}
		bound, err := verification.NewService(h.db).IsOperatorVerified(user.ID, "email")
		if err != nil {
			return "", fmt.Errorf("check email identity binding: %w", err)
		}
		if !bound {
			return "", ErrEmailIdentityUnbound
		}
	}
	return h.mintOperatorSession(c, user, "email")
}

// mintOperatorSession persists an operator session for user under provider,
// signs its JWT and writes the operator cookie pair. Callers own every
// identity check; this only mints.
func (h *AuthHandler) mintOperatorSession(c *gin.Context, user *database.User, provider string) (string, error) {
	var token string
	var err error
	var refreshToken string
	if session.GlobalStore != nil {
		uid := user.ID
		sess, createErr := session.GlobalStore.Create(CreateSessionInput{
			UserID:    &uid,
			Provider:  provider,
			IPAddress: c.ClientIP(),
			UserAgent: c.GetHeader("User-Agent"),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		if createErr != nil {
			return "", fmt.Errorf("create operator session: %w", createErr)
		}

		token, err = h.generateJWT(user.ID, user.Email, user.Role, sess.ID)
		if err != nil {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			return "", fmt.Errorf("generate operator token: %w", err)
		}
		if err := session.GlobalStore.UpdateTokenHash(sess.ID, HashToken(token)); err != nil {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			return "", fmt.Errorf("persist operator token: %w", err)
		}
		var refreshErr error
		refreshToken, refreshErr = session.GenerateRefreshToken()
		if refreshErr != nil {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			return "", fmt.Errorf("generate operator refresh token: %w", refreshErr)
		}
		if err := session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshToken), time.Now().Add(7*24*time.Hour)); err != nil {
			_ = session.GlobalStore.RevokeWithReason(sess.ID, session.RevocationReasonInvalidSession)
			return "", fmt.Errorf("persist operator refresh token: %w", err)
		}
	} else {
		token, err = h.generateJWT(user.ID, user.Email, user.Role)
		if err != nil {
			return "", fmt.Errorf("generate operator token: %w", err)
		}
	}

	// Evict leftover staff/customer/parent-domain cookies before writing the
	// new operator pair. A shadowed staff_token used to win Hybrid +
	// session-info and 401 a live email session.
	utils.ClearAllAuthCookies(c)
	if refreshToken != "" {
		utils.SetSessionCookie(c, "refresh_token", refreshToken, 7*24*3600)
	}
	setSessionCookie(c, token)
	return token, nil
}

func (h *AuthHandler) buildVerificationLink(token string) string {
	return getFrontendBaseURL() + "/verify-email?token=" + url.QueryEscape(token)
}

func (h *AuthHandler) sendVerificationEmail(email, name, token string) error {
	verifyLink := h.buildVerificationLink(token)
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = strings.Split(email, "@")[0]
	}
	var user database.User
	if err := h.db.Where("LOWER(email) = LOWER(?)", email).First(&user).Error; err != nil {
		user.LanguageSelected = "" // graceful fallback → EmailFamily default
	}
	lang := locales.EmailFamily(user.LanguageSelected)
	return h.emailServer.SendEmailVerificationEmail([]string{email}, displayName, verifyLink, lang)
}

// Register handles email/password registration
func (h *AuthHandler) Register(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("register_attempt").Inc()

	if clientIPDebugEnabled() {
		logger.Logger.Infof("[ClientIPDebug] register client_ip=%q remote_addr=%q xff=%q cf_connecting_ip=%q",
			c.ClientIP(), c.Request.RemoteAddr,
			c.GetHeader("X-Forwarded-For"), c.GetHeader("CF-Connecting-IP"))
	}

	// REGISTRATION_MODE=closed: refuse before any lookup so the endpoint is
	// neither an account-creation path nor a duplicate-email oracle.
	if h.registrationClosed() {
		metrics.AuthOperations.WithLabelValues("register_closed").Inc()
		respondRegistrationClosed(c)
		return
	}

	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondBindError(c, err)
		return
	}
	req.Email = NormalizeEmail(req.Email)

	// Per-IP hourly registration cap (defense in depth behind the per-minute
	// authLimiter): account creation is expensive (bcrypt + verification
	// email) and an unbounded hourly rate also enables duplicate-email
	// enumeration at scale via the 409 response.
	// Peek only — the budget is consumed on SUCCESSFUL creation below, so
	// 400s/409s can't starve a shared IP. Keyed per IPv4 address or IPv6 /64
	// (one subscriber can rotate through a whole /64).
	if h.registrationLimiter != nil && !h.registrationLimiter.Peek(middleware.ClientRateLimitKey(c)) {
		metrics.AuthOperations.WithLabelValues("register_rate_limited").Inc()
		server.RespondWithError(c, http.StatusTooManyRequests, server.ErrCodeRateLimited, "Too many registration attempts. Please try again later.")
		return
	}

	// Validate email
	if !ValidateEmail(req.Email) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid email address")
		return
	}

	// Validate password
	if err := ValidatePassword(req.Password); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeWeakPassword, err.Error())
		return
	}

	// Validate display name. The name is echoed into the verification email that
	// greets the registrant, so reject newlines/control characters and URLs that
	// a mail client would auto-linkify (a weak phishing/abuse vector).
	if err := ValidateName(req.Name); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}
	req.Name = strings.TrimSpace(req.Name)

	// Keep active registrations reserved, but release an abandoned email-only
	// placeholder after its verification window expires. The old identity is
	// deleted rather than revived with stale credentials.
	var existingUser database.User
	lookupErr := h.db.Where("LOWER(email) = LOWER(?)", req.Email).First(&existingUser).Error
	if lookupErr == nil {
		released, releaseErr := verification.NewService(h.db).ReleaseExpiredEmailReservation(req.Email, time.Now())
		if releaseErr != nil {
			logger.Logger.Errorf("[Auth] Failed to evaluate expired registration reservation: %v", releaseErr)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to register user")
			return
		}
		if !released {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeDuplicateEmail, "User with this email already exists")
			return
		}
	} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
		logger.Logger.Errorf("[Auth] Failed to inspect registration reservation: %v", lookupErr)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to register user")
		return
	}

	// Create auth record. verificationToken is the PLAINTEXT to email; the auth
	// record persists only its hash.
	authRecord, verificationToken, err := h.authService.RegisterWithEmail(req.Email, req.Password, req.Name)
	if err != nil {
		if errors.Is(err, ErrUserExists) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeDuplicateEmail, "User with this email already exists")
			return
		}
		logger.Logger.Errorf("[Auth] Registration error: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to register user")
		return
	}

	// Create user + auth in one transaction so a failed auth insert cannot leave
	// an orphan user that blocks re-registration on the unique email.
	// LanguageSelected seeds from the funnel's operator locale (when valid) so
	// sendVerificationEmail — which re-reads this row — localizes immediately.
	language := ""
	if lang := strings.TrimSpace(req.Language); lang != "" && locales.IsOperatorLocale(lang) {
		language = lang
	}
	now := time.Now()
	signupSource := strings.TrimSpace(req.SignupSource)
	if len(signupSource) > 64 {
		signupSource = signupSource[:64]
	}
	user := database.User{
		Email:            req.Email,
		Name:             req.Name,
		AuthMethod:       "email",
		Role:             "user",
		LanguageSelected: language,
		SignupSource:     signupSource,
		ActivatedAt:      &now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	createIdentity := func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		authRecord.UserID = user.ID
		return tx.Create(authRecord).Error
	}
	var createErr error
	if h.registrationAdmission != nil {
		createErr = h.registrationAdmission.Admit(c.Request.Context(), req.Email, req.InviteCode, createIdentity)
	} else {
		createErr = h.db.Transaction(createIdentity)
	}
	if createErr != nil {
		if errors.Is(createErr, runtimecontrol.ErrRegistrationClosed) {
			respondRegistrationClosed(c)
			return
		}
		if errors.Is(createErr, runtimecontrol.ErrInviteRequired) || errors.Is(createErr, runtimecontrol.ErrInviteExpired) {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "A valid launch invitation is required")
			return
		}
		if errors.Is(createErr, runtimecontrol.ErrCohortFull) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "This launch cohort is full")
			return
		}
		if errors.Is(createErr, runtimecontrol.ErrInviteAlreadyClaimed) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "This email has already claimed an invitation")
			return
		}
		if database.IsUniqueConstraintError(createErr) {
			// Concurrent duplicate registration: both requests passed the
			// pre-checks; the unique email index decided. Same contract as
			// the pre-check — 409, not a 500 that reads as a server fault.
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeDuplicateEmail, "User with this email already exists")
			return
		}
		logger.Logger.Errorf("[Auth] Failed to create user+auth: %v", createErr)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to register user")
		return
	}

	// Consume the hourly creation budget only now that an account actually
	// exists — failed attempts above did not count against this IP.
	if h.registrationLimiter != nil {
		h.registrationLimiter.Record(middleware.ClientRateLimitKey(c))
	}

	// RegisterWithEmail decided the verification mode: no token means
	// EMAIL_VERIFICATION is off, so nothing is mailed and the new operator is
	// signed straight in. Both rows stay email_verified=false either way.
	if verificationToken == "" {
		h.respondRegisteredWithoutVerification(c, &user)
		return
	}

	verifyLink := h.buildVerificationLink(verificationToken)
	message := "Account created. Check your email to verify your account before signing in."
	if err := h.sendVerificationEmail(user.Email, user.Name, verificationToken); err != nil {
		logger.Logger.Errorf("[Auth] Failed to send verification email: %v", err)
		if verificationLinkForResponse(verifyLink) != "" {
			message = "Account created. We could not send the verification email automatically, but you can verify your email with the provided link."
		} else {
			message = "Account created. We could not send the verification email automatically. Please request a new verification email before signing in."
		}
	}

	metrics.AuthOperations.WithLabelValues("register_success").Inc()
	metrics.AuthOperations.WithLabelValues("registration_pending_created").Inc()
	if err := metrics.RefreshPendingEmailRegistrationState(h.db); err != nil {
		logger.Logger.Warnf("[Auth] Failed to refresh pending-registration metrics: %v", err)
	}

	c.JSON(http.StatusCreated, AuthResponse{
		Success:              true,
		Token:                "",
		UserID:               user.ID,
		Email:                user.Email,
		Name:                 user.Name,
		WalletLinked:         false,
		Provider:             "email",
		IsNewUser:            true,
		Message:              message,
		VerificationRequired: true,
		VerificationURL:      verificationLinkForResponse(verifyLink),
	})
}

// respondRegisteredWithoutVerification finishes a registration created while
// EMAIL_VERIFICATION is off: no verification email is sent and the new
// operator is signed in through the normal session path. If the session
// cannot be issued the account still exists, so the response stays 201 with
// no token and the operator can sign in normally.
func (h *AuthHandler) respondRegisteredWithoutVerification(c *gin.Context, user *database.User) {
	metrics.AuthOperations.WithLabelValues("register_success").Inc()

	message := "Account created."
	token, err := h.issueEmailOperatorSession(c, user)
	if err != nil {
		logger.Logger.Errorf("[Auth] Failed to issue session for new unverified-mode account: %v", err)
		token = ""
		message = "Account created. Please sign in to continue."
	}

	c.JSON(http.StatusCreated, AuthResponse{
		Success:              true,
		Token:                token,
		UserID:               user.ID,
		Email:                user.Email,
		Name:                 user.Name,
		WalletLinked:         false,
		Provider:             "email",
		IsNewUser:            true,
		Message:              message,
		VerificationRequired: false,
	})
}

// Login handles email/password login
func (h *AuthHandler) Login(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("login_attempt").Inc()

	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		return
	}
	req.Email = NormalizeEmail(req.Email)

	// Per-account lockout: check BEFORE password verification so a locked account
	// cannot be probed even with the correct password.
	const loginThrottleKind = PasswordLoginThrottleKind
	if h.loginThrottle != nil {
		locked, _, lerr := h.loginThrottle.IsLocked(req.Email, loginThrottleKind)
		if lerr != nil {
			// Fail CLOSED: the lockout backend is unavailable (e.g. the
			// auth_attempts table is missing, or the DB is
			// erroring), so we cannot prove this account is not under a
			// brute-force lock. Deny rather than expose password verification
			// unthrottled. Logged + metered so operators see the throttle backend
			// is degraded instead of it silently failing open.
			metrics.AuthOperations.WithLabelValues("login_throttle_unavailable").Inc()
			logger.Logger.Errorf("[Auth] login throttle backend error for %s; failing closed: %v", logger.RedactEmail(req.Email), lerr)
			server.RespondWithError(c, http.StatusTooManyRequests, server.ErrCodeRateLimited,
				"Sign-in is temporarily unavailable. Please try again shortly.")
			return
		}
		if locked {
			server.RespondWithError(c, http.StatusTooManyRequests, server.ErrCodeRateLimited,
				"Too many failed sign-in attempts. Please try again later.")
			return
		}
	}

	// Authenticate
	authRecord, err := h.authService.LoginWithEmail(req.Email, req.Password)
	if err != nil {
		if errors.Is(err, ErrInvalidCredentials) {
			if h.loginThrottle != nil {
				if rerr := h.loginThrottle.Record(req.Email, loginThrottleKind); rerr != nil {
					logger.Logger.Errorf("[Auth] failed to record failed login attempt for %s: %v", logger.RedactEmail(req.Email), rerr)
				}
			}
			server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeInvalidCredentials, "Invalid email or password")
			return
		}
		if errors.Is(err, ErrEmailNotVerified) {
			params := map[string]interface{}{
				"requires_email_verification": true,
			}
			message := "Email not verified. Check your inbox for a verification link before signing in."
			// Route through ResendVerification: it owns BOTH the 3/hour
			// per-email limiter and the token rotation, so repeated
			// unverified logins can no longer rotate the token outside the
			// limiter and invalidate the link the user was just emailed.
			// token=="" (rate-limited / already verified) keeps the
			// check-your-inbox message without minting anything.
			if token, tokenErr := h.authService.ResendVerification(req.Email); tokenErr == nil && token != "" {
				verifyLink := h.buildVerificationLink(token)
				if emailErr := h.sendVerificationEmail(req.Email, "", token); emailErr != nil {
					logger.Logger.Errorf("[Auth] Failed to resend verification email: %v", emailErr)
					message = "Email not verified. We generated a new verification link for you."
				} else {
					message = "Email not verified. We sent you a new verification email."
				}
				params["verification_url"] = verificationLinkForResponse(verifyLink)
			}
			server.RespondWithErrorParams(c, http.StatusForbidden, server.ErrCodeForbidden, message, params)
			return
		}
		logger.Logger.Errorf("[Auth] Login error: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Login failed")
		return
	}

	// Get user
	var user database.User
	if err := h.db.First(&user, authRecord.UserID).Error; err != nil {
		logger.Logger.Errorf("[Auth] User not found: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeUserNotFound, "User not found")
		return
	}

	// P1-11: a soft-deleted account must not sign back in — the 30-day grace
	// window is a support-mediated restore path, not a self-service undo.
	// 403 (not 401): the credentials were correct, so this must not feed the
	// failed-login throttle or read as "wrong password".
	if user.DeletedAt != nil {
		server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
			"This account is scheduled for deletion. Contact support to restore it.")
		return
	}

	// Clear the failed-attempt counter on successful authentication.
	if h.loginThrottle != nil {
		if cerr := h.loginThrottle.Clear(req.Email, loginThrottleKind); cerr != nil {
			logger.Logger.Errorf("[Auth] failed to clear login throttle for %s: %v", logger.RedactEmail(req.Email), cerr)
		}
	}

	// Check for linked wallet
	walletAddress, _ := h.authService.GetUserWallet(user.ID)

	// Verification and login share one normal-session issuance path. With
	// EMAIL_VERIFICATION=off LoginWithEmail lets an unverified record through
	// and issueEmailOperatorSession signs it in as it is (no flag is written).
	token, err := h.issueEmailOperatorSession(c, &user)
	if err != nil {
		switch {
		case errors.Is(err, ErrEmailIdentityUnbound):
			// The sign-in identity no longer matches the account's current
			// email (an email change left it behind). Off mode only vouches
			// for the address the account holds now, so refuse the stale
			// identity like an unverified one instead of failing with 500.
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
				"This sign-in email is no longer the account's email. Sign in with the account's current email.")
		case errors.Is(err, ErrEmailNotVerified):
			// A verified credential on an account row that is not verified
			// (inconsistent legacy data): refuse like any unverified account.
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
				"Email not verified. Check your inbox for a verification link before signing in.")
		default:
			logger.Logger.Errorf("[Auth] Failed to issue email operator session: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create session")
		}
		return
	}

	metrics.AuthOperations.WithLabelValues("login_success").Inc()
	c.JSON(http.StatusOK, AuthResponse{
		Success:       true,
		Token:         token,
		UserID:        user.ID,
		Email:         user.Email,
		Name:          user.Name,
		WalletLinked:  walletAddress != "",
		WalletAddress: walletAddress,
		Provider:      "email",
		IsNewUser:     false,
	})
}

// GoogleAuthURL returns the Google OAuth URL
func (h *AuthHandler) GoogleAuthURL(c *gin.Context) {
	state, err := GenerateOAuthState()
	if err != nil {
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to generate state")
		return
	}

	// Explicit link intent must be declared at start while authenticated and
	// encoded into the bound state. Ambient login at callback is not enough (SEC-1).
	var linkUserID uint
	if wantsGoogleAccountLink(c) {
		linkUserID = sessionUserIDFromCookie(c)
	}

	h.stateStore.Set(state, time.Now().Add(oauthStateTTL), c.Query("invite_code"), linkUserID)
	// Bind state to this browser so an attacker-minted state cannot be completed
	// by a victim who only has a session_token (SEC-1).
	setOAuthStateCookie(c, state)

	url := h.oauthConfig.GetGoogleAuthURL(state)
	if url == "" {
		server.RespondWithError(c, http.StatusServiceUnavailable, server.ErrCodeServiceUnavailable, "Google OAuth not configured")
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": url, "state": state})
}

func (h *AuthHandler) exchangeGoogleCode(ctx context.Context, code string) (*oauth2.Token, error) {
	if h.exchangeGoogleCodeFn != nil {
		return h.exchangeGoogleCodeFn(ctx, code)
	}
	return h.oauthConfig.ExchangeGoogleCode(ctx, code)
}

func (h *AuthHandler) lookupGoogleUserInfo(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	if h.getGoogleUserInfoFn != nil {
		return h.getGoogleUserInfoFn(ctx, accessToken)
	}
	return GetGoogleUserInfo(ctx, accessToken)
}

// unprovenEmailAccountMessage is the 409 copy for ErrUnprovenEmailAccount. The
// remedy depends on the mode: with verification required the unverified
// password cannot sign in (Login answers 403 and emails a fresh link), so the
// only working path is the verification link; with verification off the
// password signs in and Google can be linked from account settings.
func unprovenEmailAccountMessage() string {
	if config.EmailVerificationRequired() {
		return "An account with this email already exists but its email address was never verified. " +
			"If you created it, open the verification link sent to this address (signing in with its password sends a fresh one), " +
			"then continue with Google again. Otherwise ask the administrator."
	}
	return "An account with this email already exists but its email address was never verified. " +
		"Sign in with its password and link Google from your account settings, or ask the administrator."
}

// GoogleCallback handles Google OAuth callback
func (h *AuthHandler) GoogleCallback(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("google_auth_attempt").Inc()

	code := c.Query("code")
	state := c.Query("state")

	// Staff Google login shares this same redirect_uri (the staff auth URL is
	// built with the regular callback) but keeps its state in a separate store.
	// Route staff states to the staff handler — otherwise they fail the user
	// state check below with a 400 and staff can never sign in with Google.
	if h.isStaffState(state) {
		h.GoogleStaffCallback(c)
		return
	}

	// Browser-bound state cookie must match the query state before we consume
	// the server-side single-use entry (SEC-1).
	if !oauthStateCookieMatches(c, state) {
		clearOAuthStateCookie(c)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid or expired state")
		return
	}

	// Validate state (single-use, thread-safe)
	stateEntry, validState := h.stateStore.Consume(state)
	clearOAuthStateCookie(c)
	if !validState {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid or expired state")
		return
	}
	inviteCode := stateEntry.inviteCode

	// Exchange code for token
	ctx := context.Background()
	token, err := h.exchangeGoogleCode(ctx, code)
	if err != nil {
		logger.Logger.Errorf("[Auth] Google token exchange failed: %v", err)
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Failed to exchange authorization code")
		return
	}

	// Account linking requires link-intent encoded at OAuth start — never infer
	// isLinking solely from an ambient session_token on this GET (SEC-1).
	linkingUserID, isLinking := googleCallbackLinkIntent(stateEntry.linkUserID, sessionUserIDFromCookie(c))
	if isLinking {
		logger.Logger.Infof("[Auth] Completing explicit Google account link for user %d", linkingUserID)
	}

	// Get user info
	userInfo, err := h.lookupGoogleUserInfo(ctx, token.AccessToken)
	if err != nil {
		logger.Logger.Errorf("[Auth] Failed to get Google user info: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to get user info")
		return
	}
	userInfo.Email = NormalizeEmail(userInfo.Email)

	// Create or update auth record. EmailVerified is set from Google's real
	// verified_email flag — never assumed true (see ErrUnverifiedOAuthEmail).
	// The access token stays in memory for the userinfo lookup above and is
	// not written to user_auths.
	authRecord, isNewUser, err := h.authService.CreateOrUpdateOAuthUser(
		"google",
		userInfo.ID,
		userInfo.Email,
		userInfo.Name,
		userInfo.VerifiedEmail,
	)
	if err != nil {
		logger.Logger.Errorf("[Auth] Failed to create/update OAuth user: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to process authentication")
		return
	}

	user, _, err := h.resolveGoogleCallbackUserWithInvite(c.Request.Context(), userInfo, authRecord, isNewUser, isLinking, linkingUserID, inviteCode)
	if err != nil {
		if errors.Is(err, runtimecontrol.ErrRegistrationClosed) {
			respondRegistrationClosed(c)
			return
		}
		if errors.Is(err, runtimecontrol.ErrInviteRequired) || errors.Is(err, runtimecontrol.ErrInviteExpired) {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "A valid launch invitation is required")
			return
		}
		if errors.Is(err, runtimecontrol.ErrCohortFull) || errors.Is(err, runtimecontrol.ErrInviteAlreadyClaimed) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "This launch invitation is no longer available")
			return
		}
		if errors.Is(err, ErrArchivedAccount) {
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden,
				"This account is scheduled for deletion. Contact support to restore it.")
			return
		}
		if errors.Is(err, ErrUnprovenEmailAccount) {
			// Pre-account-takeover guard: the matching account's password
			// credential was never proven to own this address and the account
			// is in use, so the Google identity is not merged into it.
			logger.LogSecurity(
				"google_oauth_unproven_email_merge_blocked",
				c.ClientIP(),
				"email_domain="+emailDomainForLog(userInfo.Email),
				"warning",
			)
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, unprovenEmailAccountMessage())
			return
		}
		if errors.Is(err, ErrUnverifiedOAuthEmail) {
			// Account-takeover guard: an unverified Google email must not be
			// merged into (or collide with) a pre-existing account. Fail closed.
			logger.LogSecurity(
				"google_oauth_unverified_email_merge_blocked",
				c.ClientIP(),
				"email_domain="+emailDomainForLog(userInfo.Email),
				"warning",
			)
			server.RespondWithError(c, http.StatusForbidden, server.ErrCodeForbidden, "Google email is not verified")
			return
		}
		logger.Logger.Errorf("[Auth] Failed to resolve Google user: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to process authentication")
		return
	}
	// Create session and generate JWT with OAuth profile info
	var jwtToken string
	if session.GlobalStore != nil {
		sess, sessErr := session.GlobalStore.Create(CreateSessionInput{
			UserID:    &user.ID,
			Provider:  "google",
			IPAddress: c.ClientIP(),
			UserAgent: c.GetHeader("User-Agent"),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		if sessErr != nil {
			logger.Logger.Errorf("[Auth] Failed to create session: %v", sessErr)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create session")
			return
		}
		jwtToken, err = server.GenerateOAuthUserToken(user.ID, user.Email, userInfo.Name, userInfo.Picture, user.Role, sess.ID)
		if err != nil {
			logger.Logger.Errorf("[Auth] Failed to generate token: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to generate token")
			return
		}
		if err := session.GlobalStore.UpdateTokenHash(sess.ID, HashToken(jwtToken)); err != nil {
			logger.Logger.Errorf("[Auth] Failed to persist token hash for session %v: %v", sess.ID, err)
		}
		refreshToken, _ := session.GenerateRefreshToken()
		if err := session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshToken), time.Now().Add(7*24*time.Hour)); err != nil {
			logger.Logger.Errorf("[Auth] Failed to rotate refresh token for session %v: %v", sess.ID, err)
		}
		// Evict rival realm cookies (staff/customer) before writing the
		// owner pair. Email login already does this; leaving a live
		// staff_token here lets Hybrid/session-info keep the staff principal.
		utils.ClearAllAuthCookies(c)
		utils.SetSessionCookie(c, "refresh_token", refreshToken, 7*24*3600)
	} else {
		jwtToken, err = server.GenerateOAuthUserToken(user.ID, user.Email, userInfo.Name, userInfo.Picture, user.Role)
		if err != nil {
			logger.Logger.Errorf("[Auth] Failed to generate token: %v", err)
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to generate token")
			return
		}
		utils.ClearAllAuthCookies(c)
	}

	metrics.AuthOperations.WithLabelValues("google_auth_success").Inc()

	setSessionCookie(c, jwtToken)
	c.Redirect(http.StatusTemporaryRedirect, resolveSafeFrontendRedirect(c, "/dashboard"))
}

// resolveGoogleCallbackUserWithInvite resolves the database user for a Google OAuth
// callback and links the Google auth record appropriately. It returns the
// resolved user and whether a brand-new user was created (for analytics).
//
// Security: when isNewUser is true and an account with the same email already
// exists (registered via a different method), linking the Google identity into
// that account is an account-takeover vector unless Google has actually verified
// the email. If userInfo.VerifiedEmail is false in that case, this fails closed
// with ErrUnverifiedOAuthEmail — it does NOT merge, and does NOT create a
// duplicate account that would collide on email. The explicit-linking branch
// (isLinking) is only entered when OAuth start encoded linkUserID into the
// cookie-bound state AND the callback session still matches that user (SEC-1);
// ambient session_token on GET alone must never set isLinking.
func (h *AuthHandler) resolveGoogleCallbackUserWithInvite(
	ctx context.Context,
	userInfo *GoogleUserInfo,
	authRecord *UserAuth,
	isNewUser bool,
	isLinking bool,
	linkingUserID uint,
	inviteCode string,
) (database.User, bool, error) {
	var user database.User
	// Provider assertions are authoritative. An unverified OAuth email may be
	// linked only by a user who already proved control of an active account; it
	// must never create or log into an operator identity on its own.
	if !userInfo.VerifiedEmail && !isLinking {
		return database.User{}, false, ErrUnverifiedOAuthEmail
	}

	if !isNewUser {
		// Existing Google auth record — fetch its user.
		if err := h.db.First(&user, authRecord.UserID).Error; err != nil {
			return database.User{}, false, err
		}
		if user.DeletedAt != nil {
			return database.User{}, false, ErrArchivedAccount
		}
		if userInfo.Picture != "" && user.Picture != userInfo.Picture {
			h.db.Model(&user).Update("picture", userInfo.Picture)
			user.Picture = userInfo.Picture
		}
		if err := h.authService.SaveAuth(authRecord); err != nil {
			logger.Logger.Errorf("[Auth] Failed to update auth record: %v", err)
		}
		return user, false, nil
	}

	// New Google auth record. Decide how to attach it.
	existingUserErr := h.db.Where("LOWER(email) = LOWER(?)", userInfo.Email).First(&user).Error
	if existingUserErr == nil {
		if user.DeletedAt != nil {
			return database.User{}, false, ErrArchivedAccount
		}
		// An account with this email already exists. Merging the Google identity
		// into it is only safe when Google actually verified the email —
		// otherwise an attacker who controls an unverified Google account with a
		// victim's email could take over the victim's account. Fail closed.
		if !userInfo.VerifiedEmail {
			return database.User{}, false, ErrUnverifiedOAuthEmail
		}
		// Google proved the address; the existing account may not have. An
		// email/password credential nobody verified (registered while
		// EMAIL_VERIFICATION was off, or still waiting for its link) belongs
		// to whoever chose the password, not necessarily to this Google user.
		// An unused account loses that credential before the merge; a used one
		// is refused. Explicit link mode from the same account already proved
		// control of it and is exempt.
		if !(isLinking && linkingUserID == user.ID) {
			if err := verification.NewService(h.db).DropUnprovenEmailCredential(user.ID); err != nil {
				if errors.Is(err, verification.ErrUnprovenEmailAccount) {
					return database.User{}, false, ErrUnprovenEmailAccount
				}
				return database.User{}, false, err
			}
		}
		logger.Logger.Infof("[Auth] Linking Google auth to existing user (verified email): %s", logger.RedactEmail(userInfo.Email))
		authRecord.UserID = user.ID
		if err := h.authService.CreateAuth(authRecord); err != nil {
			return database.User{}, false, err
		}
		if userInfo.Picture != "" && user.Picture != userInfo.Picture {
			h.db.Model(&user).Update("picture", userInfo.Picture)
			user.Picture = userInfo.Picture
		}
		return user, false, nil
	}

	if isLinking {
		// Explicit link-mode: OAuth start was authenticated with link intent and
		// the bound state + matching session were validated by the caller (SEC-1).
		// No verified-email gate is needed here — ownership was proven at start.
		if err := h.db.First(&user, linkingUserID).Error; err != nil {
			return database.User{}, false, err
		}
		if user.DeletedAt != nil {
			return database.User{}, false, ErrArchivedAccount
		}
		logger.Logger.Infof("[Auth] Linking Google auth to currently logged in user: %d", linkingUserID)
		authRecord.UserID = linkingUserID
		if err := h.authService.CreateAuth(authRecord); err != nil {
			return database.User{}, false, err
		}
		return user, false, nil
	}

	// Brand-new user: no existing account, no active session to link to.
	user = database.User{
		Email:      userInfo.Email,
		Name:       userInfo.Name,
		Picture:    userInfo.Picture,
		AuthMethod: "google",
		Role:       "user",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	createIdentity := func(tx *gorm.DB) error {
		if err := tx.Create(&user).Error; err != nil {
			return err
		}
		authRecord.UserID = user.ID
		return tx.Create(authRecord).Error
	}
	var err error
	if h.registrationAdmission != nil {
		err = h.registrationAdmission.Admit(ctx, userInfo.Email, inviteCode, createIdentity)
	} else {
		err = h.db.Transaction(createIdentity)
	}
	if err != nil {
		return database.User{}, false, err
	}
	return user, true, nil
}

// LinkWallet links a wallet to the authenticated user
func (h *AuthHandler) LinkWallet(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("link_wallet_attempt").Inc()

	// Get user ID from context (set by auth middleware)
	userIDInterface, exists := c.Get("user_id")
	if !exists {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Not authenticated")
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		// Try float64 (JWT claims often decode numbers as float64)
		if userIDFloat, ok := userIDInterface.(float64); ok {
			userID = uint(userIDFloat)
		} else {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Invalid user ID")
			return
		}
	}

	var req LinkWalletRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		return
	}

	// Verify the signature
	address := strings.ToLower(req.Address)
	siwe, ok := server.ParseAndValidateSIWE(c, req.Message)
	if !ok {
		return
	}
	if siwe.Address != address {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid wallet message")
		return
	}
	chainID := siwe.ChainID

	// Stateless check that we issued this nonce for this address, then the
	// signature, then an atomic spend: the old Get-then-Delete let two
	// concurrent requests reuse one challenge.
	challenge, ok := server.ChallengeStore.Verify(siwe.Nonce, address)
	if !ok {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid or expired challenge")
		return
	}

	if !utils.VerifySignature(address, req.Message, req.Signature, chainID) {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid signature")
		return
	}

	if err := server.ChallengeStore.TryRedeem(challenge); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid or expired challenge")
		return
	}

	// Link wallet
	if err := h.authService.LinkWallet(userID, address); err != nil {
		if errors.Is(err, ErrWalletAlreadyLinked) {
			server.RespondWithError(c, http.StatusConflict, server.ErrCodeConflict, "Wallet already linked to another account")
			return
		}
		logger.Logger.Errorf("[Auth] Failed to link wallet: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to link wallet")
		return
	}

	// Update user's address field
	if err := h.db.Model(&database.User{}).Where("id = ?", userID).Update("address", address).Error; err != nil {
		logger.Logger.Errorf("[Auth] Failed to update user address: %v", err)
	}

	metrics.AuthOperations.WithLabelValues("link_wallet_success").Inc()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Wallet linked successfully",
		"address": address,
	})
}

// UnlinkWallet removes the wallet link from the authenticated user
func (h *AuthHandler) UnlinkWallet(c *gin.Context) {
	userIDInterface, exists := c.Get("user_id")
	if !exists {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Not authenticated")
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		if userIDFloat, ok := userIDInterface.(float64); ok {
			userID = uint(userIDFloat)
		} else {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Invalid user ID")
			return
		}
	}

	if err := h.authService.UnlinkWallet(userID); err != nil {
		logger.Logger.Errorf("[Auth] Failed to unlink wallet: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to unlink wallet")
		return
	}

	// Clear user's address field
	if err := h.db.Model(&database.User{}).Where("id = ?", userID).Update("address", "").Error; err != nil {
		logger.Logger.Errorf("[Auth] Failed to clear user address: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Wallet unlinked successfully",
	})
}

// GetAuthMethods returns the authentication methods for the current user
func (h *AuthHandler) GetAuthMethods(c *gin.Context) {
	userIDInterface, exists := c.Get("user_id")
	if !exists {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Not authenticated")
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		if userIDFloat, ok := userIDInterface.(float64); ok {
			userID = uint(userIDFloat)
		} else {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Invalid user ID")
			return
		}
	}

	auths, err := h.authService.GetAuthByUserID(userID)
	if err != nil {
		logger.Logger.Errorf("[Auth] Failed to get auth methods: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to get auth methods")
		return
	}

	methods := make([]gin.H, 0, len(auths))
	for _, auth := range auths {
		method := gin.H{
			"provider":   auth.Provider,
			"created_at": auth.CreatedAt,
		}
		if auth.Provider == "wallet" && auth.WalletAddress != "" {
			method["wallet_address"] = auth.WalletAddress
		}
		if auth.Provider == "email" {
			method["email_verified"] = auth.EmailVerified
		}
		methods = append(methods, method)
	}

	c.JSON(http.StatusOK, gin.H{"methods": methods})
}

// RequestPasswordReset initiates a password reset
func (h *AuthHandler) RequestPasswordReset(c *gin.Context) {
	var req PasswordResetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		return
	}
	req.Email = NormalizeEmail(req.Email)

	token, err := h.authService.RequestPasswordReset(req.Email)
	if err != nil || token == "" {
		// Don't reveal if email exists; also covers per-email rate limiting
		// (token == "" with nil err) so abused inboxes keep a generic 200.
		c.JSON(http.StatusOK, gin.H{"message": "If an account exists with this email, a reset link will be sent"})
		return
	}

	// Build the password reset link
	resetLink := getFrontendBaseURL() + "/reset-password?token=" + url.QueryEscape(token)

	// Send password reset email
	var user database.User
	if err := h.db.Where("LOWER(email) = LOWER(?)", req.Email).First(&user).Error; err != nil {
		user.LanguageSelected = ""
	}
	lang := locales.EmailFamily(user.LanguageSelected)
	if err := h.emailServer.SendPasswordResetEmail([]string{req.Email}, resetLink, lang); err != nil {
		logger.Logger.Errorf("[Auth] Failed to send password reset email: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"message": "If an account exists with this email, a reset link will be sent"})
}

type ResendVerificationRequest struct {
	Email string `json:"email" binding:"required,email"`
}

// ResendVerification dispatches a fresh verification email if the account
// exists and is unverified. Always returns 200 to prevent enumeration.
func (h *AuthHandler) ResendVerification(c *gin.Context) {
	var req ResendVerificationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		return
	}
	req.Email = NormalizeEmail(req.Email)
	token, err := h.authService.ResendVerification(req.Email)
	if err != nil {
		metrics.AuthOperations.WithLabelValues("verification_resend_failure").Inc()
		logger.Logger.Errorf("[Auth] ResendVerification error: %v", err)
		// Still return 200 — log internally only.
	} else if token != "" {
		if emailErr := h.sendVerificationEmail(req.Email, "", token); emailErr != nil {
			metrics.AuthOperations.WithLabelValues("verification_resend_failure").Inc()
			logger.Logger.Errorf("[Auth] Failed to resend verification email: %v", emailErr)
		}
	}
	c.JSON(http.StatusOK, gin.H{"message": "If your account is unverified, a fresh link is on its way."})
}

// ResetPassword resets the password
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req PasswordResetConfirm
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		return
	}

	if err := ValidatePassword(req.NewPassword); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, err.Error())
		return
	}

	userID, err := h.authService.ResetPassword(req.Token, req.NewPassword)
	if err != nil {
		if errors.Is(err, ErrTokenInvalid) || errors.Is(err, ErrTokenExpired) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid or expired reset token")
			return
		}
		logger.Logger.Errorf("[Auth] Password reset error: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to reset password")
		return
	}

	// Revoke every operator session for this identity, including
	// production-shaped address-keyed web3 rows (user_id NULL).
	if userID != 0 && session.GlobalStore != nil {
		if err := session.GlobalStore.RevokeAllLinkedOperatorSessions(
			userID,
			linkedOperatorAddresses(h, userID),
			session.RevocationReasonSecurityReset,
		); err != nil {
			logger.Logger.Errorf("[Auth] Failed to revoke sessions after password reset for user %d: %v", userID, err)
			server.RespondWithError(c, http.StatusInternalServerError, "sign_out_partial", "Password was reset, but other sessions could not be signed out. Sign in and sign out of every device, or contact support.")
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Password reset successfully"})
}

func linkedOperatorAddresses(h *AuthHandler, userID uint) []string {
	var addrs []string
	if h.db != nil {
		var user database.User
		if err := h.db.First(&user, userID).Error; err == nil && strings.TrimSpace(user.Address) != "" {
			addrs = append(addrs, user.Address)
		}
	}
	if h.authService != nil {
		if wallet, err := h.authService.GetUserWallet(userID); err == nil && strings.TrimSpace(wallet) != "" {
			addrs = append(addrs, wallet)
		}
	}
	return addrs
}

// VerifyEmail verifies the user's email
func (h *AuthHandler) VerifyEmail(c *gin.Context) {
	var req VerifyEmailRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid request")
		return
	}

	identity, err := h.authService.VerifyEmailIdentity(req.Token)
	if err != nil {
		if errors.Is(err, ErrTokenInvalid) || errors.Is(err, ErrTokenExpired) {
			server.RespondWithError(c, http.StatusBadRequest, server.ErrCodeInvalidInput, "Invalid or expired verification token")
			return
		}
		logger.Logger.Errorf("[Auth] Email verification error: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to verify email")
		return
	}
	if identity.RegistrationLatency > 0 {
		metrics.EmailVerificationLatency.Observe(identity.RegistrationLatency.Seconds())
	}
	if err := metrics.RefreshPendingEmailRegistrationState(h.db); err != nil {
		logger.Logger.Warnf("[Auth] Failed to refresh verification-state metrics: %v", err)
	}

	token, err := h.issueEmailOperatorSession(c, &identity.User)
	if err != nil {
		logger.Logger.Errorf("[Auth] Failed to issue verified operator session: %v", err)
		server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Failed to create session")
		return
	}

	walletAddress, _ := h.authService.GetUserWallet(identity.User.ID)
	metrics.AuthOperations.WithLabelValues("email_verification_success").Inc()
	c.JSON(http.StatusOK, AuthResponse{
		Success:              true,
		Token:                token,
		UserID:               identity.User.ID,
		Email:                identity.User.Email,
		Name:                 identity.User.Name,
		WalletLinked:         walletAddress != "",
		WalletAddress:        walletAddress,
		Provider:             "email",
		IsNewUser:            true,
		Message:              "Email verified successfully",
		VerificationRequired: false,
	})
}

func verificationLinkForResponse(link string) string {
	if utils.IsProduction() {
		return ""
	}
	return link
}

// Logout logs out the user. If server-side revocation fails, cookies are still
// cleared and the client gets 500 so it can retry — a local sign-out must not
// look complete while the token stays replayable.
func (h *AuthHandler) Logout(c *gin.Context) {
	// Revoke by access JWT and/or refresh cookie so logout remains containment
	// even when the access token is already missing/expired.
	var revokeErr error
	if session.GlobalStore != nil {
		if tokenString := extractLogoutToken(c); tokenString != "" {
			if err := session.GlobalStore.RevokeByTokenHashWithReason(HashToken(tokenString), session.RevocationReasonUserLogout); err != nil {
				logger.Logger.Errorf("[Auth] Failed to revoke session on logout: %v", err)
				revokeErr = err
			}
		}
		if refreshRaw, err := c.Cookie("refresh_token"); err == nil && refreshRaw != "" {
			if err := session.GlobalStore.RevokeByRefreshTokenHashWithReason(HashToken(refreshRaw), session.RevocationReasonUserLogout); err != nil {
				logger.Logger.Errorf("[Auth] Failed to revoke refresh session on logout: %v", err)
				revokeErr = err
			}
		}
	}
	utils.ClearAllAuthCookies(c)
	if revokeErr != nil {
		server.RespondWithError(c, http.StatusInternalServerError, "sign_out_partial", "Local session cleared, but server-side revocation failed. Please retry.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Logged out successfully"})
}

// extractLogoutToken extracts the JWT from the request for revocation on logout.
func extractLogoutToken(c *gin.Context) string {
	if header := strings.TrimSpace(c.GetHeader("Authorization")); header != "" {
		parts := strings.Split(header, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			return parts[1]
		}
	}
	if cookie, err := c.Cookie("session_token"); err == nil && cookie != "" {
		return cookie
	}
	return ""
}

// GetCurrentUser returns the current authenticated user
func (h *AuthHandler) GetCurrentUser(c *gin.Context) {
	userIDInterface, exists := c.Get("user_id")
	if !exists {
		server.RespondWithError(c, http.StatusUnauthorized, server.ErrCodeNotAuthenticated, "Not authenticated")
		return
	}

	userID, ok := userIDInterface.(uint)
	if !ok {
		if userIDFloat, ok := userIDInterface.(float64); ok {
			userID = uint(userIDFloat)
		} else {
			server.RespondWithError(c, http.StatusInternalServerError, server.ErrCodeInternal, "Invalid user ID")
			return
		}
	}

	var user database.User
	if err := h.db.First(&user, userID).Error; err != nil {
		server.RespondWithError(c, http.StatusNotFound, server.ErrCodeUserNotFound, "User not found")
		return
	}

	walletAddress, _ := h.authService.GetUserWallet(userID)
	picture := user.Picture
	if pictureClaim, exists := c.Get("picture"); exists {
		if pictureValue, ok := pictureClaim.(string); ok && pictureValue != "" {
			picture = pictureValue
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":             user.ID,
		"email":          user.Email,
		"name":           user.Name,
		"role":           user.Role,
		"picture":        picture,
		"auth_method":    user.AuthMethod,
		"wallet_linked":  walletAddress != "",
		"wallet_address": walletAddress,
		"created_at":     user.CreatedAt,
	})
}

package server

import (
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/logic"
	"github.com/stdevmac/payverge/backend/internal/metrics"
	"github.com/stdevmac/payverge/backend/internal/session"
	"github.com/stdevmac/payverge/backend/internal/structs"
	"github.com/stdevmac/payverge/backend/internal/utils"

	"github.com/ethereum/go-ethereum/common"
	"github.com/gin-gonic/gin"
)

// Helper function for setting session cookies
func setSessionCookie(c *gin.Context, token string) {
	utils.SetSessionCookie(c, "session_token", token, 15*60) // 15 min — matches JWT TTL
}

var ChallengeStore *logic.ChallengeStore

func init() {
	ChallengeStore = logic.NewChallengeStore()
}

type SignatureRequest struct {
	Address   string `json:"address"`
	Signature string `json:"signature"`
}

type SignInRequest struct {
	Message    string `json:"message"`
	Signature  string `json:"signature"`
	InviteCode string `json:"invite_code"`
}

// GenerateChallenge generates a challenge for the user to sign
func GenerateChallenge(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("challenge_generated").Inc()
	var req struct {
		Address string `json:"address"`
	}
	if err := c.BindJSON(&req); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, err.Error())
		return
	}

	if !common.IsHexAddress(req.Address) {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid Ethereum address")
		return
	}

	// The challenge is a stateless nonce MAC-bound to the address and its
	// expiry: issuing one stores nothing, so requesting challenges (for any
	// address, from any number of clients) can neither evict a challenge
	// another client holds nor exhaust a shared pool.
	challenge, err := ChallengeStore.Issue(req.Address, WalletChallengeTTL)
	if err != nil {
		log.Printf("[Auth] wallet challenge not issued: %v", err)
		RespondWithError(c, http.StatusInternalServerError, "", "Failed to generate challenge")
		return
	}

	c.JSON(http.StatusOK, gin.H{"challenge": challenge.Value})
}

// GetSession returns the session token if it is valid
func GetSession(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("session_check").Inc()
	authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
	var sessionToken string

	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid Authorization header format")
			return
		}
		sessionToken = normalizeTokenString(parts[1])
	} else if cookieToken, err := c.Cookie("session_token"); err == nil && strings.TrimSpace(cookieToken) != "" {
		sessionToken = normalizeTokenString(cookieToken)
	} else {
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenMissing, "Missing Authorization header")
		return
	}

	if _, err := VerifyToken(sessionToken); err != nil {
		lowerErr := strings.ToLower(err.Error())
		// Keep the malformed/segments branch distinct for client-side
		// "bad cookie" telemetry, but do NOT echo the raw jwt-go error —
		// it leaks algorithm/claim internals to unauthenticated callers.
		if strings.Contains(lowerErr, "malformed") || strings.Contains(lowerErr, "invalid number of segments") {
			RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Malformed token")
			return
		}
		log.Printf("[Auth] GetSession token verify failed: %v", err)
		RespondWithError(c, http.StatusUnauthorized, ErrCodeTokenInvalid, "Invalid or expired token")
		return
	}

	// Return the original JWT token
	c.JSON(http.StatusOK, gin.H{
		"session_token": sessionToken,
	})
}

// SignIn signs the user in by verifying the signature
func SignIn(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("sign_in_attempt").Inc()
	var req SignInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid request")
		return
	}

	siwe, ok := ParseAndValidateSIWE(c, req.Message)
	if !ok {
		return
	}
	address, chainId := siwe.Address, siwe.ChainID

	// 1. The nonce must be one we issued for this address, unexpired and
	// unspent. This check is stateless.
	challenge, ok := ChallengeStore.Verify(siwe.Nonce, address)
	if !ok {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid or expired challenge")
		return
	}

	// 2. Verify the signature.
	if !utils.VerifySignature(address, req.Message, req.Signature, chainId) {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid signature")
		return
	}

	// 3. Spend the nonce. Redeem is one critical section, so of two
	// concurrent sign-ins replaying the same signed message only one wins.
	if err := ChallengeStore.TryRedeem(challenge); err != nil {
		RespondWithError(c, http.StatusBadRequest, ErrCodeInvalidInput, "Invalid or expired challenge")
		return
	}

	user, _, err := getOrCreateWalletUserWithInvite(c.Request.Context(), address, structs.Role("user"), req.InviteCode)
	if err != nil {
		if respondLaunchInviteAdmissionError(c, err) {
			return
		}
		metrics.AuthOperations.WithLabelValues("sign_in_failed").Inc()
		log.Printf("[Auth] wallet identity resolution failed: %v", err)
		RespondWithError(c, http.StatusInternalServerError, "", "Error registering user")
		return
	}
	var token string
	var refreshToken string
	if session.GlobalStore != nil {
		sess, sessErr := session.GlobalStore.Create(session.CreateInput{
			Address:   address,
			Provider:  "web3",
			IPAddress: c.ClientIP(),
			UserAgent: c.GetHeader("User-Agent"),
			ExpiresAt: time.Now().Add(24 * time.Hour),
		})
		if sessErr != nil {
			log.Printf("[Auth] Failed to create session: %v", sessErr)
			RespondWithError(c, http.StatusInternalServerError, "", "Error creating session")
			return
		}
		token, err = GenerateToken(address, user.Role, sess.ID)
		if err != nil {
			RespondWithError(c, http.StatusInternalServerError, "", "Error generating token")
			return
		}
		if err := session.GlobalStore.UpdateTokenHash(sess.ID, session.HashToken(token)); err != nil {
			log.Printf("[Auth] Failed to update session token hash (sid=%d): %v", sess.ID, err)
			RespondWithError(c, http.StatusInternalServerError, "", "Error persisting session")
			return
		}
		refreshToken, err = session.GenerateRefreshToken()
		if err != nil {
			log.Printf("[Auth] Failed to generate refresh token (sid=%d): %v", sess.ID, err)
			RespondWithError(c, http.StatusInternalServerError, "", "Error generating refresh token")
			return
		}
		if err := session.GlobalStore.RotateRefreshToken(sess.ID, session.HashToken(refreshToken), time.Now().Add(7*24*time.Hour)); err != nil {
			log.Printf("[Auth] Failed to persist refresh token (sid=%d): %v", sess.ID, err)
			RespondWithError(c, http.StatusInternalServerError, "", "Error persisting refresh token")
			return
		}
	} else {
		token, err = GenerateToken(address, user.Role)
		if err != nil {
			RespondWithError(c, http.StatusInternalServerError, "", "Error generating token")
			return
		}
	}

	metrics.AuthOperations.WithLabelValues("sign_in_success").Inc()
	// Evict leftover staff/customer cookies so Hybrid/session-info resolve
	// the just-authenticated owner instead of a shadowed staff principal.
	utils.ClearAllAuthCookies(c)
	if refreshToken != "" {
		utils.SetSessionCookie(c, "refresh_token", refreshToken, 7*24*3600)
	}
	setSessionCookie(c, token)
	c.JSON(http.StatusOK, gin.H{"success": true, "token": token, "address": address})
}

// SignOut signs the user out by invalidating the session token.
// If server-side revocation fails, we STILL clear the cookies (best-effort
// local sign-out) but return 500 so the client knows the token may still be
// replayable server-side and can prompt the user to retry. On a shared
// device, "signed out" must reflect reality.
func SignOut(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("sign_out").Inc()
	var revokeErr error
	if session.GlobalStore != nil {
		if tokenStr := extractTokenFromRequest(c, "session_token"); tokenStr != "" {
			if err := session.GlobalStore.RevokeByTokenHashWithReason(session.HashToken(tokenStr), session.RevocationReasonUserLogout); err != nil {
				log.Printf("[Auth] Failed to revoke session on sign out: %v", err)
				revokeErr = err
			}
		}
		if refreshRaw, err := c.Cookie("refresh_token"); err == nil && refreshRaw != "" {
			if err := session.GlobalStore.RevokeByRefreshTokenHashWithReason(session.HashToken(refreshRaw), session.RevocationReasonUserLogout); err != nil {
				log.Printf("[Auth] Failed to revoke refresh session on sign out: %v", err)
				revokeErr = err
			}
		}
	}
	// Clear client-side cookies regardless so the local browser is signed out.
	utils.ClearAllAuthCookies(c)
	if revokeErr != nil {
		RespondWithError(c, http.StatusInternalServerError, "sign_out_partial", "Local session cleared, but server-side revocation failed. Please retry.")
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Successfully signed out"})
}

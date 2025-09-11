package server

import (
	"encoding/hex"
	"errors"
	"log"
	"math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/gin-gonic/gin"
	"payverge/internal/database"
	"payverge/internal/logic"
	"payverge/internal/metrics"
	"payverge/internal/structs"
	"payverge/internal/utils"
)

var ChallengeStore *logic.ChallengeStore

func init() {
	ChallengeStore = logic.NewChallengeStore()
}

type SignatureRequest struct {
	Address   string `json:"address"`
	Signature string `json:"signature"`
}

type SignInRequest struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
}

// GenerateChallenge generates a challenge for the user to sign
func GenerateChallenge(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("challenge_generated").Inc()
	var req struct {
		Address string `json:"address"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	challenge := make([]byte, 32)
	_, err := rand.Read(challenge)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate challenge"})
		return
	}

	address := strings.ToLower(req.Address)

	// Store the challenge
	ChallengeStore.Set(address, logic.Challenge{
		Value:     hex.EncodeToString(challenge),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	})

	c.JSON(http.StatusOK, gin.H{"challenge": hex.EncodeToString(challenge)})

	// Track challenge generation
	properties := map[string]interface{}{
		"user_address": req.Address,
		"timestamp":    time.Now(),
	}
	err = metrics.TrackGeneralEvent("Auth Challenge Generated", properties)
	if err != nil {
		log.Printf("Failed to track challenge generation: %v", err)
	}
}

// GetSession returns the session token if it is valid
func GetSession(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("session_check").Inc()
	authHeader := c.GetHeader("Authorization")
	if authHeader == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
		return
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
		return
	}

	sessionToken := parts[1]
	// remove \"\" from the token
	sessionToken = sessionToken[1 : len(sessionToken)-1]
	token, err := jwt.Parse(sessionToken, func(token *jwt.Token) (interface{}, error) {
		return structs.SecretKey, nil
	})

	if err != nil {
		var validationError *jwt.ValidationError
		if errors.As(err, &validationError) {
			switch validationError.Errors {
			case jwt.ValidationErrorExpired:
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Token has expired"})
				return
			case jwt.ValidationErrorMalformed:
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Malformed token"})
				return
			default:
				c.JSON(http.StatusUnauthorized, gin.H{"error": "Token validation error"})
				return
			}
		}
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid session token"})
		return
	}
	if token.Valid {
		// Return the original JWT token
		c.JSON(http.StatusOK, gin.H{
			"session_token": sessionToken,
		})
	} else {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid session token"})
	}
}

// SignIn signs the user in by verifying the signature
func SignIn(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("sign_in_attempt").Inc()
	var req SignInRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	lines := strings.Split(req.Message, "\n")
	if len(lines) < 11 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid message"})
		return
	}
	address := strings.ToLower(strings.Replace(strings.Split(lines[5], ":")[1], " ", "", -1))
	challenge := strings.Replace(strings.Split(lines[6], ":")[1], " ", "", -1)
	chainId := strings.Split(lines[7], "#")[1]

	if !utils.VerifySignature(address, req.Message, req.Signature, chainId) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid signature"})
		return
	}
	storedChallenge, ok := ChallengeStore.Get(address)
	if !ok || storedChallenge.Value != challenge {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid or expired challenge"})
		return
	}

	if time.Now().After(storedChallenge.ExpiresAt) {
		ChallengeStore.Delete(address)
		c.JSON(http.StatusBadRequest, gin.H{"error": "Challenge expired"})
		return
	}

	user, err := database.GetUserByAddress(address)
	if err != nil {
		user = newUser(address)
		err = database.RegisterUser(user)
		if err != nil {
			metrics.AuthOperations.WithLabelValues("sign_in_failed").Inc()
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Error registering user"})
			return
		}

		// PostHog: Track new user registration event
		properties := map[string]interface{}{
			"email":     user.Email,
			"address":   user.Address,
			"timestamp": time.Now(),
		}
		err = metrics.TrackEvent(user.Address, "User Signed Up", properties)
		if err != nil {
			log.Printf("Error tracking user signup event: %v", err)
		}
	}

	token, err := GenerateToken(address, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Error generating token"})
		return
	}

	// Track user login event
	properties := map[string]interface{}{
		"email":     user.Email,
		"address":   user.Address,
		"timestamp": time.Now(),
	}
	err = metrics.TrackEvent(user.Address, "User Login", properties)
	if err != nil {
		log.Printf("Error tracking user login: %v", err)
	}

	metrics.AuthOperations.WithLabelValues("sign_in_success").Inc()
	ChallengeStore.Delete(address)
	c.SetCookie("session_token", token, 3600*24, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"success": true, "token": token, "address": address})

	// Track successful sign in
	properties = map[string]interface{}{
		"user_address": address,
		"timestamp":    time.Now(),
	}
	err = metrics.TrackGeneralEvent("User Sign In", properties)
	if err != nil {
		log.Printf("Failed to track user sign in: %v", err)
	}
}

// SignOut signs the user out by invalidating the session token
func SignOut(c *gin.Context) {
	metrics.AuthOperations.WithLabelValues("sign_out").Inc()
	// Invalidate the session token (remove cookie)
	c.SetCookie("session_token", "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"message": "Successfully signed out"})

	// Track sign out
	claims, ok := c.Get("claims")
	if !ok {
		log.Println("Failed to get claims from context")
		return
	}
	claimsMap, ok := claims.(map[string]interface{})
	if !ok {
		log.Println("Failed to convert claims to map")
		return
	}
	properties := map[string]interface{}{
		"user_address": claimsMap["address"],
		"timestamp":    time.Now(),
	}
	err := metrics.TrackGeneralEvent("User Sign Out", properties)
	if err != nil {
		log.Printf("Failed to track user sign out: %v", err)
	}
}

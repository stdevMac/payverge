package middleware

import (
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

type AuthMiddleware struct {
	jwtSecret []byte
}

type Claims struct {
	Address string `json:"address"`
	jwt.RegisteredClaims
}

type SignatureAuthRequest struct {
	Address   string `json:"address" binding:"required"`
	Message   string `json:"message" binding:"required"`
	Signature string `json:"signature" binding:"required"`
}

type AuthResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

func NewAuthMiddleware(jwtSecret string) *AuthMiddleware {
	return &AuthMiddleware{
		jwtSecret: []byte(jwtSecret),
	}
}

// GenerateAuthChallenge creates a challenge message for wallet signature
func (am *AuthMiddleware) GenerateAuthChallenge(address string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("Sign this message to authenticate with Invoice Generator.\n\nAddress: %s\nTimestamp: %d\n\nThis request will not trigger a blockchain transaction or cost any gas fees.", address, timestamp)
}

// VerifySignature verifies an Ethereum wallet signature
func (am *AuthMiddleware) VerifySignature(address, message, signature string) error {
	// Remove 0x prefix if present
	if strings.HasPrefix(signature, "0x") {
		signature = signature[2:]
	}

	// Decode signature
	sigBytes, err := hex.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("invalid signature format: %v", err)
	}

	if len(sigBytes) != 65 {
		return fmt.Errorf("signature must be 65 bytes long")
	}

	// Ethereum signatures have recovery ID as last byte (27 or 28, but we need 0 or 1)
	if sigBytes[64] >= 27 {
		sigBytes[64] -= 27
	}

	// Hash the message with Ethereum prefix
	messageHash := crypto.Keccak256Hash([]byte(fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(message), message)))

	// Recover public key
	pubKey, err := crypto.SigToPub(messageHash.Bytes(), sigBytes)
	if err != nil {
		return fmt.Errorf("failed to recover public key: %v", err)
	}

	// Get address from public key
	recoveredAddress := crypto.PubkeyToAddress(*pubKey)

	// Compare addresses (case-insensitive)
	if !strings.EqualFold(recoveredAddress.Hex(), address) {
		return fmt.Errorf("signature verification failed: recovered address %s does not match provided address %s", recoveredAddress.Hex(), address)
	}

	return nil
}

// GenerateJWT creates a JWT token for authenticated user
func (am *AuthMiddleware) GenerateJWT(address string) (*AuthResponse, error) {
	expirationTime := time.Now().Add(24 * time.Hour) // 24 hour expiration

	claims := &Claims{
		Address: strings.ToLower(address),
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "invoice-generator",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(am.jwtSecret)
	if err != nil {
		return nil, fmt.Errorf("failed to generate token: %v", err)
	}

	return &AuthResponse{
		Token:     tokenString,
		ExpiresAt: expirationTime.Unix(),
	}, nil
}

// ValidateJWT validates and parses a JWT token
func (am *AuthMiddleware) ValidateJWT(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return am.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("invalid token: %v", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is not valid")
	}

	return claims, nil
}

// AuthenticateHandler handles wallet signature authentication
func (am *AuthMiddleware) AuthenticateHandler(c *gin.Context) {
	var req SignatureAuthRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request format"})
		return
	}

	// Validate Ethereum address format
	if !common.IsHexAddress(req.Address) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Ethereum address"})
		return
	}

	// Verify signature
	if err := am.VerifySignature(req.Address, req.Message, req.Signature); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Signature verification failed"})
		return
	}

	// Generate JWT token
	authResponse, err := am.GenerateJWT(req.Address)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to generate authentication token"})
		return
	}

	c.JSON(http.StatusOK, authResponse)
}

// GetChallengeHandler returns a challenge message for signing
func (am *AuthMiddleware) GetChallengeHandler(c *gin.Context) {
	address := c.Query("address")
	if address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Address parameter required"})
		return
	}

	if !common.IsHexAddress(address) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid Ethereum address"})
		return
	}

	challenge := am.GenerateAuthChallenge(address)
	c.JSON(http.StatusOK, gin.H{
		"message": challenge,
		"address": address,
	})
}

// RequireAuth middleware that validates JWT tokens
func (am *AuthMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization header required"})
			c.Abort()
			return
		}

		tokenString := strings.TrimPrefix(authHeader, "Bearer ")
		claims, err := am.ValidateJWT(tokenString)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "Invalid or expired token"})
			c.Abort()
			return
		}

		// Store user address in context for use in handlers
		c.Set("userAddress", claims.Address)
		c.Next()
	}
}

// RequireOwnership middleware that ensures user owns the resource
func (am *AuthMiddleware) RequireOwnership() gin.HandlerFunc {
	return func(c *gin.Context) {
		userAddress, exists := c.Get("userAddress")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
			c.Abort()
			return
		}

		// Get creator address from query or request body
		creatorAddress := c.Query("creator")
		if creatorAddress == "" {
			// Try to get from request body for POST requests
			var body map[string]interface{}
			if err := c.ShouldBindJSON(&body); err == nil {
				if creator, ok := body["creator"].(string); ok {
					creatorAddress = creator
				}
			}
		}

		if creatorAddress == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Creator address required"})
			c.Abort()
			return
		}

		// Ensure user owns the resource
		if !strings.EqualFold(userAddress.(string), creatorAddress) {
			c.JSON(http.StatusForbidden, gin.H{"error": "Access denied: you can only access your own resources"})
			c.Abort()
			return
		}

		c.Next()
	}
}

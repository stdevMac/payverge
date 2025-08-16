package server

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"github.com/stdevMac/shares/internal/structs"
	"time"
)

// GenerateToken generates a JWT token with the user ID as part of the claims
func GenerateToken(address string, role structs.Role) (string, error) {
	// Create the claims
	claims := jwt.MapClaims{
		"address": address,
		"role":    role,
		"exp":     time.Now().Add(time.Hour * 24).Unix(),
	}

	// Create the token
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Sign the token
	return token.SignedString(structs.SecretKey)
}

// VerifyToken verifies a token JWT validate
func VerifyToken(tokenString string) (jwt.MapClaims, error) {
	// Parse the token
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return structs.SecretKey, nil
	})

	// Check if there was an error parsing the token
	if err != nil {
		return nil, err
	}

	// Check if the token is valid
	if !token.Valid {
		fmt.Println("Token is invalid")
		return nil, jwt.ErrSignatureInvalid
	}

	// Return the claims
	return token.Claims.(jwt.MapClaims), nil
}

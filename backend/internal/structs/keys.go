package structs

import "os"

var SecretKey = []byte(getSecretKey())

func getSecretKey() string {
	key := os.Getenv("JWT_SECRET_KEY")
	if key == "" {
		// Fallback for development only - should never be used in production
		return "aWjY5AgqQog7vCw4/DAtg7xzDISSCqCQXaWsz5MwfgI="
	}
	return key
}

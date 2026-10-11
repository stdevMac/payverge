package structs

import (
	"os"
	"strings"
	"sync"
)

var (
	SecretKey   []byte
	secretKeyMu sync.Mutex
)

func GetSecretKey() []byte {
	secretKeyMu.Lock()
	defer secretKeyMu.Unlock()

	if len(SecretKey) > 0 {
		return SecretKey
	}

	key := os.Getenv("JWT_SECRET_KEY")
	if strings.TrimSpace(key) == "" {
		panic("JWT_SECRET_KEY environment variable is required")
	}

	SecretKey = []byte(key)
	return SecretKey
}

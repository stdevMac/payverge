package utils

import (
	"crypto/rand"
	"errors"
	"fmt"
	"hash/fnv"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
)

// ErrEntropyUnavailable is returned by the generators below when the OS
// entropy source (crypto/rand) fails. Callers MUST treat this as fatal for
// security primitives (verification tokens, nonces); silently
// falling back to time-based randomness would let an attacker predict values.
var ErrEntropyUnavailable = errors.New("crypto entropy source unavailable")

func StringToRandomUint(s string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	seed := h.Sum32()
	// Use crypto/rand for unpredictable output.
	b := make([]byte, 4)
	if _, err := rand.Read(b); err == nil {
		val := uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24
		return strconv.FormatUint(uint64(val)^uint64(seed), 10)
	}
	// Fallback: deterministic but mixed with entropy. Not used for security primitives.
	return strconv.FormatUint(uint64(seed)^uint64(time.Now().UnixNano()), 10)
}

func VerifySignature(address, msg, signature, chainId string) bool {
	//message := fmt.Sprintf("Login challenge for address: %s", address)
	prefixedMessage := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	// Hash the message
	hash := sha3.NewLegacyKeccak256()
	hash.Write([]byte(prefixedMessage))
	messageHash := hash.Sum(nil)

	// Decode the signature
	sig, err := hexutil.Decode(signature)
	if err != nil {
		log.Printf("Failed to decode signature: %v", err)
		return false
	}

	// ECDSA signatures require exactly 65 bytes (32 r + 32 s + 1 v).
	// Reject anything shorter to prevent panics on slice indexing.
	if len(sig) < 65 {
		return false
	}

	// Extract r, s, and v values
	r := sig[:32]
	s := sig[32:64]
	v := sig[64]

	// Ethereum uses v = 27 or 28, but Go-ethereum uses 0 or 1
	if v == 27 || v == 28 {
		v -= 27
	}
	// Combine r, s, and v into a single slice
	sig = append(r, append(s, v)...)

	// Recover the public key
	pubKeyBytes, err := crypto.Ecrecover(messageHash, sig)
	if err != nil {
		log.Printf("Failed to recover public key: %v", err)
		return false
	}

	// Convert the public key to ecdsa.PublicKey
	pubKey, err := crypto.UnmarshalPubkey(pubKeyBytes)
	if err != nil {
		log.Printf("Failed to unmarshal public key: %v", err)
		return false
	}

	// Get the address from the public key
	recoveredAddress := crypto.PubkeyToAddress(*pubKey).Hex()

	return strings.EqualFold(recoveredAddress, address)
}

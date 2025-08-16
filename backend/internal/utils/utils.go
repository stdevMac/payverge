package utils

import (
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"golang.org/x/crypto/sha3"
	"hash/fnv"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"
)

const (
	referralCodeLength      = 8
	codeChars               = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	emailVerificationLength = 64
)

func GenerateReferralCode() string {
	rand.Seed(time.Now().UnixNano())
	var sb strings.Builder
	sb.Grow(referralCodeLength)

	for i := 0; i < referralCodeLength; i++ {
		sb.WriteByte(codeChars[rand.Intn(len(codeChars))])
	}
	referralCode := sb.String()
	return strings.ToLower(referralCode)
}

func GenerateEmailVerificationToken() string {
	rand.Seed(time.Now().UnixNano())
	var sb strings.Builder
	sb.Grow(emailVerificationLength)

	for i := 0; i < emailVerificationLength; i++ {
		sb.WriteByte(codeChars[rand.Intn(len(codeChars))])
	}

	return sb.String()
}

func StringToRandomUint(s string) string {
	h := fnv.New32a()
	h.Write([]byte(s))
	seed := h.Sum32()
	rand.Seed(int64(seed) + time.Now().UnixNano())
	return strconv.FormatUint(uint64(rand.Uint32()), 10)
}

func FormatFleetId(fleetId int) string {
	return fmt.Sprintf("%04d", fleetId)
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

func GenerateNonce() string {
	rand.Seed(time.Now().UnixNano())
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	nonce := make([]byte, 16)
	for i := range nonce {
		nonce[i] = charset[rand.Intn(len(charset))]
	}
	return string(nonce)
}

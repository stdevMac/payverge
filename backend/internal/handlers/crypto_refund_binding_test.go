package handlers

import (
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func signPersonalMessage(t *testing.T, key *ecdsa.PrivateKey, msg string) string {
	t.Helper()
	prefixed := fmt.Sprintf("\x19Ethereum Signed Message:\n%d%s", len(msg), msg)
	hash := crypto.Keccak256Hash([]byte(prefixed))
	sig, err := crypto.Sign(hash.Bytes(), key)
	require.NoError(t, err)
	// crypto.Sign returns v=0/1; Ethereum personal_sign uses 27/28.
	if sig[64] < 27 {
		sig[64] += 27
	}
	return hexutil.Encode(sig)
}

func testBindingKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	// Deterministic test key (NOT a real secret — unit tests only).
	key, err := crypto.ToECDSA(hexutil.MustDecode("0xac0974bec39a17e36ba4a6b4d238ff944bacb478cbed5efcae784d7bf4f2ff80"))
	require.NoError(t, err)
	addr := crypto.PubkeyToAddress(key.PublicKey).Hex()
	return key, addr
}

func TestVerifyRefundDestinationBinding_AcceptsValidSignature(t *testing.T) {
	key, addr := testBindingKey(t)
	now := time.Unix(1_700_000_000, 0)
	binding := RefundDestinationBinding{
		Domain:          refundBindingDomain(),
		BillID:          42,
		BusinessID:      7,
		SourceTx:        "0xabc123",
		AmountBaseUnits: 5_000_000,
		ChainID:         84532,
		Destination:     "0x1111111111111111111111111111111111111111",
		RefundAddress:   addr,
		Exp:             now.Add(10 * time.Minute).Unix(),
	}
	sig := signPersonalMessage(t, key, FormatRefundDestinationBindingMessage(binding))

	err := VerifyRefundDestinationBinding(
		binding, sig,
		42, 7, "0xABC123", 5_000_000, 84532,
		"0x1111111111111111111111111111111111111111",
		now,
	)
	require.NoError(t, err)
}

func TestVerifyRefundDestinationBinding_RejectsWrongSigner(t *testing.T) {
	key, _ := testBindingKey(t)
	other := "0x2222222222222222222222222222222222222222"
	now := time.Unix(1_700_000_000, 0)
	binding := RefundDestinationBinding{
		Domain:          refundBindingDomain(),
		BillID:          42,
		BusinessID:      7,
		SourceTx:        "0xabc",
		AmountBaseUnits: 5_000_000,
		ChainID:         84532,
		Destination:     "0x1111111111111111111111111111111111111111",
		RefundAddress:   other, // claims other, signed by key
		Exp:             now.Add(10 * time.Minute).Unix(),
	}
	sig := signPersonalMessage(t, key, FormatRefundDestinationBindingMessage(binding))
	err := VerifyRefundDestinationBinding(
		binding, sig,
		42, 7, "0xabc", 5_000_000, 84532,
		"0x1111111111111111111111111111111111111111",
		now,
	)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "signature")
}

func TestVerifyRefundDestinationBinding_RejectsWrongBillAmountChain(t *testing.T) {
	key, addr := testBindingKey(t)
	now := time.Unix(1_700_000_000, 0)
	binding := RefundDestinationBinding{
		Domain:          refundBindingDomain(),
		BillID:          42,
		BusinessID:      7,
		SourceTx:        "0xabc",
		AmountBaseUnits: 5_000_000,
		ChainID:         84532,
		Destination:     "0x1111111111111111111111111111111111111111",
		RefundAddress:   addr,
		Exp:             now.Add(10 * time.Minute).Unix(),
	}
	sig := signPersonalMessage(t, key, FormatRefundDestinationBindingMessage(binding))

	require.Error(t, VerifyRefundDestinationBinding(binding, sig, 99, 7, "0xabc", 5_000_000, 84532, binding.Destination, now))
	require.Error(t, VerifyRefundDestinationBinding(binding, sig, 42, 7, "0xabc", 9_000_000, 84532, binding.Destination, now))
	require.Error(t, VerifyRefundDestinationBinding(binding, sig, 42, 7, "0xabc", 5_000_000, 1, binding.Destination, now))
}

func TestVerifyRefundDestinationBinding_RejectsExpiredAndReplayWindow(t *testing.T) {
	key, addr := testBindingKey(t)
	now := time.Unix(1_700_000_000, 0)
	binding := RefundDestinationBinding{
		Domain:          refundBindingDomain(),
		BillID:          1,
		BusinessID:      1,
		SourceTx:        "0x1",
		AmountBaseUnits: 1,
		ChainID:         84532,
		Destination:     "0x1111111111111111111111111111111111111111",
		RefundAddress:   addr,
		Exp:             now.Add(-time.Second).Unix(),
	}
	sig := signPersonalMessage(t, key, FormatRefundDestinationBindingMessage(binding))
	require.Error(t, VerifyRefundDestinationBinding(binding, sig, 1, 1, "0x1", 1, 84532, binding.Destination, now))

	// Far-future expiry rejected.
	binding.Exp = now.Add(24 * time.Hour).Unix()
	sig = signPersonalMessage(t, key, FormatRefundDestinationBindingMessage(binding))
	require.Error(t, VerifyRefundDestinationBinding(binding, sig, 1, 1, "0x1", 1, 84532, binding.Destination, now))
}

func TestVerifyRefundDestinationBinding_RejectsMalformedAddress(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	binding := RefundDestinationBinding{
		Domain:          refundBindingDomain(),
		BillID:          1,
		BusinessID:      1,
		SourceTx:        "0x1",
		AmountBaseUnits: 1,
		ChainID:         84532,
		Destination:     "0x1111111111111111111111111111111111111111",
		RefundAddress:   "not-an-address",
		Exp:             now.Add(5 * time.Minute).Unix(),
	}
	err := VerifyRefundDestinationBinding(binding, "0x00", 1, 1, "0x1", 1, 84532, binding.Destination, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "malformed")
}

// Ensure big.Int import is used if needed by future vectors (suppress unused).
var _ = big.NewInt

func TestRefundBindingDomainFollowsInstanceHost(t *testing.T) {
	for _, k := range []string{"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Setenv(k, "")
	}
	t.Setenv("PUBLIC_URL", "https://payverge.io")
	require.Equal(t, "payverge.io/crypto-refund-destination/v1", refundBindingDomain(),
		"hosted upstream keeps its historical binding domain")
	t.Setenv("PUBLIC_URL", "https://POS.Example.com/")
	require.Equal(t, "pos.example.com/crypto-refund-destination/v1", refundBindingDomain())
}

func TestVerifyRefundDestinationBinding_RejectsOtherInstanceDomain(t *testing.T) {
	for _, k := range []string{"PUBLIC_URL", "FRONTEND_URL", "BASE_URL", "NEXT_PUBLIC_BASE_URL", "APP_BASE_URL"} {
		t.Setenv(k, "")
	}
	key, addr := testBindingKey(t)
	now := time.Unix(1_700_000_000, 0)
	t.Setenv("PUBLIC_URL", "https://other.example.net")
	binding := RefundDestinationBinding{
		Domain:          refundBindingDomain(),
		BillID:          42,
		BusinessID:      7,
		SourceTx:        "0xabc",
		AmountBaseUnits: 5_000_000,
		ChainID:         84532,
		Destination:     "0x1111111111111111111111111111111111111111",
		RefundAddress:   addr,
		Exp:             now.Add(10 * time.Minute).Unix(),
	}
	sig := signPersonalMessage(t, key, FormatRefundDestinationBindingMessage(binding))

	// The same signature presented to a different instance must not verify.
	t.Setenv("PUBLIC_URL", "https://pos.example.com")
	err := VerifyRefundDestinationBinding(binding, sig, 42, 7, "0xabc", 5_000_000, 84532, binding.Destination, now)
	require.Error(t, err)
	require.Contains(t, err.Error(), "domain mismatch")
}

package blockchain

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// TestVerifyUSDCTransfer_RejectsNonCanonicalTxHashSpellings pins that the
// verifier only accepts a well-formed 32-byte hash. common.HexToHash silently
// accepts upper/lower case, a missing 0x, zero-padding and trailing garbage,
// which lets one receipt be claimed under many distinct ledger keys.
func TestVerifyUSDCTransfer_RejectsNonCanonicalTxHashSpellings(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	body := "5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"

	for name, hash := range map[string]string{
		"no 0x prefix":     body,
		"zero padded":      "0x00" + body,
		"trailing garbage": "0x" + body + "zz",
		"short":            "0x" + body[:62],
		"non hex":          "0x" + strings.Repeat("g", 64),
	} {
		fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102}
		svc := newServiceWithFake(fake, 3)
		if _, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), hash, recipient.Hex(), 5_000_000, true); err == nil {
			t.Errorf("%s: non-canonical tx hash %q must be rejected", name, hash)
		}
	}
}

// TestVerifyUSDCTransfer_EvidenceCarriesCanonicalLowercaseHash pins that a
// mixed-case 0x hash is accepted but reported back in canonical lowercase so
// every downstream ledger key is one spelling per transfer.
func TestVerifyUSDCTransfer_EvidenceCarriesCanonicalLowercaseHash(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	body := "5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102}
	svc := newServiceWithFake(fake, 3)

	ev, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), "0X"+strings.ToUpper(body), recipient.Hex(), 5_000_000, true)
	if err != nil {
		t.Fatalf("mixed-case hash should verify: %v", err)
	}
	if ev.TxHash != "0x"+body {
		t.Fatalf("evidence tx hash = %q, want canonical %q", ev.TxHash, "0x"+body)
	}
}

// TestVerifyUSDCTransfer_EvidenceCarriesBlockTimestamp pins that the verifier
// reports the mined block's time so callers can reject transfers that predate
// the quote they are being claimed against.
func TestVerifyUSDCTransfer_EvidenceCarriesBlockTimestamp(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102, blockTime: 1_790_000_000}
	svc := newServiceWithFake(fake, 3)
	ev, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, true)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if ev.BlockTimestamp != 1_790_000_000 {
		t.Fatalf("BlockTimestamp = %d, want 1790000000", ev.BlockTimestamp)
	}
}

// TestVerifyUSDCTransfer_HeaderFailureIsRetryable pins fail-closed behavior:
// without a block time the transfer cannot be bound to the quote window, so
// verification is unavailable (retryable), never accepted.
func TestVerifyUSDCTransfer_HeaderFailureIsRetryable(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102, headerErr: errors.New("rpc down")}
	svc := newServiceWithFake(fake, 3)
	_, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, true)
	if !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("want ErrVerificationUnavailable, got %v", err)
	}
}

package handlers

// Wave 4 Task 11: cross-chain guest wallet signature binding a refund
// destination before settlement. The bridge contract sender is NOT a safe
// refund destination — guests must explicitly sign the refund address.

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/utils"
)

const (
	// refundBindingDomainPath is the versioned protocol path appended to the
	// instance host to form the signed binding domain.
	refundBindingDomainPath = "/crypto-refund-destination/v1"
	// maxRefundBindingTTL caps how long a signed binding is accepted.
	maxRefundBindingTTL = 30 * time.Minute
)

// refundBindingDomain is the domain string included in the signed message so
// signatures cannot be replayed across products or across instances: it is
// the instance's public host (PUBLIC_URL) plus a versioned protocol path,
// e.g. "pos.example.com/crypto-refund-destination/v1".
func refundBindingDomain() string {
	return config.PublicHost() + refundBindingDomainPath
}

// RefundDestinationBinding is the structured payload a guest wallet must sign
// (EIP-191 personal_sign) before cross-chain settlement.
type RefundDestinationBinding struct {
	Domain          string
	BillID          uint
	BusinessID      uint
	SourceTx        string
	AmountBaseUnits int64
	ChainID         int64
	Destination     string // settlement recipient (business SettlementAddr)
	RefundAddress   string // guest wallet that will receive refunds
	Exp             int64  // unix seconds
}

// FormatRefundDestinationBindingMessage produces the exact personal_sign message.
func FormatRefundDestinationBindingMessage(b RefundDestinationBinding) string {
	return strings.Join([]string{
		"Payverge refund destination binding",
		"domain:" + b.Domain,
		"bill_id:" + strconv.FormatUint(uint64(b.BillID), 10),
		"business_id:" + strconv.FormatUint(uint64(b.BusinessID), 10),
		"source_tx:" + strings.ToLower(strings.TrimSpace(b.SourceTx)),
		"amount_base_units:" + strconv.FormatInt(b.AmountBaseUnits, 10),
		"chain_id:" + strconv.FormatInt(b.ChainID, 10),
		"destination:" + strings.ToLower(strings.TrimSpace(b.Destination)),
		"refund_address:" + strings.ToLower(strings.TrimSpace(b.RefundAddress)),
		"exp:" + strconv.FormatInt(b.Exp, 10),
	}, "\n")
}

// VerifyRefundDestinationBinding checks signature, domain, expiry, and field
// bindings. signer must equal RefundAddress. chainId is passed to the SIWE-
// style verifier for multi-chain wallets (may be empty).
func VerifyRefundDestinationBinding(
	binding RefundDestinationBinding,
	signature string,
	expectedBillID, expectedBusinessID uint,
	expectedSourceTx string,
	expectedAmount int64,
	expectedChainID int64,
	expectedDestination string,
	now time.Time,
) error {
	expectedDomain := refundBindingDomain()
	if strings.TrimSpace(binding.Domain) == "" {
		binding.Domain = expectedDomain
	}
	if binding.Domain != expectedDomain {
		return fmt.Errorf("refund binding domain mismatch")
	}
	if binding.BillID != expectedBillID {
		return fmt.Errorf("refund binding bill mismatch")
	}
	if binding.BusinessID != expectedBusinessID {
		return fmt.Errorf("refund binding business mismatch")
	}
	if !strings.EqualFold(strings.TrimSpace(binding.SourceTx), strings.TrimSpace(expectedSourceTx)) {
		return fmt.Errorf("refund binding source tx mismatch")
	}
	if binding.AmountBaseUnits != expectedAmount {
		return fmt.Errorf("refund binding amount mismatch")
	}
	if binding.ChainID != expectedChainID {
		return fmt.Errorf("refund binding chain mismatch")
	}
	if !strings.EqualFold(strings.TrimSpace(binding.Destination), strings.TrimSpace(expectedDestination)) {
		return fmt.Errorf("refund binding destination mismatch")
	}
	refundAddr := strings.TrimSpace(binding.RefundAddress)
	if len(refundAddr) != 42 || !strings.HasPrefix(strings.ToLower(refundAddr), "0x") {
		return fmt.Errorf("malformed refund address in binding")
	}
	// Bridge contract senders are never accepted as the refund destination via
	// this path without an explicit guest signature over that address — the
	// signature itself binds refund_address.
	if binding.Exp == 0 || now.Unix() >= binding.Exp {
		return fmt.Errorf("refund binding expired")
	}
	// Reject absurdly long-lived bindings (anti-replay window).
	if binding.Exp-now.Unix() > int64(maxRefundBindingTTL.Seconds())+60 {
		return fmt.Errorf("refund binding expiry too far in the future")
	}

	msg := FormatRefundDestinationBindingMessage(binding)
	chainIDStr := strconv.FormatInt(binding.ChainID, 10)
	if !utils.VerifySignature(refundAddr, msg, strings.TrimSpace(signature), chainIDStr) {
		return fmt.Errorf("refund binding signature invalid or wrong signer")
	}
	return nil
}

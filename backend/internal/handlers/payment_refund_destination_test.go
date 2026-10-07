package handlers

import (
	"testing"

	"github.com/stdevmac/payverge/backend/internal/blockchain"
	"github.com/stdevmac/payverge/backend/internal/database"

	"github.com/stretchr/testify/require"
)

func TestRefundDestinationFromTransferEvidenceBuildsDurableProof(t *testing.T) {
	dest := refundDestinationFromTransferEvidence(blockchain.USDCTransferEvidence{
		From: "0xdead00000000000000000000000000000000beef", AmountBaseUnits: 5_000_000,
		ChainID: 84532, Token: "USDC", TxHash: "0xpayment", LogIndex: 2,
	})
	require.NotNil(t, dest)
	require.Zero(t, dest.PaymentID, "the settlement transaction assigns payment_id")
	require.Equal(t, database.RefundEvidenceTransferLog, dest.EvidenceType)
	require.Equal(t, "0xpayment:2", *dest.LogRef)
}

func TestRefundDestinationFromTransferEvidenceKeepsVerifyOnlyFallback(t *testing.T) {
	require.Nil(t, refundDestinationFromTransferEvidence(blockchain.USDCTransferEvidence{}))
}

func TestRefundDestinationFromWalletSignatureBuildsDurableProof(t *testing.T) {
	dest := refundDestinationFromWalletSignature(
		8453,
		5_000_000,
		"0xdead00000000000000000000000000000000beef",
		"0xsigned-binding",
		"0xpayment",
	)
	require.NotNil(t, dest)
	require.Equal(t, database.RefundEvidenceWalletSignature, dest.EvidenceType)
	require.Equal(t, "0xsigned-binding", *dest.SignatureRef)
	require.Equal(t, "0xpayment", *dest.LogRef)
}

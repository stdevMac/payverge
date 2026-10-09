package blockchain

import (
	"context"
	"errors"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// Guest settlement binds a transfer to its quote by exact amount.
// A transfer that pays the right wallet the wrong amount must be a distinct,
// non-retryable error so the handler can tell the guest exactly why, and an
// over-payment must not satisfy an exact check.

func TestVerifyUSDCTransfer_WrongAmountToRecipientIsAmountMismatch(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 200}
	svc := newServiceWithFake(fake, 3)

	for _, expected := range []int64{5_000_001, 4_999_999} {
		err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), expected)
		if !errors.Is(err, ErrTransferAmountMismatch) {
			t.Fatalf("expected %d: want ErrTransferAmountMismatch, got %v", expected, err)
		}
		if errors.Is(err, ErrAwaitingConfirmations) || errors.Is(err, ErrVerificationUnavailable) {
			t.Fatalf("amount mismatch must not be retryable, got %v", err)
		}
	}
	if fake.headCalls != 0 {
		t.Errorf("no head lookup is needed to reject an amount mismatch; got %d calls", fake.headCalls)
	}
}

func TestVerifyUSDCTransfer_ExactCheckRejectsOverpayment(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_004_321, 100), head: 200}
	svc := newServiceWithFake(fake, 3)

	if _, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, false); !errors.Is(err, ErrTransferAmountMismatch) {
		t.Fatalf("exact verification must reject an over-payment, got %v", err)
	}
	ev, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_004_321, false)
	if err != nil {
		t.Fatalf("exact amount must verify, got %v", err)
	}
	if ev.AmountBaseUnits != 5_004_321 {
		t.Fatalf("evidence amount = %d, want 5004321", ev.AmountBaseUnits)
	}
}

func TestVerifyUSDCTransfer_TransferToOtherWalletIsNotAmountMismatch(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	other := common.HexToAddress("0x2222222222222222222222222222222222222222")
	fake := &fakeRPC{receipt: transferReceipt(other, 5_000_000, 100), head: 200}
	svc := newServiceWithFake(fake, 3)

	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if err == nil || errors.Is(err, ErrTransferAmountMismatch) {
		t.Fatalf("a transfer to another wallet is a plain non-match, got %v", err)
	}
}

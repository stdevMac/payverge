package blockchain

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
	"testing"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

// fakeRPC is a stand-in for *ethclient.Client implementing the minimal rpcClient
// seam so the confirmation/finality logic is testable without a live node.
type fakeRPC struct {
	receipt    *types.Receipt
	receiptErr error
	head       uint64
	headErr    error
	headCalls  int
	blockTime  uint64
	headerErr  error
	// canonicalHash overrides the hash the node reports at the receipt's
	// height; zero means "same block as the receipt".
	canonicalHash  common.Hash
	canonicalErr   error
	canonicalCalls int
}

func (f *fakeRPC) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return f.receipt, f.receiptErr
}

func (f *fakeRPC) BlockNumber(ctx context.Context) (uint64, error) {
	f.headCalls++
	return f.head, f.headErr
}

func (f *fakeRPC) HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error) {
	if f.headerErr != nil {
		return nil, f.headerErr
	}
	return &types.Header{Time: f.blockTime}, nil
}

func (f *fakeRPC) CanonicalBlockHash(ctx context.Context, number *big.Int) (common.Hash, error) {
	f.canonicalCalls++
	if f.canonicalErr != nil {
		return common.Hash{}, f.canonicalErr
	}
	if f.canonicalHash != (common.Hash{}) {
		return f.canonicalHash, nil
	}
	return common.BigToHash(number), nil
}

func (f *fakeRPC) Close() {}

// testTxHash is a well-formed canonical tx hash; the fake node ignores it.
const testTxHash = "0x5c504ed432cb51138bcf09aa5e8a410dd4a1e204ef84bfed1be16dfba1b22060"

// baseUSDC is the Base-mainnet USDC address the verifier expects for chain 8453.
var baseUSDC = common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913")

// transferReceipt builds a successful receipt carrying one USDC Transfer log of
// `amount` micro-units to `recipient`, mined at block `minedAt`.
func transferReceipt(recipient common.Address, amount int64, minedAt uint64) *types.Receipt {
	logEntry := &types.Log{
		Address: baseUSDC,
		Topics: []common.Hash{
			erc20TransferTopic,
			common.HexToHash("0x000000000000000000000000dead00000000000000000000000000000000beef"),
			common.BytesToHash(recipient.Bytes()),
		},
		Data: big.NewInt(amount).Bytes(),
	}
	return &types.Receipt{
		Status:      types.ReceiptStatusSuccessful,
		Logs:        []*types.Log{logEntry},
		BlockNumber: new(big.Int).SetUint64(minedAt),
		BlockHash:   common.BigToHash(new(big.Int).SetUint64(minedAt)),
	}
}

func newServiceWithFake(fake *fakeRPC, minConf uint64) *BlockchainService {
	return &BlockchainService{client: fake, chainID: big.NewInt(8453), minConfirmations: minConf}
}

func TestConfirmationDepth(t *testing.T) {
	cases := []struct {
		head  uint64
		block *big.Int
		want  uint64
	}{
		{head: 10, block: big.NewInt(10), want: 1}, // mined block counts as 1 confirmation
		{head: 12, block: big.NewInt(10), want: 3}, // head - block + 1
		{head: 9, block: big.NewInt(10), want: 0},  // head behind the tx (lag/reorg)
		{head: 100, block: nil, want: 0},           // unknown block number
	}
	for _, tc := range cases {
		if got := confirmationDepth(tc.head, tc.block); got != tc.want {
			t.Errorf("confirmationDepth(%d, %v) = %d, want %d", tc.head, tc.block, got, tc.want)
		}
	}
}

func TestVerifyUSDCTransfer_AcceptsWhenSufficientlyConfirmed(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102} // depth 3
	svc := newServiceWithFake(fake, 3)
	if err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000); err != nil {
		t.Fatalf("expected accept at 3 confirmations, got %v", err)
	}
}

func TestVerifyUSDCTransfer_AwaitsWhenTooShallow(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 100} // depth 1 < 3
	svc := newServiceWithFake(fake, 3)
	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if !errors.Is(err, ErrAwaitingConfirmations) {
		t.Fatalf("expected ErrAwaitingConfirmations for a shallow tx, got %v", err)
	}
}

func TestVerifyUSDCTransfer_FailedTxIsHardError(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	rcpt := transferReceipt(recipient, 5_000_000, 100)
	rcpt.Status = types.ReceiptStatusFailed
	fake := &fakeRPC{receipt: rcpt, head: 200}
	svc := newServiceWithFake(fake, 3)
	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if err == nil || errors.Is(err, ErrAwaitingConfirmations) {
		t.Fatalf("a failed tx must be a hard error, got %v", err)
	}
}

func TestVerifyUSDCTransfer_NoMatchingTransferIsHardError(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	// Transfer is to a different address; no head lookup should be needed.
	other := common.HexToAddress("0x2222222222222222222222222222222222222222")
	fake := &fakeRPC{receipt: transferReceipt(other, 5_000_000, 100), head: 200}
	svc := newServiceWithFake(fake, 3)
	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if err == nil || errors.Is(err, ErrAwaitingConfirmations) {
		t.Fatalf("a non-matching tx must be a hard error, got %v", err)
	}
	if fake.headCalls != 0 {
		t.Errorf("should not fetch chain head when no transfer matches; got %d calls", fake.headCalls)
	}
}

func TestVerifyUSDCTransfer_DisabledConfirmationsSkipsHeadLookup(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	// minConfirmations == 0 means accept-on-inclusion (no head RPC).
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 100}
	svc := newServiceWithFake(fake, 0)
	if err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000); err != nil {
		t.Fatalf("expected accept with confirmations disabled (min=0), got %v", err)
	}
	if fake.headCalls != 0 {
		t.Errorf("minConfirmations=0 must not fetch the chain head; got %d calls", fake.headCalls)
	}
}

func TestVerifyUSDCTransfer_MinOneStillChecksHead(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	// min=1 must load head and require confs >= 1 (inclusion on tip).
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 100}
	svc := newServiceWithFake(fake, 1)
	if err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000); err != nil {
		t.Fatalf("expected accept with min=1 when confs>=1, got %v", err)
	}
	if fake.headCalls == 0 {
		t.Errorf("minConfirmations=1 must fetch the chain head; got 0 calls")
	}
}

// Wave 4 Task 11: refund destinations must come from the verified Transfer
// log sender (Topics[1]), never from guest-supplied fields or ledger sentinels.
func TestVerifyUSDCTransferWithEvidence_ExtractsPayerFromTransferLog(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	payer := common.HexToAddress("0xdead00000000000000000000000000000000beef")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102}
	svc := newServiceWithFake(fake, 3)

	ev, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, false)
	if err != nil {
		t.Fatalf("expected evidence, got %v", err)
	}
	if !stringsEqualFold(ev.From, payer.Hex()) {
		t.Fatalf("from = %q, want payer %q", ev.From, payer.Hex())
	}
	if !stringsEqualFold(ev.To, recipient.Hex()) {
		t.Fatalf("to = %q, want recipient %q", ev.To, recipient.Hex())
	}
	if ev.AmountBaseUnits != 5_000_000 {
		t.Fatalf("amount = %d, want 5000000 micro-units", ev.AmountBaseUnits)
	}
	if ev.Token != "USDC" {
		t.Fatalf("token = %q, want USDC", ev.Token)
	}
	if ev.ChainID != 8453 {
		t.Fatalf("chain = %d, want 8453", ev.ChainID)
	}
}

func TestVerifyOutboundUSDCTransfer_RejectsWrongFrom(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	settlement := common.HexToAddress("0x2222222222222222222222222222222222222222")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102}
	svc := newServiceWithFake(fake, 3)

	// transferReceipt uses Topics[1]=0xdead…beef, not settlement — must reject.
	_, err := svc.VerifyOutboundUSDCTransfer(context.Background(), testTxHash, settlement.Hex(), recipient.Hex(), 5_000_000)
	if err == nil {
		t.Fatal("expected reject when Transfer from != settlement wallet")
	}
}

func TestVerifyOutboundUSDCTransfer_AcceptsExactOutbound(t *testing.T) {
	from := common.HexToAddress("0xdead00000000000000000000000000000000beef")
	to := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(to, 5_000_000, 100), head: 102}
	svc := newServiceWithFake(fake, 3)

	ev, err := svc.VerifyOutboundUSDCTransfer(context.Background(), testTxHash, from.Hex(), to.Hex(), 5_000_000)
	if err != nil {
		t.Fatalf("expected accept, got %v", err)
	}
	if !stringsEqualFold(ev.From, from.Hex()) {
		t.Fatalf("from = %q, want %q", ev.From, from.Hex())
	}
	if ev.AmountBaseUnits != 5_000_000 {
		t.Fatalf("amount = %d", ev.AmountBaseUnits)
	}
}

func stringsEqualFold(a, b string) bool {
	return strings.EqualFold(a, b)
}

// TestVerifyUSDCTransfer_EvidenceCarriesBlock pins that settlement evidence now
// records the block the transfer landed in — the durable input a reorg sweep
// re-reads. Without block_hash persisted, an orphaned confirmed payment can
// never be detected.
func TestVerifyUSDCTransfer_EvidenceCarriesBlock(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102}
	svc := newServiceWithFake(fake, 3)

	ev, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, false)
	if err != nil {
		t.Fatalf("expected accept, got %v", err)
	}
	if ev.BlockNumber != 100 {
		t.Fatalf("block number = %d, want 100", ev.BlockNumber)
	}
	wantHash := common.BigToHash(new(big.Int).SetUint64(100)).Hex()
	if ev.BlockHash != wantHash {
		t.Fatalf("block hash = %q, want %q", ev.BlockHash, wantHash)
	}
}

func TestVerifyUSDCTransfer_ReceiptNotFoundIsAwaiting(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receiptErr: ethereum.NotFound}
	svc := newServiceWithFake(fake, 1)

	// VerifyUSDCTransfer returns only error (not evidence).
	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if !errors.Is(err, ErrAwaitingConfirmations) {
		t.Fatalf("not-found receipt must map to ErrAwaitingConfirmations, got %v", err)
	}
}

func TestVerifyUSDCTransfer_RPCDownIsUnavailableNotAwaiting(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receiptErr: fmt.Errorf("dial tcp: connection refused")}
	svc := newServiceWithFake(fake, 1)

	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("RPC failure must map to ErrVerificationUnavailable, got %v", err)
	}
	if errors.Is(err, ErrAwaitingConfirmations) {
		t.Fatalf("RPC failure must NOT read as awaiting confirmations")
	}
}

func TestVerifyUSDCTransfer_HeadFailureIsUnavailable(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{
		receipt: transferReceipt(recipient, 5_000_000, 100),
		headErr: fmt.Errorf("dial tcp: connection refused"),
	}
	svc := newServiceWithFake(fake, 1)

	err := svc.VerifyUSDCTransfer(context.Background(), testTxHash, recipient.Hex(), 5_000_000)
	if !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("head-fetch failure must map to ErrVerificationUnavailable, got %v", err)
	}
}

// TestCanonicalBlock_ReturnsCanonicalForPresentTx: a tx still in the chain
// reports its canonical block hash/number, found=true.
func TestCanonicalBlock_ReturnsCanonicalForPresentTx(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 102}
	svc := newServiceWithFake(fake, 3)

	hash, number, found, err := svc.CanonicalBlock(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !found {
		t.Fatal("expected found=true for a present tx")
	}
	if number != 100 {
		t.Fatalf("block number = %d, want 100", number)
	}
	if hash != common.BigToHash(new(big.Int).SetUint64(100)).Hex() {
		t.Fatalf("block hash = %q", hash)
	}
}

// TestCanonicalBlock_NotFoundIsOrphanNotError: an orphaned/dropped tx (RPC
// returns ethereum.NotFound) is reported as found=false with NO error, so the
// reconciler treats it as a reorg candidate rather than a transient failure.
func TestCanonicalBlock_NotFoundIsOrphanNotError(t *testing.T) {
	fake := &fakeRPC{receiptErr: ethereum.NotFound}
	svc := newServiceWithFake(fake, 3)

	hash, number, found, err := svc.CanonicalBlock(context.Background(), testTxHash)
	if err != nil {
		t.Fatalf("NotFound must not be an error, got %v", err)
	}
	if found {
		t.Fatal("expected found=false for a dropped tx")
	}
	if hash != "" || number != 0 {
		t.Fatalf("expected zero value for a dropped tx, got hash=%q number=%d", hash, number)
	}
}

// TestCanonicalBlock_TransientRPCErrorIsError: a non-NotFound RPC error must
// surface as an error (found=false) so a flaky node is never mistaken for a
// reorg and cannot trigger a false alert.
func TestCanonicalBlock_TransientRPCErrorIsError(t *testing.T) {
	fake := &fakeRPC{receiptErr: errors.New("dial tcp: connection refused")}
	svc := newServiceWithFake(fake, 3)

	_, _, found, err := svc.CanonicalBlock(context.Background(), testTxHash)
	if err == nil {
		t.Fatal("expected a transient RPC error to surface, not be swallowed as an orphan")
	}
	if found {
		t.Fatal("found must be false on error")
	}
}

// TestVerifyUSDCTransfer_RejectsNonCanonicalBlock pins that a receipt from a
// block the node no longer holds at that height (a reorged-out fork, or a
// lagging node behind a load balancer) is never credited, however deep.
func TestVerifyUSDCTransfer_RejectsNonCanonicalBlock(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{
		receipt:       transferReceipt(recipient, 5_000_000, 100),
		head:          200,
		canonicalHash: common.HexToHash("0xf0f0"),
	}
	svc := newServiceWithFake(fake, 3)

	_, err := svc.VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, false)
	if !errors.Is(err, ErrNonCanonicalBlock) || !errors.Is(err, ErrAwaitingConfirmations) {
		t.Fatalf("expected non-canonical awaiting error, got %v", err)
	}

	// Inclusion-only mode must not skip the canonical check either.
	fake.canonicalCalls = 0
	_, err = newServiceWithFake(fake, 0).VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, false)
	if !errors.Is(err, ErrNonCanonicalBlock) || fake.canonicalCalls != 1 {
		t.Fatalf("min=0: expected non-canonical rejection after one lookup, got %v (calls=%d)", err, fake.canonicalCalls)
	}
}

func TestVerifyUSDCTransfer_CanonicalLookupFailureIsUnavailable(t *testing.T) {
	recipient := common.HexToAddress("0x1111111111111111111111111111111111111111")
	fake := &fakeRPC{receipt: transferReceipt(recipient, 5_000_000, 100), head: 200, canonicalErr: errors.New("rpc down")}
	_, err := newServiceWithFake(fake, 3).VerifyUSDCTransferWithEvidence(context.Background(), testTxHash, recipient.Hex(), 5_000_000, false)
	if !errors.Is(err, ErrVerificationUnavailable) {
		t.Fatalf("expected ErrVerificationUnavailable, got %v", err)
	}
}

func TestResolveMinConfirmations_ProductionNeverZero(t *testing.T) {
	t.Setenv("USDC_MIN_CONFIRMATIONS", "0")
	if got := resolveMinConfirmations(true); got != 1 {
		t.Fatalf("production min confirmations = %d, want 1", got)
	}
	if got := resolveMinConfirmations(false); got != 0 {
		t.Fatalf("non-production min confirmations = %d, want 0", got)
	}
	t.Setenv("USDC_MIN_CONFIRMATIONS", "5")
	if got := resolveMinConfirmations(true); got != 5 {
		t.Fatalf("production min confirmations = %d, want 5", got)
	}
	t.Setenv("USDC_MIN_CONFIRMATIONS", "")
	if got := resolveMinConfirmations(true); got != defaultMinConfirmations {
		t.Fatalf("default min confirmations = %d, want %d", got, defaultMinConfirmations)
	}
}

package blockchain

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"os"
	"strconv"
	"strings"
	"time"

	ethereum "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/stdevmac/payverge/backend/internal/config"
	"github.com/stdevmac/payverge/backend/internal/txhash"
)

// usdcVerifyRPCTimeout bounds each on-chain receipt lookup. go-ethereum's
// ethclient has no built-in timeout and relies on the context; without this a
// hung/slow RPC node would stall the payment-verify goroutine for as long as
// the guest keeps the HTTP connection open (the server sets no ReadTimeout
// because it also streams SSE). The bound is derived from the caller's ctx, so
// an earlier client disconnect still cancels sooner.
const usdcVerifyRPCTimeout = 30 * time.Second

// defaultMinConfirmations is how many block confirmations an inbound USDC
// transfer must have before it is accepted as settled. Accepting a transfer at
// 0–1 blocks deep lets a chain reorg silently reverse an already-credited bill
// (the merchant ships against money that no longer exists). The default is
// conservative for the Base production chain (~2s blocks) while keeping the POS
// flow snappy; tune via USDC_MIN_CONFIRMATIONS.
const defaultMinConfirmations uint64 = 3

// ErrAwaitingConfirmations is returned when a transaction is found and matches
// the expected transfer but has not yet reached the required confirmation
// depth. It is a *retryable* condition (not a verification failure): the guest
// should be told to wait and retry with the same tx hash, which settles
// idempotently once the transfer is deep enough.
var ErrAwaitingConfirmations = errors.New("transaction awaiting blockchain confirmations")

// ErrVerificationUnavailable means we could not reach the chain to check the
// transaction at all — distinct from ErrAwaitingConfirmations, which means we
// reached it and the transfer simply isn't confirmed yet.
var ErrVerificationUnavailable = errors.New("blockchain verification temporarily unavailable")

// ErrTransferAmountMismatch means the transaction does pay the expected
// recipient in USDC, but not the amount required. It is a hard failure (never
// retryable): guest settlement binds a transfer to its quote by exact amount,
// so a different amount is either a different payment or a manual send that
// staff must reconcile by hand.
var ErrTransferAmountMismatch = errors.New("usdc transfer amount does not match the expected amount")

var erc20TransferTopic = crypto.Keccak256Hash([]byte("Transfer(address,address,uint256)"))

// rpcClient is the minimal slice of *ethclient.Client the verifier depends on.
// Narrowing to an interface lets the confirmation/finality logic be unit-tested
// against a fake node.
type rpcClient interface {
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	BlockNumber(ctx context.Context) (uint64, error)
	HeaderByHash(ctx context.Context, hash common.Hash) (*types.Header, error)
	// CanonicalBlockHash returns the hash of the block the node currently
	// holds on its canonical chain at number.
	CanonicalBlockHash(ctx context.Context, number *big.Int) (common.Hash, error)
	Close()
}

// ethRPC adapts *ethclient.Client to rpcClient.
type ethRPC struct {
	*ethclient.Client
}

// CanonicalBlockHash reads the hash the node reports for the block at number.
// It deliberately does not recompute the hash from a decoded header: an L2
// header field this go-ethereum version does not model would change the local
// hash and make every receipt look non-canonical. Both sides of the comparison
// (this hash and receipt.BlockHash) are then node-reported.
func (c ethRPC) CanonicalBlockHash(ctx context.Context, number *big.Int) (common.Hash, error) {
	if number == nil || number.Sign() < 0 {
		return common.Hash{}, errors.New("canonical block lookup needs a concrete block number")
	}
	var block *struct {
		Hash *common.Hash `json:"hash"`
	}
	if err := c.Client.Client().CallContext(ctx, &block, "eth_getBlockByNumber", hexutil.EncodeBig(number), false); err != nil {
		return common.Hash{}, err
	}
	if block == nil || block.Hash == nil {
		return common.Hash{}, errors.New("missing canonical block")
	}
	return *block.Hash, nil
}

// ErrNonCanonicalBlock means the receipt's block is not the block the node
// holds at that height (a reorg, or a lagging node behind a load balancer).
// It wraps ErrAwaitingConfirmations so callers retry instead of crediting.
var ErrNonCanonicalBlock = fmt.Errorf("transfer block is not on the canonical chain: %w", ErrAwaitingConfirmations)

// confirmationDepth reports how many confirmations a transaction mined at
// blockNumber has, given the current chain head. The mined block itself counts
// as the first confirmation. Returns 0 when the block number is unknown or the
// head trails the tx (RPC lag / reorg), so callers treat that as "not yet
// confirmed" rather than accepting.
func confirmationDepth(head uint64, blockNumber *big.Int) uint64 {
	if blockNumber == nil || !blockNumber.IsUint64() {
		return 0
	}
	mined := blockNumber.Uint64()
	if head < mined {
		return 0
	}
	return head - mined + 1
}

// resolveMinConfirmations reads USDC_MIN_CONFIRMATIONS, falling back to the
// conservative default. A value of 0 means "accept on inclusion" and skips
// the chain-head lookup. Values >= 1 always load the head and require
// confs >= min (including min=1, which still verifies the block is on the
// canonical tip rather than assuming inclusion alone is enough under reorgs).
//
// Production never accepts on inclusion alone: a configured 0 is raised to 1.
func resolveMinConfirmations(production bool) uint64 {
	n := defaultMinConfirmations
	if raw := strings.TrimSpace(os.Getenv("USDC_MIN_CONFIRMATIONS")); raw != "" {
		if parsed, err := strconv.ParseUint(raw, 10, 64); err == nil {
			n = parsed
		}
	}
	if production && n == 0 {
		log.Printf("USDC_MIN_CONFIRMATIONS=0 is not allowed in production; using 1")
		n = 1
	}
	return n
}

// BlockchainService verifies inbound USDC transfers used by the live crypto
// and cross-chain guest payment paths. The legacy custom-contract surface
// (PayvergePayments and friends) was removed; this service no longer signs
// transactions or holds a contract address.
type BlockchainService struct {
	client           rpcClient
	chainID          *big.Int
	minConfirmations uint64
}

// chainIDProbeTimeout caps the startup eth_chainId probe.
const chainIDProbeTimeout = 15 * time.Second

// NewBlockchainService dials an EVM RPC endpoint and resolves the chain ID
// used to pick the correct USDC token address during verification.
func NewBlockchainService(rpcURL string) (*BlockchainService, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	// Bound the startup probe: an unreachable or black-holed RPC must degrade
	// crypto to unavailable, not hang boot for the restaurant flows.
	ctx, cancel := context.WithTimeout(context.Background(), chainIDProbeTimeout)
	defer cancel()
	chainID, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to get chain ID: %v", err)
	}

	return &BlockchainService{
		client:           ethRPC{client},
		chainID:          chainID,
		minConfirmations: resolveMinConfirmations(config.IsProductionMode(false)),
	}, nil
}

// USDCTransferEvidence is the verified on-chain Transfer log used for settlement
// and refund-destination capture. Amounts are integer base units (USDC 6
// decimals = micro-units). From is the ERC-20 Transfer sender (Topics[1]) and
// is the only safe refund destination for a direct Base USDC payment.
type USDCTransferEvidence struct {
	From            string
	To              string
	AmountBaseUnits int64
	TxHash          string
	LogIndex        uint
	ChainID         int64
	Token           string
	TokenAddress    string
	Confirmations   uint64
	// BlockNumber / BlockHash pin the transfer to the exact block it settled
	// in. They are persisted so a later reorg-reconciliation sweep can re-read
	// the canonical chain and detect a confirmed payment/refund that a deep
	// reorg has orphaned (see internal/services/reorgwatch). Empty/0 when the
	// receipt omitted them.
	BlockNumber uint64
	BlockHash   string
	// BlockTimestamp is the unix time (seconds) of the block the transfer was
	// mined in. Callers compare it against the quote's issued-at so a transfer
	// mined before the quote existed (e.g. a historical payment to the same
	// settlement wallet) cannot be claimed against a new bill.
	BlockTimestamp uint64
}

// VerifyUSDCTransfer confirms that a transaction contains a successful USDC
// transfer to the expected recipient for the exact expected amount in
// micro-units.
func (s *BlockchainService) VerifyUSDCTransfer(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error {
	_, err := s.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expectedAmountMicrounits, false)
	return err
}

// VerifyUSDCTransferAtLeast confirms that a transaction contains a successful
// USDC transfer to the expected recipient for at least the expected amount.
// Cross-chain routes can settle slightly above the bill amount because of
// slippage buffers; direct USDC payments still use exact verification.
func (s *BlockchainService) VerifyUSDCTransferAtLeast(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64) error {
	_, err := s.VerifyUSDCTransferWithEvidence(ctx, txHash, recipient, expectedAmountMicrounits, true)
	return err
}

// VerifyUSDCTransferWithEvidence is the evidence-returning form of USDC
// settlement verification. It extracts the payer (Topics[1]) so callers can
// persist a verified refund destination without trusting guest-supplied
// addresses or the crypto_guest / cross_chain_guest ledger sentinels.
func (s *BlockchainService) VerifyUSDCTransferWithEvidence(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64, allowExcess bool) (USDCTransferEvidence, error) {
	return s.verifyUSDCTransfer(ctx, txHash, recipient, expectedAmountMicrounits, allowExcess, "", "")
}

// VerifyOutboundUSDCTransfer confirms a successful outbound USDC transfer FROM
// a configured settlement/refund wallet TO a verified refund destination for
// the exact base-unit amount. Used by the noncustodial crypto-refund worker
// after the owner submits a tx hash. Wrong from/to/amount/token/chain are
// hard failures.
func (s *BlockchainService) VerifyOutboundUSDCTransfer(ctx context.Context, txHash string, from string, to string, expectedAmountMicrounits int64) (USDCTransferEvidence, error) {
	return s.verifyUSDCTransfer(ctx, txHash, to, expectedAmountMicrounits, false, from, "")
}

// ChainID returns the chain the service is dialed against, or 0 when unset.
func (s *BlockchainService) ChainID() int64 {
	if s == nil || s.chainID == nil {
		return 0
	}
	return s.chainID.Int64()
}

// MinConfirmations returns the configured confirmation depth.
func (s *BlockchainService) MinConfirmations() uint64 {
	if s == nil {
		return defaultMinConfirmations
	}
	return s.minConfirmations
}

// TokenSymbolForChain returns the stablecoin symbol verified on this chain.
func (s *BlockchainService) TokenSymbolForChain() string {
	return "USDC"
}

func (s *BlockchainService) verifyUSDCTransfer(ctx context.Context, txHash string, recipient string, expectedAmountMicrounits int64, allowExcess bool, requiredFrom string, _ string) (USDCTransferEvidence, error) {
	var empty USDCTransferEvidence
	if s == nil || s.client == nil {
		return empty, fmt.Errorf("blockchain verification service unavailable")
	}
	if strings.TrimSpace(txHash) == "" {
		return empty, fmt.Errorf("transaction hash is required")
	}
	// Strict parse: exactly 0x + 64 hex, reported back in lowercase. Never
	// hand a lenient spelling to common.HexToHash — it maps case variants,
	// missing prefixes, zero padding and trailing garbage onto the same
	// receipt, which turns one transfer into many distinct ledger keys.
	canonicalHash, ok := txhash.Canonical(txHash)
	if !ok {
		return empty, fmt.Errorf("transaction hash must be 0x followed by 64 hex characters")
	}
	txHash = canonicalHash
	if expectedAmountMicrounits <= 0 {
		return empty, fmt.Errorf("expected payment amount must be greater than zero")
	}

	rpcCtx, cancel := context.WithTimeout(ctx, usdcVerifyRPCTimeout)
	defer cancel()

	receipt, err := s.client.TransactionReceipt(rpcCtx, common.HexToHash(txHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return empty, fmt.Errorf("transaction not yet mined: %w", ErrAwaitingConfirmations)
		}
		return empty, fmt.Errorf("failed to load transaction receipt: %w", ErrVerificationUnavailable)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		return empty, fmt.Errorf("transaction was not successful")
	}

	usdcAddress, err := s.usdcTokenAddress()
	if err != nil {
		return empty, err
	}

	recipientAddress := common.HexToAddress(strings.TrimSpace(recipient))
	expectedAmount := big.NewInt(expectedAmountMicrounits)
	var requiredFromAddr common.Address
	requireFrom := strings.TrimSpace(requiredFrom) != ""
	if requireFrom {
		requiredFromAddr = common.HexToAddress(strings.TrimSpace(requiredFrom))
	}

	var matched *types.Log
	var matchedFrom common.Address
	var matchedAmount *big.Int
	sawRecipientTransfer := false
	for _, logEntry := range receipt.Logs {
		if logEntry == nil || logEntry.Address != usdcAddress || len(logEntry.Topics) < 3 {
			continue
		}
		if logEntry.Topics[0] != erc20TransferTopic {
			continue
		}

		from := common.HexToAddress(logEntry.Topics[1].Hex())
		to := common.HexToAddress(logEntry.Topics[2].Hex())
		if to != recipientAddress {
			continue
		}
		if requireFrom && from != requiredFromAddr {
			continue
		}

		sawRecipientTransfer = true
		amount := new(big.Int).SetBytes(logEntry.Data)
		if !amount.IsInt64() {
			continue
		}
		comparison := amount.Cmp(expectedAmount)
		if comparison == 0 || (allowExcess && comparison > 0) {
			matched = logEntry
			matchedFrom = from
			matchedAmount = amount
			break
		}
	}

	if matched == nil {
		if sawRecipientTransfer {
			return empty, fmt.Errorf("%w (expected %d micro-units)", ErrTransferAmountMismatch, expectedAmountMicrounits)
		}
		return empty, fmt.Errorf("no matching USDC transfer found for transaction")
	}

	// The transfer matches; only accept it once it is buried deep enough that a
	// reorg can't quietly reverse the credit. A shallow-but-valid transfer is a
	// retryable "awaiting confirmations", distinct from a hard failure, so the
	// guest is told to wait and retry the same (idempotent) tx hash.
	// minConfirmations == 0: accept on inclusion (no head RPC). min >= 1
	// (including exactly 1) always loads the chain head so reorg risk is
	// measured — previously min==1 skipped the head and treated every
	// inclusion as confirmed.
	var confs uint64 = 1
	if s.minConfirmations == 0 {
		// accept on inclusion
	} else {
		head, err := s.client.BlockNumber(rpcCtx)
		if err != nil {
			return empty, fmt.Errorf("failed to load chain head: %w", ErrVerificationUnavailable)
		}
		confs = confirmationDepth(head, receipt.BlockNumber)
		if confs < s.minConfirmations {
			return empty, fmt.Errorf("%w: %d/%d confirmations", ErrAwaitingConfirmations, confs, s.minConfirmations)
		}
	}

	// The receipt's block must be the block the node holds at that height.
	// Depth alone is measured against the head and says nothing about which
	// fork the receipt came from.
	if receipt.BlockNumber == nil {
		return empty, fmt.Errorf("transfer receipt has no block number: %w", ErrVerificationUnavailable)
	}
	canonical, err := s.client.CanonicalBlockHash(rpcCtx, receipt.BlockNumber)
	if err != nil {
		return empty, fmt.Errorf("failed to load canonical block: %w", ErrVerificationUnavailable)
	}
	if canonical != receipt.BlockHash {
		return empty, ErrNonCanonicalBlock
	}

	// Block time lets the handler bind the transfer to "mined after the quote
	// was issued". A missing header is an RPC problem, not a verdict: it is
	// retryable, never a silent pass with an unknown timestamp.
	header, err := s.client.HeaderByHash(rpcCtx, receipt.BlockHash)
	if err != nil || header == nil {
		return empty, fmt.Errorf("failed to load transfer block header: %w", ErrVerificationUnavailable)
	}

	return USDCTransferEvidence{
		From:            matchedFrom.Hex(),
		To:              recipientAddress.Hex(),
		AmountBaseUnits: matchedAmount.Int64(),
		TxHash:          txHash,
		LogIndex:        matched.Index,
		ChainID:         s.ChainID(),
		Token:           "USDC",
		TokenAddress:    usdcAddress.Hex(),
		Confirmations:   confs,
		BlockNumber:     blockNumberU64(receipt.BlockNumber),
		BlockHash:       receipt.BlockHash.Hex(),
		BlockTimestamp:  header.Time,
	}, nil
}

// blockNumberU64 safely narrows a receipt block number to uint64, returning 0
// when it is nil or out of range (treated as "unknown block").
func blockNumberU64(bn *big.Int) uint64 {
	if bn == nil || !bn.IsUint64() {
		return 0
	}
	return bn.Uint64()
}

// CanonicalBlock re-reads the current canonical chain state for txHash. It is
// the read side of reorg reconciliation: given a tx we previously accepted as
// settled, it reports the block hash/number the canonical chain associates with
// it NOW. found=false means the tx is no longer in the canonical chain — it was
// dropped/orphaned by a reorganization and any credit booked against it is
// suspect. A transient RPC error is returned as err (callers must treat that as
// "unknown", never as an orphan) so a flaky node cannot masquerade as a reorg.
func (s *BlockchainService) CanonicalBlock(ctx context.Context, txHash string) (blockHash string, blockNumber uint64, found bool, err error) {
	if s == nil || s.client == nil {
		return "", 0, false, fmt.Errorf("blockchain verification service unavailable")
	}
	txHash = strings.TrimSpace(txHash)
	if txHash == "" {
		return "", 0, false, fmt.Errorf("transaction hash is required")
	}

	rpcCtx, cancel := context.WithTimeout(ctx, usdcVerifyRPCTimeout)
	defer cancel()

	receipt, err := s.client.TransactionReceipt(rpcCtx, common.HexToHash(txHash))
	if err != nil {
		if errors.Is(err, ethereum.NotFound) {
			return "", 0, false, nil
		}
		return "", 0, false, fmt.Errorf("failed to load transaction receipt: %w", err)
	}
	if receipt == nil {
		return "", 0, false, nil
	}
	return receipt.BlockHash.Hex(), blockNumberU64(receipt.BlockNumber), true, nil
}

func (s *BlockchainService) usdcTokenAddress() (common.Address, error) {
	switch s.chainID.Uint64() {
	case 8453:
		return common.HexToAddress("0x833589fCD6eDb6E08f4c7C32D4f71b54bdA02913"), nil
	case 84532:
		return common.HexToAddress("0x82d491aB292C06Aa7148234b910cdea5FE788223"), nil
	case 1:
		return common.HexToAddress("0xA0b86991c6218b36c1d19D4a2e9Eb0cE3606eB48"), nil
	case 11155111:
		return common.HexToAddress("0x1c7D4B196Cb0C7B01d743Fbc6116a902379C7238"), nil
	default:
		return common.Address{}, fmt.Errorf("unsupported chain for USDC verification: %s", s.chainID.String())
	}
}

// Close releases the underlying EVM client connection.
func (s *BlockchainService) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

package services

import (
	"context"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// CouponService handles coupon synchronization with smart contracts
type CouponService struct {
	client          *ethclient.Client
	contractAddress common.Address
	contractABI     abi.ABI
}

// CouponInfo represents coupon data from smart contract
type CouponInfo struct {
	Code           string    `json:"code"`
	Hash           string    `json:"hash"`
	DiscountAmount *big.Int  `json:"discountAmount"`
	ExpiryTime     *big.Int  `json:"expiryTime"`
	IsActive       bool      `json:"isActive"`
	IsUsed         bool      `json:"isUsed"`
	UsedBy         string    `json:"usedBy,omitempty"`
	UsedAt         time.Time `json:"usedAt,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

// CouponEvent represents events from the smart contract
type CouponEvent struct {
	Type           string    `json:"type"`
	CouponHash     string    `json:"couponHash"`
	Code           string    `json:"code,omitempty"`
	DiscountAmount *big.Int  `json:"discountAmount,omitempty"`
	ExpiryTime     *big.Int  `json:"expiryTime,omitempty"`
	Business       string    `json:"business,omitempty"`
	Timestamp      time.Time `json:"timestamp"`
	BlockNumber    uint64    `json:"blockNumber"`
	TxHash         string    `json:"txHash"`
}

// NewCouponService creates a new coupon service
func NewCouponService(rpcURL, contractAddress string) (*CouponService, error) {
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	// PayvergePayments ABI for coupon events (simplified)
	abiJSON := `[
		{
			"anonymous": false,
			"inputs": [
				{"indexed": true, "name": "couponHash", "type": "bytes32"},
				{"indexed": false, "name": "discountAmount", "type": "uint256"},
				{"indexed": false, "name": "expiryTime", "type": "uint64"}
			],
			"name": "CouponCreated",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{"indexed": true, "name": "couponHash", "type": "bytes32"}
			],
			"name": "CouponDeactivated",
			"type": "event"
		},
		{
			"anonymous": false,
			"inputs": [
				{"indexed": true, "name": "couponHash", "type": "bytes32"},
				{"indexed": true, "name": "business", "type": "address"},
				{"indexed": false, "name": "discountAmount", "type": "uint256"}
			],
			"name": "CouponUsed",
			"type": "event"
		},
		{
			"inputs": [{"name": "couponHash", "type": "bytes32"}],
			"name": "getCouponInfo",
			"outputs": [
				{"name": "discountAmount", "type": "uint256"},
				{"name": "expiryTime", "type": "uint64"},
				{"name": "isActive", "type": "bool"},
				{"name": "isUsed", "type": "bool"}
			],
			"stateMutability": "view",
			"type": "function"
		}
	]`

	contractABI, err := abi.JSON(strings.NewReader(abiJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract ABI: %v", err)
	}

	return &CouponService{
		client:          client,
		contractAddress: common.HexToAddress(contractAddress),
		contractABI:     contractABI,
	}, nil
}

// GenerateCouponHash generates the hash for a coupon code (matches Solidity keccak256)
func (cs *CouponService) GenerateCouponHash(code string) string {
	hash := crypto.Keccak256Hash([]byte(code))
	return hash.Hex()
}

// GetCouponInfo retrieves coupon information from smart contract
func (cs *CouponService) GetCouponInfo(couponHash string) (*CouponInfo, error) {
	// Call smart contract
	result, err := cs.client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &cs.contractAddress,
		Data: cs.contractABI.Methods["getCouponInfo"].ID,
	}, nil)

	if err != nil {
		return nil, fmt.Errorf("failed to call smart contract: %v", err)
	}

	// Parse result
	outputs, err := cs.contractABI.Methods["getCouponInfo"].Outputs.Unpack(result)

	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	// Extract values from outputs
	if len(outputs) < 4 {
		return nil, fmt.Errorf("insufficient outputs from contract call")
	}

	discountAmount, ok := outputs[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to parse discount amount")
	}

	expiryTime, ok := outputs[1].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("failed to parse expiry time")
	}

	isActive, ok := outputs[2].(bool)
	if !ok {
		return nil, fmt.Errorf("failed to parse active status")
	}

	isUsed, ok := outputs[3].(bool)
	if !ok {
		return nil, fmt.Errorf("failed to parse used status")
	}

	return &CouponInfo{
		Hash:           couponHash,
		DiscountAmount: discountAmount,
		ExpiryTime:     expiryTime,
		IsActive:       isActive,
		IsUsed:         isUsed,
		CreatedAt:      time.Now(), // We don't have creation time from contract
	}, nil
}

// WatchCouponEvents watches for coupon events from the smart contract
func (cs *CouponService) WatchCouponEvents(ctx context.Context, eventChan chan<- CouponEvent, fromBlock *big.Int) error {
	// Create filter query
	query := ethereum.FilterQuery{
		Addresses: []common.Address{cs.contractAddress},
		Topics: [][]common.Hash{
			{
				cs.contractABI.Events["CouponCreated"].ID,
				cs.contractABI.Events["CouponDeactivated"].ID,
				cs.contractABI.Events["CouponUsed"].ID,
			},
		},
		FromBlock: fromBlock,
	}

	// Subscribe to logs
	logs := make(chan types.Log)
	sub, err := cs.client.SubscribeFilterLogs(ctx, query, logs)
	if err != nil {
		return fmt.Errorf("failed to subscribe to logs: %v", err)
	}

	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case err := <-sub.Err():
				log.Printf("Coupon event subscription error: %v", err)
				return
			case vLog := <-logs:
				event, err := cs.parseLogEvent(vLog)
				if err != nil {
					log.Printf("Error parsing log event: %v", err)
					continue
				}
				if event != nil {
					eventChan <- *event
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return nil
}

// parseLogEvent parses a log event into a CouponEvent
func (cs *CouponService) parseLogEvent(vLog types.Log) (*CouponEvent, error) {
	switch vLog.Topics[0] {
	case cs.contractABI.Events["CouponCreated"].ID:
		return cs.parseCouponCreatedEvent(vLog)
	case cs.contractABI.Events["CouponDeactivated"].ID:
		return cs.parseCouponDeactivatedEvent(vLog)
	case cs.contractABI.Events["CouponUsed"].ID:
		return cs.parseCouponUsedEvent(vLog)
	default:
		return nil, nil // Unknown event
	}
}

// parseCouponCreatedEvent parses CouponCreated event
func (cs *CouponService) parseCouponCreatedEvent(vLog types.Log) (*CouponEvent, error) {
	var event struct {
		CouponHash     common.Hash
		DiscountAmount *big.Int
		ExpiryTime     uint64
	}

	err := cs.contractABI.UnpackIntoInterface(&event, "CouponCreated", vLog.Data)
	if err != nil {
		return nil, err
	}

	// Get coupon hash from topics
	couponHash := vLog.Topics[1].Hex()

	return &CouponEvent{
		Type:           "created",
		CouponHash:     couponHash,
		DiscountAmount: event.DiscountAmount,
		ExpiryTime:     big.NewInt(int64(event.ExpiryTime)),
		Timestamp:      time.Now(),
		BlockNumber:    vLog.BlockNumber,
		TxHash:         vLog.TxHash.Hex(),
	}, nil
}

// parseCouponDeactivatedEvent parses CouponDeactivated event
func (cs *CouponService) parseCouponDeactivatedEvent(vLog types.Log) (*CouponEvent, error) {
	couponHash := vLog.Topics[1].Hex()

	return &CouponEvent{
		Type:        "deactivated",
		CouponHash:  couponHash,
		Timestamp:   time.Now(),
		BlockNumber: vLog.BlockNumber,
		TxHash:      vLog.TxHash.Hex(),
	}, nil
}

// parseCouponUsedEvent parses CouponUsed event
func (cs *CouponService) parseCouponUsedEvent(vLog types.Log) (*CouponEvent, error) {
	var event struct {
		CouponHash     common.Hash
		Business       common.Address
		DiscountAmount *big.Int
	}

	err := cs.contractABI.UnpackIntoInterface(&event, "CouponUsed", vLog.Data)
	if err != nil {
		return nil, err
	}

	couponHash := vLog.Topics[1].Hex()
	business := vLog.Topics[2].Hex()

	return &CouponEvent{
		Type:           "used",
		CouponHash:     couponHash,
		Business:       business,
		DiscountAmount: event.DiscountAmount,
		Timestamp:      time.Now(),
		BlockNumber:    vLog.BlockNumber,
		TxHash:         vLog.TxHash.Hex(),
	}, nil
}

// ValidateCouponCode validates a coupon code format
func (cs *CouponService) ValidateCouponCode(code string) error {
	if len(code) < 3 || len(code) > 32 {
		return fmt.Errorf("coupon code must be between 3 and 32 characters")
	}

	// Only allow alphanumeric and some special characters
	for _, char := range code {
		if !((char >= 'A' && char <= 'Z') || 
			 (char >= '0' && char <= '9') || 
			 char == '-' || char == '_') {
			return fmt.Errorf("coupon code can only contain uppercase letters, numbers, hyphens, and underscores")
		}
	}

	return nil
}

// FormatUSDC formats a big.Int USDC amount to a human-readable string
func (cs *CouponService) FormatUSDC(amount *big.Int) string {
	if amount == nil {
		return "0.00"
	}

	// Convert to float for proper formatting (USDC has 6 decimals)
	amountFloat := float64(amount.Int64()) / 1000000.0
	
	// Format as currency with 2 decimal places
	return fmt.Sprintf("%.2f", amountFloat)
}

// ParseUSDC parses a human-readable USDC string to big.Int
func (cs *CouponService) ParseUSDC(amount string) (*big.Int, error) {
	// Remove any whitespace
	amount = strings.TrimSpace(amount)
	
	// Handle decimal point
	parts := strings.Split(amount, ".")
	if len(parts) > 2 {
		return nil, fmt.Errorf("invalid amount format")
	}

	wholePart := parts[0]
	decimalPart := ""
	if len(parts) == 2 {
		decimalPart = parts[1]
		// Pad or truncate to 6 decimal places
		if len(decimalPart) > 6 {
			decimalPart = decimalPart[:6]
		} else {
			for len(decimalPart) < 6 {
				decimalPart += "0"
			}
		}
	} else {
		decimalPart = "000000"
	}

	// Combine and parse
	fullAmount := wholePart + decimalPart
	result, ok := new(big.Int).SetString(fullAmount, 10)
	if !ok {
		return nil, fmt.Errorf("invalid amount format")
	}

	return result, nil
}

package blockchain

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// PaymentRequest represents a payment request for the unified payment system
type PaymentRequest struct {
	BillID    string `json:"bill_id"`
	Amount    int64  `json:"amount"`     // USDC amount in wei (6 decimals)
	TipAmount int64  `json:"tip_amount"` // Tip amount in wei (6 decimals)
}

// PaymentResult represents the result of a payment transaction
type PaymentResult struct {
	TransactionHash string    `json:"transaction_hash"`
	BlockNumber     uint64    `json:"block_number"`
	GasUsed         uint64    `json:"gas_used"`
	PlatformFee     int64     `json:"platform_fee"`
	Status          string    `json:"status"`
	Timestamp       time.Time `json:"timestamp"`
}

// Payment represents a payment record from the blockchain
type Payment struct {
	BillID          string    `json:"bill_id"`
	Payer           string    `json:"payer"`
	Amount          int64     `json:"amount"`
	TipAmount       int64     `json:"tip_amount"`
	PlatformFee     int64     `json:"platform_fee"`
	Timestamp       time.Time `json:"timestamp"`
	TransactionHash string    `json:"transaction_hash"`
}

// Participant represents a bill participant from the unified payment system
type Participant struct {
	Address      string `json:"address"`
	PaidAmount   int64  `json:"paid_amount"`
	PaymentCount int    `json:"payment_count"`
	LastPayment  int64  `json:"last_payment"`
}

// BlockchainService handles blockchain interactions
type BlockchainService struct {
	client          *ethclient.Client
	contractAddress common.Address
	contractABI     abi.ABI
	privateKey      *ecdsa.PrivateKey
	chainID         *big.Int
}

// PayvergePayments contract ABI (v5.0.0-unified-simple - key functions only)
const PayvergePaymentsABI = `[
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"},
			{"internalType": "uint256", "name": "amount", "type": "uint256"},
			{"internalType": "uint256", "name": "tipAmount", "type": "uint256"}
		],
		"name": "processPayment",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"},
			{"internalType": "address", "name": "businessAddress", "type": "address"},
			{"internalType": "uint256", "name": "totalAmount", "type": "uint256"},
			{"internalType": "string", "name": "metadata", "type": "string"},
			{"internalType": "bytes32", "name": "nonce", "type": "bytes32"}
		],
		"name": "createBill",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"},
			{"internalType": "uint256", "name": "amount", "type": "uint256"},
			{"internalType": "uint256", "name": "tipAmount", "type": "uint256"},
			{"internalType": "address", "name": "businessAddress", "type": "address"},
			{"internalType": "address", "name": "tipAddress", "type": "address"}
		],
		"name": "payBill",
		"outputs": [],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"}
		],
		"name": "getBillPayments",
		"outputs": [
			{
				"components": [
					{"internalType": "bytes32", "name": "billId", "type": "bytes32"},
					{"internalType": "address", "name": "payer", "type": "address"},
					{"internalType": "uint256", "name": "amount", "type": "uint256"},
					{"internalType": "uint256", "name": "tipAmount", "type": "uint256"},
					{"internalType": "address", "name": "businessAddress", "type": "address"},
					{"internalType": "address", "name": "tipAddress", "type": "address"},
					{"internalType": "uint256", "name": "platformFee", "type": "uint256"},
					{"internalType": "uint256", "name": "timestamp", "type": "uint256"},
					{"internalType": "string", "name": "transactionHash", "type": "string"}
				],
				"internalType": "struct PayvergePayments.Payment[]",
				"name": "",
				"type": "tuple[]"
			}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"}
		],
		"name": "getBillTotalPaid",
		"outputs": [
			{"internalType": "uint256", "name": "", "type": "uint256"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"}
		],
		"name": "getBillParticipants",
		"outputs": [
			{"internalType": "address[]", "name": "", "type": "address[]"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"},
			{"internalType": "address", "name": "participant", "type": "address"}
		],
		"name": "getParticipantInfo",
		"outputs": [
			{"internalType": "uint256", "name": "paidAmount", "type": "uint256"},
			{"internalType": "uint32", "name": "paymentCount", "type": "uint32"},
			{"internalType": "uint32", "name": "lastPaymentTime", "type": "uint32"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"inputs": [
			{"internalType": "bytes32", "name": "billId", "type": "bytes32"}
		],
		"name": "getBillSummary",
		"outputs": [
			{"internalType": "uint256", "name": "totalAmount", "type": "uint256"},
			{"internalType": "uint256", "name": "paidAmount", "type": "uint256"},
			{"internalType": "uint8", "name": "participantCount", "type": "uint8"},
			{"internalType": "bool", "name": "isPaid", "type": "bool"}
		],
		"stateMutability": "view",
		"type": "function"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "bytes32", "name": "billId", "type": "bytes32"},
			{"indexed": true, "internalType": "address", "name": "payer", "type": "address"},
			{"internalType": "uint256", "name": "amount", "type": "uint256"},
			{"internalType": "uint256", "name": "tipAmount", "type": "uint256"},
			{"indexed": true, "internalType": "address", "name": "businessAddress", "type": "address"},
			{"internalType": "address", "name": "tipAddress", "type": "address"},
			{"internalType": "uint256", "name": "platformFee", "type": "uint256"},
			{"internalType": "uint256", "name": "timestamp", "type": "uint256"}
		],
		"name": "PaymentMade",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "internalType": "bytes32", "name": "billId", "type": "bytes32"},
			{"indexed": true, "internalType": "address", "name": "businessAddress", "type": "address"},
			{"internalType": "address", "name": "tipAddress", "type": "address"},
			{"internalType": "uint256", "name": "totalAmount", "type": "uint256"}
		],
		"name": "BillCreated",
		"type": "event"
	}
]`

// NewBlockchainService creates a new blockchain service
func NewBlockchainService(rpcURL, contractAddress, privateKeyHex string) (*BlockchainService, error) {
	// Connect to Ethereum client
	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	// Parse contract address
	contractAddr := common.HexToAddress(contractAddress)

	// Parse contract ABI
	contractABI, err := abi.JSON(strings.NewReader(PayvergePaymentsABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract ABI: %v", err)
	}

	// Parse private key
	privateKey, err := crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %v", err)
	}

	// Get chain ID
	chainID, err := client.ChainID(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get chain ID: %v", err)
	}

	return &BlockchainService{
		client:          client,
		contractAddress: contractAddr,
		contractABI:     contractABI,
		privateKey:      privateKey,
		chainID:         chainID,
	}, nil
}

// CreateBill creates a bill record on the blockchain (unified payment system)
func (s *BlockchainService) CreateBill(billID string, businessAddress string, totalAmount int64, metadata string, nonce string) (*PaymentResult, error) {
	// Convert bill ID to bytes32
	billIDBytes := crypto.Keccak256Hash([]byte(billID))

	// Convert business address
	businessAddr := common.HexToAddress(businessAddress)

	// Convert amount to wei (USDC has 6 decimals)
	amountWei := big.NewInt(totalAmount)

	// Convert nonce to bytes32
	nonceBytes := crypto.Keccak256Hash([]byte(nonce))

	// Get auth
	auth, err := s.getAuth()
	if err != nil {
		return nil, fmt.Errorf("failed to get auth: %v", err)
	}

	// Pack function call for unified payment system
	data, err := s.contractABI.Pack("createBill", billIDBytes, businessAddr, amountWei, metadata, nonceBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to pack function call: %v", err)
	}

	// Create transaction
	tx := types.NewTransaction(
		auth.Nonce.Uint64(),
		s.contractAddress,
		big.NewInt(0),
		auth.GasLimit,
		auth.GasPrice,
		data,
	)

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(s.chainID), s.privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %v", err)
	}

	// Send transaction
	err = s.client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %v", err)
	}

	// Wait for receipt
	receipt, err := s.waitForReceipt(signedTx.Hash())
	if err != nil {
		return nil, fmt.Errorf("failed to get transaction receipt: %v", err)
	}

	return &PaymentResult{
		TransactionHash: signedTx.Hash().Hex(),
		BlockNumber:     receipt.BlockNumber.Uint64(),
		GasUsed:         receipt.GasUsed,
		Status:          "success",
		Timestamp:       time.Now(),
	}, nil
}

// ProcessPayment processes a payment transaction (this would typically be called by the frontend)
// This function is mainly for monitoring and validation purposes
func (s *BlockchainService) ProcessPayment(payment PaymentRequest) (*PaymentResult, error) {
	// In a real implementation, this would validate the payment
	// For now, we'll return a mock result since payments are initiated from frontend
	return &PaymentResult{
		TransactionHash: "0x" + payment.BillID, // Mock transaction hash
		Status:          "pending",
		Timestamp:       time.Now(),
	}, nil
}

// GetBillPayments retrieves all payments for a specific bill
func (s *BlockchainService) GetBillPayments(ctx context.Context, billID string) ([]*PaymentResult, error) {
	billIDBytes := crypto.Keccak256Hash([]byte(billID))
	data, err := s.contractABI.Pack("getBillPayments", billIDBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to pack function call: %v", err)
	}

	// Call contract
	result, err := s.client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &s.contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %v", err)
	}

	// Unpack result
	var payments []struct {
		BillID          [32]byte
		Payer           common.Address
		Amount          *big.Int
		TipAmount       *big.Int
		BusinessAddress common.Address
		TipAddress      common.Address
		PlatformFee     *big.Int
		Timestamp       *big.Int
		TransactionHash string
		Status          string
	}

	err = s.contractABI.UnpackIntoInterface(&payments, "getBillPayments", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	// Convert database payments to PaymentResult format
	result_payments := make([]*PaymentResult, len(payments))
	for i, payment := range payments {
		result_payments[i] = &PaymentResult{
			TransactionHash: payment.TransactionHash,
			Status:          payment.Status,
			Timestamp:       time.Unix(payment.Timestamp.Int64(), 0),
		}
	}

	return result_payments, nil
}

// GetBillTotalPaid gets the total amount paid for a bill
func (s *BlockchainService) GetBillTotalPaid(billID string) (int64, error) {
	// Convert bill ID to bytes32
	billIDBytes := crypto.Keccak256Hash([]byte(billID))

	// Pack function call
	data, err := s.contractABI.Pack("getBillTotalPaid", billIDBytes)
	if err != nil {
		return 0, fmt.Errorf("failed to pack function call: %v", err)
	}

	// Call contract
	result, err := s.client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &s.contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to call contract: %v", err)
	}

	// Unpack result
	var totalPaid *big.Int
	err = s.contractABI.UnpackIntoInterface(&totalPaid, "getBillTotalPaid", result)
	if err != nil {
		return 0, fmt.Errorf("failed to unpack result: %v", err)
	}

	return totalPaid.Int64(), nil
}

// MonitorPayments starts monitoring for payment events
func (s *BlockchainService) MonitorPayments(callback func(Payment)) error {
	// Create filter for PaymentMade events
	query := ethereum.FilterQuery{
		Addresses: []common.Address{s.contractAddress},
		Topics: [][]common.Hash{
			{crypto.Keccak256Hash([]byte("PaymentMade(bytes32,address,uint256,uint256,address,address,uint256,uint256)"))},
		},
	}

	// Subscribe to logs
	logs := make(chan types.Log)
	sub, err := s.client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		return fmt.Errorf("failed to subscribe to logs: %v", err)
	}

	// Process logs
	go func() {
		defer sub.Unsubscribe()
		for {
			select {
			case err := <-sub.Err():
				log.Printf("Subscription error: %v", err)
				return
			case vLog := <-logs:
				payment, err := s.parsePaymentEvent(vLog)
				if err != nil {
					log.Printf("Failed to parse payment event: %v", err)
					continue
				}
				callback(payment)
			}
		}
	}()

	return nil
}

// parsePaymentEvent parses a PaymentMade event log
func (s *BlockchainService) parsePaymentEvent(vLog types.Log) (Payment, error) {
	// Parse the event
	event := struct {
		BillID          [32]byte
		Payer           common.Address
		Amount          *big.Int
		TipAmount       *big.Int
		BusinessAddress common.Address
		TipAddress      common.Address
		PlatformFee     *big.Int
		Timestamp       *big.Int
	}{}

	err := s.contractABI.UnpackIntoInterface(&event, "PaymentMade", vLog.Data)
	if err != nil {
		return Payment{}, fmt.Errorf("failed to unpack event: %v", err)
	}

	return Payment{
		BillID:          string(event.BillID[:]),
		Payer:           event.Payer.Hex(),
		Amount:          event.Amount.Int64(),
		TipAmount:       event.TipAmount.Int64(),
		PlatformFee:     event.PlatformFee.Int64(),
		Timestamp:       time.Unix(event.Timestamp.Int64(), 0),
		TransactionHash: vLog.TxHash.Hex(),
	}, nil
}

// getAuth creates transaction auth
func (s *BlockchainService) getAuth() (*bind.TransactOpts, error) {
	publicKey := s.privateKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		return nil, fmt.Errorf("error casting public key to ECDSA")
	}

	fromAddress := crypto.PubkeyToAddress(*publicKeyECDSA)
	nonce, err := s.client.PendingNonceAt(context.Background(), fromAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %v", err)
	}

	gasPrice, err := s.client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %v", err)
	}

	auth, err := bind.NewKeyedTransactorWithChainID(s.privateKey, s.chainID)
	if err != nil {
		return nil, fmt.Errorf("failed to create transactor: %v", err)
	}

	auth.Nonce = big.NewInt(int64(nonce))
	auth.Value = big.NewInt(0)
	auth.GasLimit = uint64(300000) // Adjust as needed
	auth.GasPrice = gasPrice

	return auth, nil
}

// waitForReceipt waits for a transaction receipt
func (s *BlockchainService) waitForReceipt(txHash common.Hash) (*types.Receipt, error) {
	for i := 0; i < 60; i++ { // Wait up to 60 seconds
		receipt, err := s.client.TransactionReceipt(context.Background(), txHash)
		if err == nil {
			return receipt, nil
		}
		time.Sleep(1 * time.Second)
	}
	return nil, fmt.Errorf("transaction receipt not found after 60 seconds")
}

// GetBillParticipants retrieves all participants for a bill
func (s *BlockchainService) GetBillParticipants(ctx context.Context, billID string) ([]string, error) {
	billIDBytes := crypto.Keccak256Hash([]byte(billID))
	data, err := s.contractABI.Pack("getBillParticipants", billIDBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to pack function call: %v", err)
	}

	result, err := s.client.CallContract(ctx, ethereum.CallMsg{
		To:   &s.contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %v", err)
	}

	var participants []common.Address
	err = s.contractABI.UnpackIntoInterface(&participants, "getBillParticipants", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	// Convert addresses to strings
	participantStrings := make([]string, len(participants))
	for i, addr := range participants {
		participantStrings[i] = addr.Hex()
	}

	return participantStrings, nil
}

// GetParticipantInfo retrieves information about a specific participant
func (s *BlockchainService) GetParticipantInfo(ctx context.Context, billID string, participant string) (*Participant, error) {
	billIDBytes := crypto.Keccak256Hash([]byte(billID))
	participantAddr := common.HexToAddress(participant)
	
	data, err := s.contractABI.Pack("getParticipantInfo", billIDBytes, participantAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to pack function call: %v", err)
	}

	result, err := s.client.CallContract(ctx, ethereum.CallMsg{
		To:   &s.contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %v", err)
	}

	var info struct {
		PaidAmount   *big.Int
		PaymentCount uint32
		LastPayment  uint32
	}
	
	err = s.contractABI.UnpackIntoInterface(&info, "getParticipantInfo", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	return &Participant{
		Address:      participant,
		PaidAmount:   info.PaidAmount.Int64(),
		PaymentCount: int(info.PaymentCount),
		LastPayment:  int64(info.LastPayment),
	}, nil
}

// GetBillSummary retrieves a summary of the bill including participant count and status
func (s *BlockchainService) GetBillSummary(ctx context.Context, billID string) (map[string]interface{}, error) {
	billIDBytes := crypto.Keccak256Hash([]byte(billID))
	data, err := s.contractABI.Pack("getBillSummary", billIDBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to pack function call: %v", err)
	}

	result, err := s.client.CallContract(ctx, ethereum.CallMsg{
		To:   &s.contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %v", err)
	}

	var summary struct {
		TotalAmount      *big.Int
		PaidAmount       *big.Int
		ParticipantCount uint8
		IsPaid           bool
	}
	
	err = s.contractABI.UnpackIntoInterface(&summary, "getBillSummary", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	return map[string]interface{}{
		"total_amount":      summary.TotalAmount.Int64(),
		"paid_amount":       summary.PaidAmount.Int64(),
		"participant_count": int(summary.ParticipantCount),
		"is_paid":           summary.IsPaid,
		"remaining_amount":  summary.TotalAmount.Int64() - summary.PaidAmount.Int64(),
	}, nil
}

// Close closes the blockchain service
func (s *BlockchainService) Close() {
	if s.client != nil {
		s.client.Close()
	}
}

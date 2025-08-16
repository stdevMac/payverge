package faucet

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"log"
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/stdevMac/shares/internal/database"
)

var (
	client         *ethclient.Client
	senderKey      *ecdsa.PrivateKey
	senderAddress  common.Address
	topUpAmountWei = big.NewInt(20000000000000) // 0.00002 ETH in Wei (reduced to avoid insufficient funds)
	minBalanceWei  = big.NewInt(80000000000000) // 0.00008 ETH in Wei (4x topUpAmount)
	chainID        *big.Int                     // Chain ID for the network
)

// InitFaucet initializes the faucet with the Ethereum client and sender's private key
func InitFaucet(rpcUrl, privateKeyHex string, networkChainID int64) {
	var err error

	// Ethereum node RPC URL
	client, err = ethclient.Dial(rpcUrl)
	if err != nil {
		log.Fatalf("Failed to connect to Ethereum: %v", err)
	}

	// Load private key
	senderKey, err = crypto.HexToECDSA(privateKeyHex)
	if err != nil {
		log.Fatalf("Invalid private key: %v", err)
	}

	senderAddress = crypto.PubkeyToAddress(senderKey.PublicKey)
	log.Printf("Funding wallet: %s\n", senderAddress.Hex())

	// Set chain ID
	chainID = big.NewInt(networkChainID)
}

// TopUpResponse contains information about the top-up transaction
type TopUpResponse struct {
	Status     string `json:"status"`
	TxHash     string `json:"tx_hash,omitempty"`
	ToAddress  string `json:"to_address,omitempty"`
	AmountSent string `json:"amount_sent,omitempty"`
	Balance    string `json:"balance,omitempty"`
	Error      string `json:"error,omitempty"`
}

// CheckAndTopUp checks if the address needs ETH and sends it if needed
func CheckAndTopUp(address string) (*TopUpResponse, error) {
	// Validate address
	if !common.IsHexAddress(address) {
		return &TopUpResponse{
			Status: "error",
			Error:  "Invalid Ethereum address",
		}, errors.New("invalid ethereum address")
	}

	target := common.HexToAddress(address)

	// Check balance
	balance, err := client.BalanceAt(context.Background(), target, nil)
	if err != nil {
		return &TopUpResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to fetch balance: %v", err),
		}, fmt.Errorf("failed to fetch balance: %w", err)
	}

	// If balance is sufficient, return early
	if balance.Cmp(minBalanceWei) >= 0 {
		return &TopUpResponse{
			Status:  "sufficient_balance",
			Balance: balance.String(),
		}, nil
	}

	// Check if address is eligible for top-up based on cooldown period
	canReceive, err := database.CanReceiveTopUp(address)
	if err != nil {
		return &TopUpResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to check top-up eligibility: %v", err),
		}, fmt.Errorf("failed to check top-up eligibility: %w", err)
	}

	if !canReceive {
		return &TopUpResponse{
			Status:  "cooldown_period",
			Balance: balance.String(),
			Error:   "Address has received ETH in the last 3 days",
		}, nil
	}

	// Send top-up transaction
	tx, err := sendTransaction(target)
	if err != nil {
		return &TopUpResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to send transaction: %v", err),
		}, fmt.Errorf("failed to send transaction: %w", err)
	}

	// Wait for transaction to be mined
	receipt, err := waitForTransaction(tx.Hash())
	if err != nil {
		return &TopUpResponse{
			Status: "error",
			Error:  fmt.Sprintf("Transaction sent but failed to confirm: %v", err),
		}, fmt.Errorf("transaction sent but failed to confirm: %w", err)
	}

	// Check if transaction was successful
	if receipt.Status == 0 {
		return &TopUpResponse{
			Status: "error",
			Error:  "Transaction failed on-chain",
			TxHash: tx.Hash().Hex(),
		}, fmt.Errorf("transaction failed on-chain: %s", tx.Hash().Hex())
	}

	// Save transaction to database
	err = database.SaveFaucetTransaction(address, topUpAmountWei.String(), tx.Hash().Hex(), "ETH")
	if err != nil {
		log.Printf("Warning: Transaction sent but failed to save to database: %v", err)
	}

	// Wait a moment for MetaMask and other wallets to notice the transaction
	time.Sleep(2 * time.Second)

	return &TopUpResponse{
		Status:     "topped_up",
		TxHash:     tx.Hash().Hex(),
		ToAddress:  target.Hex(),
		AmountSent: topUpAmountWei.String(),
	}, nil
}

// CheckFaucetAvailability checks if an address is eligible for a faucet top-up without actually sending ETH
func CheckFaucetAvailability(address string) (*TopUpResponse, error) {
	// Validate address
	if !common.IsHexAddress(address) {
		return &TopUpResponse{
			Status: "error",
			Error:  "Invalid Ethereum address",
		}, errors.New("invalid ethereum address")
	}

	target := common.HexToAddress(address)

	// Check balance
	balance, err := client.BalanceAt(context.Background(), target, nil)
	if err != nil {
		return &TopUpResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to fetch balance: %v", err),
		}, fmt.Errorf("failed to fetch balance: %w", err)
	}

	// If balance is sufficient, return early
	if balance.Cmp(minBalanceWei) >= 0 {
		return &TopUpResponse{
			Status:  "sufficient_balance",
			Balance: balance.String(),
		}, nil
	}

	// Check if address is eligible for top-up based on cooldown period
	canReceive, err := database.CanReceiveTopUp(address)
	if err != nil {
		return &TopUpResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to check top-up eligibility: %v", err),
		}, fmt.Errorf("failed to check top-up eligibility: %w", err)
	}

	if !canReceive {
		return &TopUpResponse{
			Status:  "cooldown_period",
			Balance: balance.String(),
			Error:   "Address has received ETH in the last 3 days",
		}, nil
	}

	// Address is eligible for top-up
	return &TopUpResponse{
		Status:  "eligible",
		Balance: balance.String(),
	}, nil
}

// sendTransaction sends ETH to the target address
func sendTransaction(target common.Address) (*types.Transaction, error) {
	// Get nonce
	nonce, err := client.PendingNonceAt(context.Background(), senderAddress)
	if err != nil {
		return nil, fmt.Errorf("failed to get nonce: %w", err)
	}

	// Get gas price
	gasPrice, err := client.SuggestGasPrice(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get gas price: %w", err)
	}

	// Create transaction data
	tx := types.NewTransaction(
		nonce,
		target,
		topUpAmountWei,
		uint64(21000), // Standard ETH transfer gas limit
		gasPrice,
		nil, // No data for simple ETH transfer
	)

	// Sign transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), senderKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send transaction
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	return signedTx, nil
}

// waitForTransaction waits for a transaction to be mined and returns its receipt
func waitForTransaction(txHash common.Hash) (*types.Receipt, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for {
		receipt, err := client.TransactionReceipt(ctx, txHash)
		if err != nil {
			if err.Error() == "not found" {
				time.Sleep(1 * time.Second)
				continue
			}
			return nil, err
		}
		return receipt, nil
	}
}

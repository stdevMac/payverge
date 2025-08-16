package faucet

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"math/rand"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

var (
	usdcContractAddress common.Address
	rpcUrl              string
	faucetPrivateKey    string
	chainId             int64
)

func GenerateCode() string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	const length = 8
	code := make([]byte, length)
	for i := range code {
		code[i] = charset[rand.Intn(len(charset))]
	}
	return string(code)
}

// USDCTransferResponse contains information about the USDC transfer transaction
type USDCTransferResponse struct {
	Status     string `json:"status"`
	TxHash     string `json:"tx_hash,omitempty"`
	ToAddress  string `json:"to_address,omitempty"`
	AmountSent string `json:"amount_sent,omitempty"`
	Error      string `json:"error,omitempty"`
}

// InitUSDCTransfer initializes the USDC transfer functionality
func InitUSDCTransfer(contractAddress string, rpcURL string, privateKey string, networkChainID int64) {
	usdcContractAddress = common.HexToAddress(contractAddress)
	rpcUrl = rpcURL
	faucetPrivateKey = privateKey
	chainId = networkChainID
	fmt.Printf("USDC contract initialized at: %s\n", usdcContractAddress.Hex())
}

// The minimal ABI for ERC20 token transfers
const erc20ABI = `[
	{
		"constant": false,
		"inputs": [
			{
				"name": "_to",
				"type": "address"
			},
			{
				"name": "_value",
				"type": "uint256"
			}
		],
		"name": "transfer",
		"outputs": [
			{
				"name": "",
				"type": "bool"
			}
		],
		"payable": false,
		"stateMutability": "nonpayable",
		"type": "function"
	}
]`

// SendUSDCWithCode sends USDC tokens to the specified address using a code
func SendUSDCWithCode(address string, amount string, code string) (*USDCTransferResponse, error) {
	// Validate address
	if !common.IsHexAddress(address) {
		return &USDCTransferResponse{
			Status: "error",
			Error:  "Invalid Ethereum address",
		}, errors.New("invalid ethereum address")
	}

	// Parse amount - USDC has 6 decimal places
	// Handle decimal input (e.g., "0.000063" should be converted to 63 base units)
	amountFloat, ok := new(big.Float).SetString(amount)
	if !ok {
		return &USDCTransferResponse{
			Status: "error",
			Error:  "Invalid amount format",
		}, errors.New("invalid amount format")
	}
	
	// USDC has 6 decimal places
	decimals := new(big.Float).SetInt(new(big.Int).Exp(big.NewInt(10), big.NewInt(6), nil))
	
	// Convert to base units by multiplying by 10^6
	amountInBaseUnits := new(big.Float).Mul(amountFloat, decimals)
	
	// Convert to integer (truncating any remaining decimals)
	amountBig := new(big.Int)
	amountInBaseUnits.Int(amountBig)

	// Send USDC transaction
	tx, err := sendUSDCTransaction(common.HexToAddress(address), amountBig)
	if err != nil {
		return &USDCTransferResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to send transaction: %v", err),
		}, fmt.Errorf("failed to send transaction: %w", err)
	}

	// We don't update the code status here - that will be done by the handler
	// to avoid import cycles

	// Log the transaction
	fmt.Printf("USDC transfer: %s tokens to %s, tx: %s\n", amount, address, tx.Hash().Hex())

	return &USDCTransferResponse{
		Status:     "success",
		TxHash:     tx.Hash().Hex(),
		ToAddress:  address,
		AmountSent: amount,
	}, nil
}

// sendUSDCTransaction sends USDC tokens to the target address
func sendUSDCTransaction(target common.Address, amount *big.Int) (*types.Transaction, error) {
	// Connect to Ethereum client
	client, err := ethclient.Dial(rpcUrl)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum: %w", err)
	}

	// Load private key
	senderKey, err := crypto.HexToECDSA(faucetPrivateKey)
	if err != nil {
		return nil, fmt.Errorf("invalid private key: %w", err)
	}

	senderAddress := crypto.PubkeyToAddress(senderKey.PublicKey)

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

	// Parse ERC20 ABI
	parsedABI, err := abi.JSON(strings.NewReader(erc20ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Prepare transaction data for ERC20 transfer
	data, err := parsedABI.Pack("transfer", target, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to pack ABI data: %w", err)
	}

	// Create transaction
	tx := types.NewTransaction(
		nonce,
		usdcContractAddress,
		big.NewInt(0),  // No ETH value for token transfers
		uint64(100000), // Gas limit for token transfer
		gasPrice,
		data,
	)

	// Get chain ID
	chainID := big.NewInt(chainId)

	// Sign the transaction
	signedTx, err := types.SignTx(tx, types.NewEIP155Signer(chainID), senderKey)
	if err != nil {
		return nil, fmt.Errorf("failed to sign transaction: %w", err)
	}

	// Send the transaction
	err = client.SendTransaction(context.Background(), signedTx)
	if err != nil {
		return nil, fmt.Errorf("failed to send transaction: %w", err)
	}

	return signedTx, nil
}

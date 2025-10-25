package faucet

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math/big"
	"strings"
	"time"

	"payverge/internal/database"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

var (
	// Testnet-specific configuration
	testnetTopUpAmountWei  = big.NewInt(1000000000000000) // 0.001 ETH in Wei
	testnetMinBalanceWei   = big.NewInt(500000000000000)  // 0.0005 ETH in Wei
	testnetUsdcTopUpAmount = big.NewInt(1000000000)       // 1000 USDC (6 decimals)
	testnetUsdcMinBalance  = big.NewInt(100000000)        // 100 USDC (6 decimals)

	// Base Sepolia USDC contract address
	testnetUsdcContractAddress = common.HexToAddress("0x82d491aB292C06Aa7148234b910cdea5FE788223")

	// ERC20 transfer function signature
	testnetErc20ABI = `[{"constant":false,"inputs":[{"name":"_to","type":"address"},{"name":"_value","type":"uint256"}],"name":"transfer","outputs":[{"name":"","type":"bool"}],"payable":false,"stateMutability":"nonpayable","type":"function"},{"constant":true,"inputs":[{"name":"_owner","type":"address"}],"name":"balanceOf","outputs":[{"name":"balance","type":"uint256"}],"payable":false,"stateMutability":"view","type":"function"}]`
)

// TestnetFaucetResponse contains information about the testnet faucet transaction
type TestnetFaucetResponse struct {
	Status      string `json:"status"`
	EthTxHash   string `json:"eth_tx_hash,omitempty"`
	UsdcTxHash  string `json:"usdc_tx_hash,omitempty"`
	ToAddress   string `json:"to_address,omitempty"`
	EthSent     string `json:"eth_sent,omitempty"`
	UsdcSent    string `json:"usdc_sent,omitempty"`
	EthBalance  string `json:"eth_balance,omitempty"`
	UsdcBalance string `json:"usdc_balance,omitempty"`
	Error       string `json:"error,omitempty"`
	Message     string `json:"message,omitempty"`
}

// CheckTestnetEligibility checks if an address is eligible for testnet faucet without sending tokens
func CheckTestnetEligibility(address string) (*TestnetFaucetResponse, error) {
	// Validate address
	if !common.IsHexAddress(address) {
		return &TestnetFaucetResponse{
			Status: "error",
			Error:  "Invalid Ethereum address",
		}, errors.New("invalid ethereum address")
	}

	target := common.HexToAddress(address)

	// Check ETH balance
	ethBalance, err := client.BalanceAt(context.Background(), target, nil)
	if err != nil {
		return &TestnetFaucetResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to fetch ETH balance: %v", err),
		}, fmt.Errorf("failed to fetch ETH balance: %w", err)
	}

	// Check USDC balance
	usdcBalance, err := getUSDCBalance(target)
	if err != nil {
		return &TestnetFaucetResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to fetch USDC balance: %v", err),
		}, fmt.Errorf("failed to fetch USDC balance: %w", err)
	}

	// Check cooldown period
	canReceive, err := database.CanReceiveTopUp(address)
	if err != nil {
		return &TestnetFaucetResponse{
			Status: "error",
			Error:  fmt.Sprintf("Failed to check eligibility: %v", err),
		}, fmt.Errorf("failed to check eligibility: %w", err)
	}

	if !canReceive {
		return &TestnetFaucetResponse{
			Status:      "cooldown_period",
			EthBalance:  ethBalance.String(),
			UsdcBalance: usdcBalance.String(),
			Error:       "Address has received tokens in the last 24 hours",
		}, nil
	}

	// Determine what needs to be sent
	needsEth := ethBalance.Cmp(testnetMinBalanceWei) < 0
	needsUsdc := usdcBalance.Cmp(testnetUsdcMinBalance) < 0

	if !needsEth && !needsUsdc {
		return &TestnetFaucetResponse{
			Status:      "sufficient_balance",
			EthBalance:  ethBalance.String(),
			UsdcBalance: usdcBalance.String(),
			Message:     "Address has sufficient ETH and USDC for testing",
		}, nil
	}

	message := "Address is eligible for: "
	if needsEth {
		message += "0.001 ETH"
	}
	if needsUsdc {
		if needsEth {
			message += " and "
		}
		message += "1000 USDC"
	}

	return &TestnetFaucetResponse{
		Status:      "eligible",
		EthBalance:  ethBalance.String(),
		UsdcBalance: usdcBalance.String(),
		Message:     message,
	}, nil
}

// TestnetFaucetTopUp sends ETH and/or USDC to the address based on their current balances
func TestnetFaucetTopUp(address string) (*TestnetFaucetResponse, error) {
	// First check eligibility
	eligibility, err := CheckTestnetEligibility(address)
	if err != nil {
		return eligibility, err
	}

	if eligibility.Status != "eligible" {
		return eligibility, nil
	}

	target := common.HexToAddress(address)
	response := &TestnetFaucetResponse{
		Status:    "processing",
		ToAddress: target.Hex(),
	}

	// Check what needs to be sent
	ethBalance, _ := new(big.Int).SetString(eligibility.EthBalance, 10)
	usdcBalance, _ := new(big.Int).SetString(eligibility.UsdcBalance, 10)

	needsEth := ethBalance.Cmp(testnetMinBalanceWei) < 0
	needsUsdc := usdcBalance.Cmp(testnetUsdcMinBalance) < 0

	var ethTx, usdcTx *types.Transaction

	// Send ETH if needed
	if needsEth {
		ethTx, err = sendETHTransaction(target, testnetTopUpAmountWei)
		if err != nil {
			response.Status = "error"
			response.Error = fmt.Sprintf("Failed to send ETH: %v", err)
			return response, fmt.Errorf("failed to send ETH: %w", err)
		}
		response.EthTxHash = ethTx.Hash().Hex()
		response.EthSent = testnetTopUpAmountWei.String()
	}

	// Send USDC if needed
	if needsUsdc {
		usdcTx, err = sendTestnetUSDCTransaction(target, testnetUsdcTopUpAmount)
		if err != nil {
			response.Status = "error"
			response.Error = fmt.Sprintf("Failed to send USDC: %v", err)
			return response, fmt.Errorf("failed to send USDC: %w", err)
		}
		response.UsdcTxHash = usdcTx.Hash().Hex()
		response.UsdcSent = testnetUsdcTopUpAmount.String()
	}

	// Wait for transactions to be mined
	if ethTx != nil {
		_, err = waitForTransaction(ethTx.Hash())
		if err != nil {
			response.Status = "error"
			response.Error = fmt.Sprintf("ETH transaction failed to confirm: %v", err)
			return response, fmt.Errorf("ETH transaction failed to confirm: %w", err)
		}
	}

	if usdcTx != nil {
		_, err = waitForTransaction(usdcTx.Hash())
		if err != nil {
			response.Status = "error"
			response.Error = fmt.Sprintf("USDC transaction failed to confirm: %v", err)
			return response, fmt.Errorf("USDC transaction failed to confirm: %w", err)
		}
	}

	// Save transactions to database
	if ethTx != nil {
		err = database.SaveFaucetTransaction(address, testnetTopUpAmountWei.String(), ethTx.Hash().Hex(), "ETH")
		if err != nil {
			log.Printf("Warning: ETH transaction sent but failed to save to database: %v", err)
		}
	}

	if usdcTx != nil {
		err = database.SaveFaucetTransaction(address, testnetUsdcTopUpAmount.String(), usdcTx.Hash().Hex(), "USDC")
		if err != nil {
			log.Printf("Warning: USDC transaction sent but failed to save to database: %v", err)
		}
	}

	// Wait for wallets to notice the transactions
	time.Sleep(3 * time.Second)

	response.Status = "completed"
	response.Message = "Testnet tokens sent successfully! You can now test the Payverge platform."

	return response, nil
}

// getUSDCBalance gets the USDC balance for an address
func getUSDCBalance(address common.Address) (*big.Int, error) {
	// Parse the ERC20 ABI
	parsedABI, err := abi.JSON(strings.NewReader(testnetErc20ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Pack the balanceOf function call
	data, err := parsedABI.Pack("balanceOf", address)
	if err != nil {
		return nil, fmt.Errorf("failed to pack balanceOf call: %w", err)
	}

	// Create the call message
	msg := ethereum.CallMsg{
		To:   &testnetUsdcContractAddress,
		Data: data,
	}

	// Make the call
	result, err := client.CallContract(context.Background(), msg, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %w", err)
	}

	// Unpack the result
	var balance *big.Int
	err = parsedABI.UnpackIntoInterface(&balance, "balanceOf", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %w", err)
	}

	return balance, nil
}

// sendETHTransaction sends ETH to the target address
func sendETHTransaction(target common.Address, amount *big.Int) (*types.Transaction, error) {
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

	// Create transaction
	tx := types.NewTransaction(
		nonce,
		target,
		amount,
		uint64(21000), // Standard ETH transfer gas limit
		gasPrice,
		nil,
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

// sendTestnetUSDCTransaction sends USDC to the target address
func sendTestnetUSDCTransaction(target common.Address, amount *big.Int) (*types.Transaction, error) {
	// Parse the ERC20 ABI
	parsedABI, err := abi.JSON(strings.NewReader(testnetErc20ABI))
	if err != nil {
		return nil, fmt.Errorf("failed to parse ABI: %w", err)
	}

	// Pack the transfer function call
	data, err := parsedABI.Pack("transfer", target, amount)
	if err != nil {
		return nil, fmt.Errorf("failed to pack transfer call: %w", err)
	}

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

	// Create transaction
	tx := types.NewTransaction(
		nonce,
		testnetUsdcContractAddress,
		big.NewInt(0),  // No ETH value for ERC20 transfer
		uint64(100000), // Higher gas limit for contract interaction
		gasPrice,
		data,
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

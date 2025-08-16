package services

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

type BlockchainService struct {
	client          *ethclient.Client
	contractAddress common.Address
	contractABI     abi.ABI
	privateKey      *ecdsa.PrivateKey
	auth            *bind.TransactOpts
}

// Contract ABI (simplified for key functions and events)
const contractABIJSON = `[
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "name": "invoiceId", "type": "uint256"},
			{"indexed": true, "name": "creator", "type": "address"},
			{"indexed": false, "name": "amount", "type": "uint256"},
			{"indexed": false, "name": "metadataURI", "type": "string"},
			{"indexed": false, "name": "timestamp", "type": "uint256"}
		],
		"name": "InvoiceCreated",
		"type": "event"
	},
	{
		"anonymous": false,
		"inputs": [
			{"indexed": true, "name": "invoiceId", "type": "uint256"},
			{"indexed": true, "name": "payer", "type": "address"},
			{"indexed": false, "name": "amount", "type": "uint256"},
			{"indexed": false, "name": "platformFee", "type": "uint256"},
			{"indexed": false, "name": "creatorAmount", "type": "uint256"},
			{"indexed": false, "name": "timestamp", "type": "uint256"}
		],
		"name": "InvoicePaid",
		"type": "event"
	},
	{
		"inputs": [
			{"name": "amount", "type": "uint256"},
			{"name": "metadataURI", "type": "string"}
		],
		"name": "createInvoice",
		"outputs": [{"name": "", "type": "uint256"}],
		"stateMutability": "nonpayable",
		"type": "function"
	},
	{
		"inputs": [{"name": "invoiceId", "type": "uint256"}],
		"name": "getInvoice",
		"outputs": [
			{"name": "id", "type": "uint256"},
			{"name": "creator", "type": "address"},
			{"name": "amount", "type": "uint256"},
			{"name": "metadataURI", "type": "string"},
			{"name": "amountPaid", "type": "uint256"},
			{"name": "isActive", "type": "bool"},
			{"name": "createdAt", "type": "uint256"}
		],
		"stateMutability": "view",
		"type": "function"
	}
]`

func NewBlockchainService() (*BlockchainService, error) {
	// Connect to Ethereum client
	rpcURL := os.Getenv("ETHEREUM_RPC_URL")
	if rpcURL == "" {
		return nil, fmt.Errorf("ETHEREUM_RPC_URL not set")
	}

	client, err := ethclient.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to Ethereum client: %v", err)
	}

	// Load contract address
	contractAddr := os.Getenv("CONTRACT_ADDRESS")
	if contractAddr == "" {
		return nil, fmt.Errorf("CONTRACT_ADDRESS not set")
	}

	// Parse contract ABI
	contractABI, err := abi.JSON(strings.NewReader(contractABIJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to parse contract ABI: %v", err)
	}

	// Load private key (for admin functions if needed)
	privateKeyHex := os.Getenv("PRIVATE_KEY")
	var privateKey *ecdsa.PrivateKey
	var auth *bind.TransactOpts

	if privateKeyHex != "" {
		privateKey, err = crypto.HexToECDSA(privateKeyHex)
		if err != nil {
			log.Printf("Warning: failed to load private key: %v", err)
		} else {
			chainID, err := client.NetworkID(context.Background())
			if err != nil {
				return nil, fmt.Errorf("failed to get network ID: %v", err)
			}

			auth, err = bind.NewKeyedTransactorWithChainID(privateKey, chainID)
			if err != nil {
				return nil, fmt.Errorf("failed to create transactor: %v", err)
			}
		}
	}

	return &BlockchainService{
		client:          client,
		contractAddress: common.HexToAddress(contractAddr),
		contractABI:     contractABI,
		privateKey:      privateKey,
		auth:            auth,
	}, nil
}

func (bs *BlockchainService) GetInvoiceFromContract(invoiceID uint64) (*ContractInvoice, error) {
	// Create call options for future use
	_ = &bind.CallOpts{
		Context: context.Background(),
	}

	// Pack the function call
	data, err := bs.contractABI.Pack("getInvoice", big.NewInt(int64(invoiceID)))
	if err != nil {
		return nil, fmt.Errorf("failed to pack function call: %v", err)
	}

	// Make the call
	result, err := bs.client.CallContract(context.Background(), ethereum.CallMsg{
		To:   &bs.contractAddress,
		Data: data,
	}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to call contract: %v", err)
	}

	// Unpack the result
	var invoice ContractInvoice
	err = bs.contractABI.UnpackIntoInterface(&invoice, "getInvoice", result)
	if err != nil {
		return nil, fmt.Errorf("failed to unpack result: %v", err)
	}

	return &invoice, nil
}

func (bs *BlockchainService) SubscribeToEvents(eventChan chan<- ContractEvent) error {
	// Create filter query for contract events
	query := ethereum.FilterQuery{
		Addresses: []common.Address{bs.contractAddress},
	}

	// Subscribe to logs
	logs := make(chan types.Log)
	sub, err := bs.client.SubscribeFilterLogs(context.Background(), query, logs)
	if err != nil {
		return fmt.Errorf("failed to subscribe to logs: %v", err)
	}

	go func() {
		defer sub.Unsubscribe()

		for {
			select {
			case err := <-sub.Err():
				log.Printf("Subscription error: %v", err)
				return
			case vLog := <-logs:
				event, err := bs.parseLogEvent(vLog)
				if err != nil {
					log.Printf("Failed to parse log event: %v", err)
					continue
				}

				if event != nil {
					eventChan <- *event
				}
			}
		}
	}()

	return nil
}

func (bs *BlockchainService) parseLogEvent(vLog types.Log) (*ContractEvent, error) {
	switch vLog.Topics[0] {
	case bs.contractABI.Events["InvoiceCreated"].ID:
		return bs.parseInvoiceCreatedEvent(vLog)
	case bs.contractABI.Events["InvoicePaid"].ID:
		return bs.parseInvoicePaidEvent(vLog)
	default:
		return nil, nil // Unknown event, skip
	}
}

func (bs *BlockchainService) parseInvoiceCreatedEvent(vLog types.Log) (*ContractEvent, error) {
	var event struct {
		InvoiceId   *big.Int
		Creator     common.Address
		Amount      *big.Int
		MetadataURI string
		Timestamp   *big.Int
	}

	err := bs.contractABI.UnpackIntoInterface(&event, "InvoiceCreated", vLog.Data)
	if err != nil {
		return nil, err
	}

	// Extract indexed parameters
	event.InvoiceId = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
	event.Creator = common.BytesToAddress(vLog.Topics[2].Bytes())

	return &ContractEvent{
		Type:        "InvoiceCreated",
		InvoiceID:   event.InvoiceId.Uint64(),
		Creator:     event.Creator.Hex(),
		Amount:      event.Amount.Uint64(),
		MetadataURI: event.MetadataURI,
		Timestamp:   event.Timestamp.Uint64(),
		TxHash:      vLog.TxHash.Hex(),
		BlockNumber: vLog.BlockNumber,
	}, nil
}

func (bs *BlockchainService) parseInvoicePaidEvent(vLog types.Log) (*ContractEvent, error) {
	var event struct {
		InvoiceId     *big.Int
		Payer         common.Address
		Amount        *big.Int
		PlatformFee   *big.Int
		CreatorAmount *big.Int
		Timestamp     *big.Int
	}

	err := bs.contractABI.UnpackIntoInterface(&event, "InvoicePaid", vLog.Data)
	if err != nil {
		return nil, err
	}

	// Extract indexed parameters
	event.InvoiceId = new(big.Int).SetBytes(vLog.Topics[1].Bytes())
	event.Payer = common.BytesToAddress(vLog.Topics[2].Bytes())

	return &ContractEvent{
		Type:          "InvoicePaid",
		InvoiceID:     event.InvoiceId.Uint64(),
		Payer:         event.Payer.Hex(),
		Amount:        event.Amount.Uint64(),
		PlatformFee:   event.PlatformFee.Uint64(),
		CreatorAmount: event.CreatorAmount.Uint64(),
		Timestamp:     event.Timestamp.Uint64(),
		TxHash:        vLog.TxHash.Hex(),
		BlockNumber:   vLog.BlockNumber,
	}, nil
}

// Data structures
type ContractInvoice struct {
	ID          *big.Int
	Creator     common.Address
	Amount      *big.Int
	MetadataURI string
	AmountPaid  *big.Int
	IsActive    bool
	CreatedAt   *big.Int
}

type ContractEvent struct {
	Type          string
	InvoiceID     uint64
	Creator       string
	Payer         string
	Amount        uint64
	PlatformFee   uint64
	CreatorAmount uint64
	MetadataURI   string
	Timestamp     uint64
	TxHash        string
	BlockNumber   uint64
}

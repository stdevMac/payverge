package database

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	faucetCollectionName = "faucet_transactions"
	topUpCooldownDays    = 3 // Days between allowed top-ups
)

// FaucetTransaction represents a record of ETH being sent to an address
type FaucetTransaction struct {
	Address   string    `bson:"address"`
	Amount    string    `bson:"amount"`
	TxHash    string    `bson:"tx_hash"`
	Timestamp time.Time `bson:"timestamp"`
	Ticker    string    `bson:"ticker"`
}

// GetFaucetCollection returns the MongoDB collection for faucet transactions
func GetFaucetCollection() *mongo.Collection {
	return mongoClient.Database(dbName).Collection(faucetCollectionName)
}

// SaveFaucetTransaction records a new faucet transaction in the database
func SaveFaucetTransaction(address, amount, txHash, ticker string) error {
	collection := GetFaucetCollection()

	transaction := FaucetTransaction{
		Address:   address,
		Amount:    amount,
		TxHash:    txHash,
		Timestamp: time.Now(),
		Ticker:    ticker,
	}

	_, err := collection.InsertOne(context.Background(), transaction)
	return err
}

// CanReceiveTopUp checks if an address is eligible to receive a top-up
// based on the cooldown period
func CanReceiveTopUp(address string) (bool, error) {
	collection := GetFaucetCollection()

	// Find the most recent transaction for this address
	filter := bson.M{"address": address}
	opts := options.FindOne().SetSort(bson.D{{Key: "timestamp", Value: -1}})

	var lastTx FaucetTransaction
	err := collection.FindOne(context.Background(), filter, opts).Decode(&lastTx)

	// If no record found, they've never received a top-up before
	if errors.Is(err, mongo.ErrNoDocuments) {
		return true, nil
	}

	if err != nil {
		return false, err
	}

	// Check if enough time has passed since the last top-up
	cooldownPeriod := time.Hour * 24 * topUpCooldownDays
	return time.Since(lastTx.Timestamp) > cooldownPeriod, nil
}

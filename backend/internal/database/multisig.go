package database

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const multisigTxCollectionName = "multisig_tx"

type multisigTx struct {
	ID   int                    `bson:"_id"`
	Data map[string]interface{} `bson:"data"`
}

// GetMultisigTx retrieves the stored multisig transaction data
func GetMultisigTx() (map[string]interface{}, error) {
	collection := mongoClient.Database(dbName).Collection(multisigTxCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var tx multisigTx
	err := collection.FindOne(ctx, bson.M{"_id": 1}).Decode(&tx)
	if err != nil {
		return nil, err
	}
	return tx.Data, nil
}

// StoreMultisigTx stores new multisig transaction data
func StoreMultisigTx(data map[string]interface{}) error {
	if data == nil {
		return errors.New("data cannot be nil")
	}

	collection := mongoClient.Database(dbName).Collection(multisigTxCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Update().SetUpsert(true)
	_, err := collection.UpdateOne(
		ctx,
		bson.M{"_id": 1},
		bson.M{"$set": bson.M{"data": data}},
		opts,
	)
	return err
}

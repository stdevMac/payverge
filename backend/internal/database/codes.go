package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

const (
	codesCollectionName = "codes"
)

type Codes struct {
	Code      string    `json:"code" bson:"code"`
	Address   string    `json:"address" bson:"address"`
	Amount    string    `json:"amount" bson:"amount"`
	Used      bool      `json:"used" bson:"used"`
	Expiry    string    `json:"expiry" bson:"expiry"`
	ClaimedAt time.Time `json:"claim_date" bson:"claimedAt"`
}

// GetCodesCollection returns the MongoDB collection for codes
func GetCodesCollection() *mongo.Collection {
	return mongoClient.Database(dbName).Collection(codesCollectionName)
}

// SaveCode records a new code in the database
func SaveCode(code *Codes) error {
	collection := GetCodesCollection()

	_, err := collection.InsertOne(context.Background(), code)
	return err
}

// GetCode retrieves a code by its code
func GetCode(code string) (*Codes, error) {
	collection := GetCodesCollection()

	var codeTransaction Codes
	err := collection.FindOne(context.Background(), bson.M{"code": code}).Decode(&codeTransaction)
	if err != nil {
		return nil, err
	}

	return &codeTransaction, nil
}

// UpdateCode updates the status of a code
func UpdateCode(code *Codes) error {
	collection := GetCodesCollection()

	filter := bson.M{"code": code.Code}

	// Use $set operator to update the document fields
	update := bson.M{"$set": bson.M{
		"amount":    code.Amount,
		"used":      code.Used,
		"expiry":    code.Expiry,
		"address":   code.Address,
		"claimedAt": code.ClaimedAt,
	}}

	_, err := collection.UpdateOne(context.Background(), filter, update)
	return err
}

// DeleteCode deletes a code from the database
func DeleteCode(code string) error {
	collection := GetCodesCollection()

	_, err := collection.DeleteOne(context.Background(), bson.M{"code": code})
	return err
}

// GetAllCodes retrieves all codes from the database
func GetAllCodes() ([]Codes, error) {
	collection := GetCodesCollection()

	cursor, err := collection.Find(context.Background(), bson.M{})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.Background())

	var codes []Codes
	for cursor.Next(context.Background()) {
		var code Codes
		if err := cursor.Decode(&code); err != nil {
			return nil, err
		}
		codes = append(codes, code)
	}

	if err := cursor.Err(); err != nil {
		return nil, err
	}

	return codes, nil
}

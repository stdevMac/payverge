package database

import (
	"context"
	"time"

	"web3-boilerplate/internal/structs"
	"go.mongodb.org/mongo-driver/bson"
)

func AddSubscriber(sub structs.Subscriber) error {
	collection := mongoClient.Database(dbName).Collection("subscribers")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if subscriber already exists
	var existingSub structs.Subscriber
	err := collection.FindOne(ctx, bson.M{"email": sub.Email}).Decode(&existingSub)
	if err == nil {
		// Subscriber already exists
		return nil
	}

	// Insert new subscriber
	_, err = collection.InsertOne(ctx, sub)
	if err != nil {
		return err
	}

	return nil
}

func GetSubscribers() ([]structs.Subscriber, error) {
	collection := mongoClient.Database(dbName).Collection("subscribers")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	var subscribers []structs.Subscriber
	if err = cursor.All(ctx, &subscribers); err != nil {
		return nil, err
	}

	return subscribers, nil
}

func UnSubscribe(email string) error {
	collection := mongoClient.Database(dbName).Collection("subscribers")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.DeleteOne(ctx, bson.M{"email": email})
	return err
}

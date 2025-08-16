package database

import (
	"context"
	"log"
	"time"

	"github.com/stdevMac/shares/internal/structs"
	"go.mongodb.org/mongo-driver/bson"
)

func RegisterUser(user structs.User) error {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.InsertOne(ctx, user)
	return err
}

func GetUserByAddress(address string) (structs.User, error) {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user structs.User
	err := collection.FindOne(ctx, bson.M{"address": address}).Decode(&user)
	if err != nil {
		return structs.User{}, err
	}

	return user, nil
}

func GetUserByReferralCode(referralCode string) (structs.User, error) {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user structs.User
	err := collection.FindOne(ctx, bson.M{"referral_code": referralCode}).Decode(&user)
	if err != nil {
		return structs.User{}, err
	}

	return user, nil
}

func GetUserByTokenId(tokenId string) (structs.User, error) {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var user structs.User
	err := collection.FindOne(ctx, bson.M{"token_id": tokenId}).Decode(&user)
	if err != nil {
		return structs.User{}, err
	}

	return user, nil

}

func UpdateUser(user structs.User) error {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := collection.ReplaceOne(ctx, bson.M{"address": user.Address}, user)
	return err
}

func GetAllUsers() ([]structs.User, error) {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	var users []structs.User
	if err = cursor.All(ctx, &users); err != nil {
		return nil, err
	}

	return users, nil
}

func UpdateNotificationPreferences(user structs.User, preferences structs.NotificationPreferences) error {
	user.NotificationPreferences = preferences

	return UpdateUser(user)
}

// SetDefaultNotificationPreferences sets default notification preferences for all users that don't have them set
func SetDefaultNotificationPreferences() error {
	collection := mongoClient.Database(dbName).Collection(usersCollectionName)

	// Default notification preferences with all fields set to true
	defaultPreferences := structs.NotificationPreferences{
		EmailEnabled:         true,
		NewsEnabled:          true,
		UpdatesEnabled:       true,
		TransactionalEnabled: true,
		SecurityEnabled:      true,
		ReportsEnabled:       true,
		StatisticsEnabled:    true,
	}

	// Update all users that don't have notification preferences set
	withoutNotificationPreferences := bson.M{
		"notification_preferences": bson.M{"$exists": false},
	}

	update := bson.M{
		"$set": bson.M{
			"notification_preferences": defaultPreferences,
		},
	}

	result, err := collection.UpdateMany(context.Background(), withoutNotificationPreferences, update)
	if err != nil {
		return err
	}

	log.Printf("Updated notification preferences for %d users", result.ModifiedCount)
	return nil
}

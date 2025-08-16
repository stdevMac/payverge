package database

import (
	"context"
	"log"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
)

type DbConfig struct {
	URI      string `json:"uri"`
	Username string `json:"username"`
	Password string `json:"password"`
}

func NewConfig(uri, username, password string) *DbConfig {
	return &DbConfig{
		URI:      uri,
		Username: username,
		Password: password,
	}
}

type DBClient *mongo.Client

var (
	dbName                   = "database"
	usersCollectionName      = "users"
	operationsCollectionName = "operations"
	errorLogsCollectionName  = "error_logs"
	subscribersCollectionName = "subscribers"

	mongoClient *mongo.Client
)

func InitDB(config *DbConfig) {
	clientOptions := options.Client().ApplyURI(config.URI)

	// Add the authentication credentials
	clientOptions.Auth = &options.Credential{
		Username: config.Username,
		Password: config.Password,
	}
	client, err := mongo.Connect(context.Background(), clientOptions)
	if err != nil {
		log.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err = client.Ping(ctx, readpref.Primary()); err != nil {
		log.Fatal(err)
	}

	mongoClient = client
}

// GetClient returns the MongoDB client instance
func GetClient() *mongo.Client {
	return mongoClient
}

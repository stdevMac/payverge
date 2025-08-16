package database

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ErrorLog represents an error log entry in the database
type ErrorLog struct {
	ID            string                 `bson:"_id,omitempty"`
	Timestamp     time.Time             `bson:"timestamp"`
	Error         string                 `bson:"error"`
	Component     string                 `bson:"component"`
	Function      string                 `bson:"function"`
	AdditionalInfo map[string]interface{} `bson:"additional_info,omitempty"`
	CreatedAt     time.Time             `bson:"created_at"`
}

// StoreErrorLog stores an error log in the database
func StoreErrorLog(ctx context.Context, log *ErrorLog) error {
	collection := mongoClient.Database(dbName).Collection(errorLogsCollectionName)
	
	// Ensure we have an index on timestamp for efficient querying
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "timestamp", Value: -1}},
		Options: options.Index().SetBackground(true),
	}
	
	if _, err := collection.Indexes().CreateOne(ctx, indexModel); err != nil {
		return err
	}

	_, err := collection.InsertOne(ctx, log)
	return err
}

// GetErrorLogs retrieves error logs within a time range
func GetErrorLogs(ctx context.Context, startTime, endTime time.Time) ([]*ErrorLog, error) {
	collection := mongoClient.Database(dbName).Collection(errorLogsCollectionName)
	
	filter := bson.M{
		"timestamp": bson.M{
			"$gte": startTime,
			"$lte": endTime,
		},
	}
	
	opts := options.Find().SetSort(bson.D{{Key: "timestamp", Value: -1}})
	
	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var logs []*ErrorLog
	if err = cursor.All(ctx, &logs); err != nil {
		return nil, err
	}

	return logs, nil
}

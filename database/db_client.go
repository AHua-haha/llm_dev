package database

import (
	"context"
	"log"
	"os"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client
var database *mongo.Database
var err error

// InitDB initializes the MongoDB connection
func InitDB() {
	// Read MongoDB URI from environment variable
	uri := os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017" // Default URI
	}

	// Set client options
	clientOptions := options.Client().ApplyURI(uri)

	// Create a new client and connect to MongoDB
	client, err = mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatalf("Error connecting to MongoDB: %v", err)
	}

	// Wait for a connection to be established
	err = client.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatalf("Error pinging MongoDB: %v", err)
	}

	// Select the database (replace "mydb" with your database name)
	database = client.Database("mydb")

	log.Println("Successfully connected to MongoDB!")
}

// GetDB returns the database instance
func GetDB() *mongo.Database {
	return database
}

// CloseDB gracefully closes the MongoDB connection
func CloseDB() {
	if err := client.Disconnect(context.TODO()); err != nil {
		log.Fatalf("Error closing MongoDB connection: %v", err)
	}
	log.Println("MongoDB connection closed.")
}

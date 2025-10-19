package database

import (
	"context"
	"os"

	_ "llm_dev/utils"

	"github.com/rs/zerolog/log"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var client *mongo.Client
var uri string

// InitDB initializes the MongoDB connection
func InitDB() {
	// Read MongoDB URI from environment variable
	uri = os.Getenv("MONGO_URI")
	if uri == "" {
		uri = "mongodb://localhost:27017" // Default URI
	}

	clientOptions := options.Client().ApplyURI(uri)
	c, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Fatal().Err(err).Str("url", uri).Msg("connect to mongodb fail")
	}

	err = c.Ping(context.TODO(), nil)
	if err != nil {
		log.Fatal().Err(err).Str("url", uri).Msg("ping mongodb fail")
	}
	client = c
	log.Info().Str("uri", uri).Msg("connect to mongodb")
}

func GetDBClient() *mongo.Client {
	if client == nil {
		log.Fatal().Msg("db client not init")
	}
	return client
}

func CloseDB() {
	if err := client.Disconnect(context.TODO()); err != nil {
		log.Fatal().Err(err).Msg("disconnect mongodb fail")
	}
	log.Info().Str("uri", uri).Msg("disconnect to mongodb")
}

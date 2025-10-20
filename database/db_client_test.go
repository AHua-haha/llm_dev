package database_test

import (
	"context"
	"llm_dev/database"
	_ "llm_dev/utils"
	"testing"

	"go.mongodb.org/mongo-driver/bson"
)

func TestDBConnect(t *testing.T) {
	t.Run("test db connect", func(t *testing.T) {
		database.InitDB()
		client := database.GetDBClient()
		db := client.Database("llm_dev")
		collection := db.Collection("Defs")
		collection.InsertOne(context.TODO(), bson.M{"name": "hello"})
		database.CloseDB()
	})
}

type Test struct {
	A string
	B int
}

func TestInsetFind(t *testing.T) {
	t.Run("test db connect", func(t *testing.T) {
		database.InitDB()
		client := database.GetDBClient()
		db := client.Database("llm_dev")
		collection := db.Collection("Defs")
		b := Test{
			A: "hello",
			B: 22,
		}
		collection.InsertOne(context.TODO(), b)
		database.CloseDB()
	})
}

package database_test

import (
	"context"
	"fmt"
	"llm_dev/database"
	_ "llm_dev/utils"
	"testing"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
)

func TestDBConnect(t *testing.T) {
	t.Run("test db connect", func(t *testing.T) {
		database.InitDB()
		client := database.GetDBClient()
		res, err := client.ListDatabases(context.TODO(), bson.M{})
		db := client.Database("llm_dev")
		fmt.Printf("db.Name(): %v\n", db.Name())
		if err != nil {
			log.Error().Err(err)
		}
		for _, elem := range res.Databases {
			fmt.Printf("elem.Name: %v\n", elem.Name)
		}
		database.CloseDB()
	})
}

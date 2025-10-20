package database

import "go.mongodb.org/mongo-driver/bson"

type DataBase struct {
}

type FilterOp string

const (
	All FilterOp = "$all"
	In  FilterOp = "$in"
	Nin FilterOp = "$nin"
	Gt  FilterOp = "$gt"
	Eq  FilterOp = "$eq"
)

type MongoDBFilterBuilder struct {
	filter bson.M
}

func NewFilter(key string, value any) MongoDBFilterBuilder {
	var builder MongoDBFilterBuilder
	builder.AddKV(key, value)
	return builder
}

func (builder *MongoDBFilterBuilder) build() bson.M {
	return builder.filter
}
func (builder *MongoDBFilterBuilder) AddKV(key string, value any) *MongoDBFilterBuilder {
	builder.filter[key] = value
	return builder
}
func (builder *MongoDBFilterBuilder) AddFilter(key string, f MongoDBFilterBuilder) *MongoDBFilterBuilder {
	builder.filter[key] = f.build()
	return builder
}

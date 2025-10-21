package database

import "go.mongodb.org/mongo-driver/bson"

type DataBase struct {
}

const (
	All  string = "$all"
	In   string = "$in"
	Nin  string = "$nin"
	Gt   string = "$gt"
	Eq   string = "$eq"
	Ne   string = "$ne"
	Expr string = "$expr"
)

type MongoDBFilterBuilder struct {
	filter bson.M
}

func NewFilter() MongoDBFilterBuilder {
	return MongoDBFilterBuilder{
		filter: bson.M{},
	}
}

func NewFilterKV(key string, value any) MongoDBFilterBuilder {
	builder := NewFilter()
	builder.AddKV(key, value)
	return builder
}

func (builder *MongoDBFilterBuilder) Build() bson.M {
	return builder.filter
}
func (builder *MongoDBFilterBuilder) AddKV(key string, value any) *MongoDBFilterBuilder {
	builder.filter[key] = value
	return builder
}
func (builder *MongoDBFilterBuilder) AddFilter(key string, f MongoDBFilterBuilder) *MongoDBFilterBuilder {
	builder.filter[key] = f.Build()
	return builder
}

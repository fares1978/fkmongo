package fkmongo

import (
	"go.mongodb.org/mongo-driver/v2/bson"
)

// TypeToBson converts a value (usually a struct) to a bson.M using its bson tags.
// When the result is used as a $set, fields without `omitempty` overwrite
// stored values with their zero value.
func TypeToBson[T any](t T) (bson.M, error) {
	doc, err := bson.Marshal(t)
	if err != nil {
		return nil, err
	}
	m := bson.M{}
	if err := bson.Unmarshal(doc, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// StrToObjectID parses a 24-character hex string into an ObjectID.
func StrToObjectID(idStr string) (bson.ObjectID, error) {
	return bson.ObjectIDFromHex(idStr)
}

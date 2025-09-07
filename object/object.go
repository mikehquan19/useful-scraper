package object

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Area struct {
	Unit  string
	Value float32
}

type Price struct {
	Unit  string
	Value float32
}

// Home info data to be uploaded to the Mongo database
type RedfinHomeInfo struct {
	Id           primitive.ObjectID
	Address      string
	Description  string
	Bedrooms     float32
	Bathrooms    float32
	HomeArea     Area
	Price        float32
	PropertyType string
	YearBuilt    int32
	PricePerUnit float32
	LotArea      Area
	HOADues      float32
	Parking      string
	Url          string
}

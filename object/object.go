package object

import (
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Area struct {
	Unit  string  `json:"unit" bson:"unit"`
	Value float32 `json:"value" bson:"value"`
}

type Price struct {
	Unit  string  `json:"unit" bson:"unit"`
	Value float32 `json:"value" bson:"value"`
}

// Home info data to be uploaded to the Mongo database
type RedfinHomeInfo struct {
	Id           primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Address      string             `json:"address" bson:"address"`
	Description  string             `json:"description" bson:"description"`
	Bedrooms     float32            `json:"bedrooms" bson:"bedrooms"`
	Bathrooms    float32            `json:"bathrooms" bson:"bathrooms"`
	HomeArea     Area               `json:"home_area" bson:"home_area"`
	Price        float32            `json:"price" bson:"price"`
	PropertyType string             `json:"property_type" bson:"property_type"`
	YearBuilt    int32              `json:"year_built" bson:"year_built"`
	PricePerUnit float32            `json:"price_per_unit" bson:"price_per_unit"`
	LotArea      Area               `json:"lot_area" bson:"lot_area"`
	HOADues      float32            `json:"hoa_dues" bson:"hoa_dues"`
	Parking      string             `json:"parking" bson:"parking"`
	Url          string             `json:"url" bson:"url"`
}

type FuelEconomy struct {
	CityMPG    float32
	HighwayMPG float32
}

type Engine struct {
	Cyclinders   int
	FuelType     string
	Displacement float32
}

type CarInfo struct {
	Id             primitive.ObjectID
	Make           string
	Model          string
	Year           int32
	Color          string
	Milage         float32
	Price          float32
	Engine         Engine
	Tranmission    string
	DriveType      string
	MilesPerGallon FuelEconomy
	Vin            int64
	Features       []string
	Url            string
}

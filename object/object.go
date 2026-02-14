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
type Address struct {
	Street  string `json:"street" bson:"street"`
	City    string `json:"city" bson:"city"`
	State   string `json:"state" bson:"state"`
	Zipcode string `json:"zip_code" bson:"zip_code"`
}

type HomeContact struct {
	Realtor     string `json:"realtor" bson:"realtor"`
	Company     string `json:"company" bson:"company"`
	PhoneNumber string `json:"phone_number" bson:"phone_number"`
}

type School struct {
	Name     string `json:"name" bson:"name"`
	Type     string `json:"type" bson:"type"`
	Distance string `json:"distance" bson:"distance"`
}

type HomeInfo struct {
	Id           primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Address      Address            `json:"address" bson:"address"`
	Bedrooms     float32            `json:"bedrooms" bson:"bedrooms"`
	Bathrooms    float32            `json:"bathrooms" bson:"bathrooms"`
	HomeArea     Area               `json:"home_area" bson:"home_area"`
	Price        float32            `json:"price" bson:"price"`
	PropertyType string             `json:"property_type" bson:"property_type"`
	YearBuilt    string             `json:"year_built" bson:"year_built"`
	PricePerUnit float32            `json:"price_per_unit" bson:"price_per_unit"`
	LotArea      Area               `json:"lot_area" bson:"lot_area"`
	HOADues      float32            `json:"hoa_dues" bson:"hoa_dues"`
	Parking      string             `json:"parking" bson:"parking"`
	Schools      []School           `json:"schools" bson:"schools"`
	Contact      HomeContact        `json:"contact" bson:"contact"`
}

type FuelEconomy struct {
	CityMPG    float32 `json:"city_mpg" bson:"city_mpg"`
	HighwayMPG float32 `json:"highway_mpg" bson:"highway_mpg"`
}

type Engine struct {
	Cylinders    int     `json:"cylinders" bson:"cylinders"`
	FuelType     string  `json:"fuel_type" bson:"fuel_type"`
	Displacement float32 `json:"displacement" bson:"displacement"`
}

type CarInfo struct {
	Id             primitive.ObjectID `json:"id" bson:"_id"`
	Make           string             `json:"make" bson:"make"`
	Model          string             `json:"model" bson:"model"`
	Year           int32              `json:"year" bson:"year"`
	Color          string             `json:"color" bson:"color"`
	Mileage        float32            `json:"mileage" bson:"mileage"`
	Price          float32            `json:"price" bson:"price"`
	Engine         Engine             `json:"engine" bson:"engine"`
	Transmission   string             `json:"transmission" bson:"transmission"`
	DriveType      string             `json:"drive_type" bson:"drive_type"`
	MilesPerGallon FuelEconomy        `json:"miles_per_gallon" bson:"miles_per_gallon"`
	Vin            int64              `json:"vin" bson:"vin"`
	Features       []string           `json:"features" bson:"features"`
	Url            string             `json:"url" bson:"url"`
}

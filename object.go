package main

// Home info data to be uploaded to the Mongo database
type RedfinHomeInfo struct {
	Address      string
	Description  string
	Bedrooms     float32
	Bathrooms    float32
	Area         float32
	Price        float32
	PropertyType string
	YearBuilt    float32
	PricePerUnit float32
	LotSize      string
	Parking      string
	URL          string
}

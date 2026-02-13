package main

import (
	"flag"
	"fmt"

	"github.com/mikehquan19/useful-scraper/scrape/internal"
)

func main() {
	objectPtr := flag.String("object", "house", "What do you want to scrape (house, car, apartment, job)?")
	houseCityHrefPtr := flag.String("href", "/city/30861/TX/Richardson", "The href of the city on Redfin")
	carCityIdPtr := flag.String("id", "7207", "The id of the city on Carmax")
	flag.Parse()

	switch *objectPtr {
	case "house":
		internal.ScrapeHousing(*houseCityHrefPtr)
	case "car":
		internal.ScrapeCars(*carCityIdPtr)
	default:
		fmt.Println("Other objects are currently not supported yet.")
	}
}

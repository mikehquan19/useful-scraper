package main

import (
	"flag"
	"fmt"

	"github.com/mikehquan19/useful-scraper/scrape/scrapeinternal"
)

func main() {
	objectPtr := flag.String("object", "house", "What do you want to scrape (house, car, apartment, job)?")
	houseCityHrefPtr := flag.String("href", "/city/30861/TX/Richardson", "The href of the city on Redfin")
	carCityIdPtr := flag.String("id", "7207", "The id of the city on Carmax")
	testPtr := flag.Bool("test", false, "Are you trying to run scraper on test data?")
	flag.Parse()

	switch *objectPtr {
	case "house":
		scrapeinternal.ScrapeHousing(*houseCityHrefPtr, *testPtr)
	case "car":
		scrapeinternal.ScrapeCars(*carCityIdPtr, *testPtr)
	default:
		fmt.Println("Other objects are currently not supported yet.")
	}
}

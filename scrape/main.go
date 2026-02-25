package main

import (
	"flag"
	"fmt"

	"github.com/mikehquan19/useful-scraper/scrape/internal"
)

func main() {
	objectPtr := flag.String("object", "house", "Object to scrape (house, car, apartments)")
	cityPtr := flag.String("city", "richardson", "City of the scraped objects")
	uploadPtr := flag.Bool("upload", false, "Put the tools in uploading mode")
	parsePtr := flag.Bool("parse", false, "Put the tools in parsing mode")
	flag.Parse()

	switch *objectPtr {
	case "house":
		Housing(*cityPtr, *parsePtr, *uploadPtr)
	case "car":
		internal.ScrapeCars(*cityPtr)
	default:
		fmt.Println("Other objects are currently not supported yet.")
	}
}

// House-related tools
func Housing(city string, parse bool, upload bool) {
	var err error
	// Tool is in parsing mode
	if parse {
		if err = internal.ParseHouse(city); err != nil {
			panic(fmt.Errorf("Failed to parse houses\n%s", err))
		}
		return
	}

	// Tool is in uploading mode

	// Tool is in scraping model by default
	if err = internal.ScrapeHouse(city); err != nil {
		panic(fmt.Errorf("Failed to scrape houses\n%s", err))
	}
}

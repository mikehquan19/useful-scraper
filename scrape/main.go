package main

import (
	"flag"

	"github.com/mikehquan19/useful-scraper/scrape/scrapeinternal"
)

func main() {
	cityHrefPtr := flag.String("href", "/city/30861/TX/Richardson", "The href of the city on Redfin")
	testPtr := flag.Bool("test", true, "Are you trying to run scraper on test data?")
	flag.Parse()

	scrapeinternal.ScrapeHousing(*cityHrefPtr, *testPtr)
}

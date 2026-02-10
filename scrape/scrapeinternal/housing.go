/* Housing data scraper from Redfin.com */

package scrapeinternal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const REDFIN_URL string = "https://www.redfin.com"

// ScrapeHousing is main function to scrape the housing info
func ScrapeHousing(cityHref string) {
	ctx, cancel := getChromedpContext(getHeader)
	defer cancel()

	var homeLinks []string
	var err error

	if homeLinks, err = getHomeLinks(ctx, cityHref); err != nil {
		panic(err)
	}

	err = os.Mkdir("./data", 0664)
	if err != nil {
		fmt.Println("Data folder already created.")
	}

	if err = saveHTML(ctx, homeLinks); err != nil {
		panic(err)
	}
}

// getHomeLinks gets the list of links to the each home
func getHomeLinks(cdpCtx context.Context, baseHref string) ([]string, error) {
	var homeLinks []string
	fmt.Println("Scraping home links...")

	// Get the all the page links
	var pageNodes []*cdp.Node
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(REDFIN_URL+baseHref),
		chromedp.Nodes(".PageNumbers__page", &pageNodes, chromedp.ByQueryAll),
	)
	if err != nil {
		return nil, err
	}

	// Navigate to each page and get all the home links
	for _, pageNode := range pageNodes {
		pageHref, hrefExists := pageNode.Attribute("href")
		if !hrefExists {
			return nil, errors.New("can't get page links")
		}

		var homeNodes []*cdp.Node
		_, err = chromedp.RunResponse(cdpCtx,
			chromedp.Sleep(1000*time.Millisecond),
			chromedp.Navigate(REDFIN_URL+pageHref),
			chromedp.Nodes(".bp-Homecard__Address", &homeNodes, chromedp.ByQueryAll),
		)
		if err != nil {
			return nil, err
		}

		for _, homeNode := range homeNodes {
			homeHref, exists := homeNode.Attribute("href")
			if !exists {
				return nil, errors.New("can't get homelinks")
			}
			homeLinks = append(homeLinks, REDFIN_URL+homeHref)
		}
	}

	fmt.Printf("Sucessfully scaped %d home links!\n", len(homeLinks))
	return homeLinks, nil
}

func saveHTML(cdpCtx context.Context, homeLinks []string) error {
	var basicInfo, keyDetails string
	fmt.Println("Saving home into files...")

	savedHomes := 0
	for _, homeLink := range homeLinks {
		// Check if the house is already in file strorage
		arr := strings.Split(homeLink, "/")
		filepath := fmt.Sprintf("./data/%s.html", arr[len(arr)-1])

		_, err := os.Stat(filepath)
		if err == nil || !os.IsNotExist(err) {
			continue
		}

		// Navigate to each house's page and save it's HTML
		_, err = chromedp.RunResponse(cdpCtx,
			chromedp.Sleep(2000*time.Millisecond),
			chromedp.Navigate(homeLink),
			chromedp.OuterHTML(".AddressBannerV2", &basicInfo, chromedp.ByQuery),
			chromedp.OuterHTML(".keyDetailsList", &keyDetails, chromedp.ByQuery),
		)
		if err != nil {
			return err
		}

		html := fmt.Appendf(nil, "%s%s", basicInfo, keyDetails)
		err = os.WriteFile(filepath, html, 0644)
		if err != nil {
			return err
		}
		savedHomes += 1
	}

	fmt.Printf("Saved %d home infos successfully\n", savedHomes)
	return nil
}

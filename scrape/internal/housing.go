/* Housing data scraper from Redfin.com */

package internal

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const REDFIN_URL string = "https://www.redfin.com"
const SCRAPE bool = false

// ScrapeHousing is main function to scrape, parse, and upload the housing info
func ScrapeHousing(cityHref string) {
	var err error
	if SCRAPE {
		ctx, cancel := getChromedpContext(getHeader)
		defer cancel()

		var homeLinks []string

		if homeLinks, err = getHomeLinks(ctx, cityHref); err != nil {
			panic(fmt.Errorf("Failed to fetch links to all of the homes\n%s", err))
		}

		err = os.MkdirAll("./data", 0755)
		if err != nil {
			panic(fmt.Errorf("Failed to create data directory\n%s", err))
		}

		if err = saveHomeHTML(ctx, homeLinks); err != nil {
			panic(fmt.Errorf("Failed to save the home infos to directory\n%s", err))
		}
	} else {
		if err = parseHomeData(); err != nil {
			panic(fmt.Errorf("Failed to parse the home\n%s", err))
		}
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
		chromedp.WaitVisible(".PageNumbers__page"),
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
			chromedp.Sleep(1500*time.Millisecond),
			chromedp.Navigate(REDFIN_URL+pageHref),
			chromedp.WaitVisible(".bp-Homecard__Address"),
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

func saveHomeHTML(cdpCtx context.Context, homeLinks []string) error {
	var basicInfo, keyDetails, schoolInfo, agentInfo string
	fmt.Println("Saving home infos...")

	savedHomes := 0
	for _, homeLink := range homeLinks {
		// Check if the house is already in file strorage
		filename := path.Base(strings.TrimRight(homeLink, "/"))
		dir := fmt.Sprintf("./data/%s.html", filename)

		_, err := os.Stat(dir)
		if err == nil {
			fmt.Println(dir + " exists!")
			continue
		} else if !os.IsNotExist(err) {
			return err
		}

		// Navigate to each house's page and save it's HTML
		_, err = chromedp.RunResponse(cdpCtx,
			chromedp.Sleep(1500*time.Millisecond),
			chromedp.Navigate(homeLink),

			chromedp.WaitVisible(".AddressBannerV2"),
			chromedp.OuterHTML(".AddressBannerV2", &basicInfo, chromedp.ByQuery),

			chromedp.WaitVisible(".keyDetailsList"),
			chromedp.OuterHTML(".keyDetailsList", &keyDetails, chromedp.ByQuery),
		)
		if err != nil {
			return err
		}

		timedoutCtx, timedoutCancel := context.WithTimeout(cdpCtx, 10*time.Second)
		defer timedoutCancel()
		err = chromedp.Run(timedoutCtx,
			chromedp.WaitVisible(".agent-info-section"),
			chromedp.OuterHTML(".agent-info-section", &agentInfo, chromedp.ByQuery),
		)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Println("Agents not available in the website")
			} else {
				return err
			}
		}

		timedoutCtx, timedoutCancel = context.WithTimeout(cdpCtx, 10*time.Second)
		defer timedoutCancel()
		err = chromedp.Run(timedoutCtx,
			chromedp.WaitVisible(".schools-content"),
			chromedp.OuterHTML(".schools-content", &schoolInfo, chromedp.ByQuery),
		)
		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				fmt.Println("Schools not available in the website")
			} else {
				return err
			}
		}

		html := fmt.Appendf(nil,
			"<div>%s%s%s%s</div>",
			basicInfo, keyDetails, agentInfo, schoolInfo,
		)
		err = os.WriteFile(dir, html, 0755)
		if err != nil {
			return err
		}
		savedHomes += 1
		if savedHomes == 100 {
			// Only save 100 houses each city now for testing
			break
		}
	}

	fmt.Printf("Saved %d home infos successfully\n", savedHomes)
	return nil
}

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

// ScrapeHouse scrapes housing info in HTML from Redfin and saves them to files
func ScrapeHouse(city string) error {
	cdpCtx, cdpCancel := getChromedpContext(getHeader)
	defer cdpCancel()

	var homeLinks []string

	homeLinks, err := getHomeLinks(cdpCtx, city)
	if err != nil {
		return fmt.Errorf("Failed to fetch links to all of the homes\n%s", err)
	}

	// Create the non-existent city directory
	dirName := fmt.Sprintf("./data/house/%s", city)
	err = os.MkdirAll(dirName, 0755)
	if err != nil {
		return fmt.Errorf("Failed to create city dir\n%s", err)
	}

	if err = saveHomeHTML(cdpCtx, city, homeLinks); err != nil {
		return fmt.Errorf("Failed to save the home infos to dir\n%s", err)
	}

	return nil
}

// getHomeLinks gets the list of links to the each home
func getHomeLinks(cdpCtx context.Context, city string) ([]string, error) {
	cityHref, exists := HREF_MAP[strings.ToLower(city)]
	if !exists {
		return nil, fmt.Errorf("The city either doesn't exist or is not supported")
	}

	var homeLinks []string
	fmt.Println("Scraping home links...")

	// Get the all the page links
	var pageNodes []*cdp.Node
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(REDFIN_URL+cityHref),
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

func saveHomeHTML(cdpCtx context.Context, city string, homeLinks []string) error {
	var basicInfo, keyDetails, description, schoolInfo, agentInfo string
	fmt.Println("Saving home infos...")

	savedHomes := 0
	for _, homeLink := range homeLinks {
		// Check if the house is already in file strorage
		filename := path.Base(strings.TrimRight(homeLink, "/"))
		filepath := fmt.Sprintf("./data/house/%s/%s.html", city, filename)

		_, err := os.Stat(filepath)
		if err == nil {
			fmt.Println(filepath + " exists!")
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

		err = extractOrSkip(cdpCtx, ".sectionContent .remarks", &description)
		if err != nil {
			return err
		}
		err = extractOrSkip(cdpCtx, ".agent-info-section", &agentInfo)
		if err != nil {
			return err
		}
		err = extractOrSkip(cdpCtx, ".schools-content", &schoolInfo)
		if err != nil {
			return err
		}

		htmlContent := fmt.Appendf(nil,
			"<div>%s%s%s%s%s</div>",
			basicInfo, keyDetails, description, agentInfo, schoolInfo,
		)
		err = os.WriteFile(filepath, htmlContent, 0755)
		if err != nil {
			return err
		}
		savedHomes += 1
		if savedHomes == 50 {
			// Only save 50 houses each city now for development phase
			break
		}
	}

	fmt.Printf("Saved %d home infos successfully\n", savedHomes)
	return nil
}

// extractOrSkip try to extract html from selector or skip if it's hanging
func extractOrSkip(cdpCtx context.Context, sel string, html *string) error {
	// Use the timeout context
	timeoutCtx, timeoutCancel := context.WithTimeout(cdpCtx, 10*time.Second)
	defer timeoutCancel()

	err := chromedp.Run(timeoutCtx,
		chromedp.WaitVisible(sel),
		chromedp.OuterHTML(sel, html, chromedp.ByQuery),
	)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			fmt.Printf("%s not available in the website\n", sel)
		} else {
			return err
		}
	}

	return nil
}

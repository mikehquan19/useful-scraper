/* Housing data scraper from Redfin.com */

package scrapeinternal

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/mikehquan19/useful-scraper/object"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const REDFIN_URL string = "https://www.redfin.com"
const SCRAPE bool = false

// ScrapeHousing is main function to scrape the housing info
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

		if err = saveHTML(ctx, homeLinks); err != nil {
			panic(fmt.Errorf("Failed to save the home infos to directory\n%s", err))
		}
	}

	if err = parseData(); err != nil {
		panic(fmt.Errorf("Failed to parse the home\n%s", err))
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
	fmt.Println("Saving home infos...")

	savedHomes := 0
	for _, homeLink := range homeLinks {
		// Check if the house is already in file strorage
		filename := path.Base(strings.TrimRight(homeLink, "/"))
		filepath := fmt.Sprintf("./data/%s.html", filename)

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

		html := fmt.Appendf(nil, "%s%s", basicInfo, keyDetails)
		err = os.WriteFile(filepath, html, 0755)
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

func parseData() error {
	var homeInfos []object.RedfinHomeInfo

	err := filepath.WalkDir("./data", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			// The entry is a directory or a non-HTML file, we will skip
			return nil
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		htmlContent, err := goquery.NewDocumentFromReader(file)
		if err != nil {
			return err
		}

		bedrooms, bathrooms := getRooms(htmlContent)

		homeInfos = append(homeInfos, object.RedfinHomeInfo{
			Id:        primitive.NewObjectID(),
			Address:   htmlContent.Find(".full-address").Text(),
			Bedrooms:  bedrooms,
			Bathrooms: bathrooms,
		})

		return nil
	})

	fmt.Println(homeInfos)
	return err
}

func getRooms(content *goquery.Document) (float32, float32) {
	text := content.Find(".beds-section .statsValue").Text()
	bedrooms, err := strconv.ParseFloat(text, 32)
	if err != nil {
		bedrooms = 0
	}

	text = content.Find(".baths-section .bath-flyout").Text()
	// Split since num is displayed with labels
	text = strings.Split(text, " ")[0]
	bathrooms, err := strconv.ParseFloat(text, 32)
	if err != nil {
		bathrooms = 0
	}

	return float32(bedrooms), float32(bathrooms)
}

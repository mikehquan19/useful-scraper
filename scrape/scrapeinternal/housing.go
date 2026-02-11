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
	"regexp"
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
	} else {
		if err = parseData(); err != nil {
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
			chromedp.Sleep(1000*time.Millisecond),
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

		html := fmt.Appendf(nil, "<div>%s%s</div>", basicInfo, keyDetails)
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
	fmt.Println("Parsing home infos...")

	err := filepath.WalkDir("./data", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			// The entry is a directory or a non-HTML file, we will skip
			// TODO: Find a stronger way to enforce the HTML-only policy in ./data
			return nil
		}
		// Get the HTML doc from the file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		htmlContent, err := goquery.NewDocumentFromReader(file)
		if err != nil {
			return err
		}

		// Parse the value from the HTML doc
		bedrooms, bathrooms := getRooms(htmlContent)
		area, err := getArea(htmlContent)
		if err != nil {
			// This house's listing has invalid area, so we will skip it
			return nil
		}
		price, err := getPrice(htmlContent)
		if err != nil {
			// Skip invalid price
			return nil
		}

		detailsMap := getDetails(htmlContent)

		homeInfos = append(homeInfos, object.RedfinHomeInfo{
			Id:           primitive.NewObjectID(),
			Address:      htmlContent.Find(".full-address").Text(),
			Bedrooms:     bedrooms,
			Bathrooms:    bathrooms,
			HomeArea:     area,
			Price:        price,
			PropertyType: detailsMap["Property Type"].(string),
			YearBuilt:    detailsMap["Year Built"].(int32),
			PricePerUnit: detailsMap["Price/Sq.Ft."].(float32),
			LotArea:      detailsMap["Lot Size"].(object.Area),
			HOADues:      detailsMap["HOA Dues"].(float32),
			Parking:      detailsMap["Parking"].(string),
		})

		return nil
	})

	fmt.Println(homeInfos)
	fmt.Printf("Parsed %d home infos completely!", len(homeInfos))
	return err
}

func getRooms(content *goquery.Document) (float32, float32) {
	bedrooms, err := strconv.ParseFloat(
		content.Find(".beds-section .statsValue").Text(), 32,
	)
	if err != nil {
		bedrooms = 0 // It's okay since some houses have no beds :)
	}

	text := content.Find(".baths-section .bath-flyout").Text()
	bathrooms, err := strconv.ParseFloat(
		// Extract num because it is displayed together with labels
		strings.Split(text, " ")[0], 32,
	)
	if err != nil {
		bathrooms = 0
	}

	return float32(bedrooms), float32(bathrooms)
}

// getArea parses the the house's area from the HTML documents
func getArea(content *goquery.Document) (object.Area, error) {
	unit := strings.ReplaceAll(
		// Remove any space among the unit
		content.Find(".sqft-section .statsLabel").Text(), " ", "",
	)

	text := content.Find(".sqft-section .statsValue").Text()
	value, err := strconv.ParseFloat(
		// Remove the , from the number to parse
		strings.ReplaceAll(text, ",", ""), 32,
	)
	if err != nil {
		// This house's listing has invalid area
		return object.Area{}, err
	}

	return object.Area{
		Unit:  unit,
		Value: float32(value),
	}, nil
}

func getPrice(content *goquery.Document) (float32, error) {
	text := content.Find(".price").Text()[1:]
	price, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 32)
	if err != nil {
		// This house's listing has invalid price
		return 0, err
	}

	return float32(price), nil
}

func getDetails(content *goquery.Document) map[string]any {
	detailsMap := make(map[string]any)

	content.Find(".keyDetails-value").Each(func(i int, s *goquery.Selection) {
		detailsMap[s.Find(".valueType").Text()] =
			s.Find(".valueText").Text()
	})

	// NOTE: Home is allowed to have missing or invalid details, we will ignore it
	// Parse the year built
	value, _ := strconv.Atoi(detailsMap["Year Built"].(string))
	detailsMap["Year Built"] = int32(value)

	// Parse the lot size
	_, ok := detailsMap["Lot Size"]
	if !ok {
		detailsMap["Lot Size"] = object.Area{}
	} else {
		lotSizeParts := strings.Split(detailsMap["Lot Size"].(string), " ")

		unit := lotSizeParts[1]
		if len(lotSizeParts) > 2 {
			unit += lotSizeParts[2]
		}

		value, err := strconv.ParseFloat(
			strings.ReplaceAll(lotSizeParts[0], ",", ""), 32,
		)
		if err != nil {
			detailsMap["Lot Size"] = object.Area{}
		}

		detailsMap["Lot Size"] = object.Area{
			Unit:  unit,
			Value: float32(value),
		}
	}

	// Parse the price per unit
	_, ok = detailsMap["Price/Sq.Ft."]
	if !ok {
		detailsMap["Price/Sq.Ft."] = float32(0)
	} else {
		value, err := strconv.ParseFloat(detailsMap["Price/Sq.Ft."].(string)[1:], 32)
		if err != nil {
			detailsMap["Price/Sq.Ft."] = float32(0)
		} else {
			detailsMap["Price/Sq.Ft."] = float32(value)
		}
	}

	// Parse the HOA
	_, ok = detailsMap["HOA Dues"]
	if !ok {
		detailsMap["HOA Dues"] = float32(0)
	} else {
		regex := regexp.MustCompile(`[\d.]+`)
		value, err := strconv.ParseFloat(
			regex.FindString(detailsMap["HOA Dues"].(string)), 32,
		)
		if err != nil {
			detailsMap["HOA Dues"] = float32(0)
		} else {
			detailsMap["HOA Dues"] = float32(value)
		}
	}

	_, ok = detailsMap["Parking"]
	if !ok {
		detailsMap["Parking"] = "Not provided"
	}

	return detailsMap
}

package internal

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
	"github.com/mikehquan19/useful-scraper/object"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const carmaxBaseUrl = "https://www.carmax.com"

func ScrapeCars(cityId string) {
	ctx, cancel := getChromedpContext(getHeader)
	defer cancel()

	var carLinks []string
	var err error
	if carLinks, err = scrapeCarLinks(ctx, cityId); err != nil {
		panic(err)
	}

	var carInfos []object.CarInfo
	for _, carLink := range carLinks {
		scrapedCar, err := scrapeCar(ctx, carLink)
		if err != nil {
			panic(err)
		}
		fmt.Println(carLink)
		carInfos = append(carInfos, scrapedCar)
	}

	writeToFile(carInfos, "car_garland")
}

// Get all the car links of the city
func scrapeCarLinks(cdpCtx context.Context, cityId string) ([]string, error) {
	var carLinks []string
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(fmt.Sprintf("%s/cars/%s", carmaxBaseUrl, cityId)),
	)
	if err != nil {
		return nil, err
	}

	// Scroll all the way to the end of the website
	for elementExists(cdpCtx, "#see-more-button") {
		err := chromedp.Run(cdpCtx,
			chromedp.ScrollIntoView("#see-more-button", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
			chromedp.Click("#see-more-button", chromedp.ByQuery),
			chromedp.Sleep(1*time.Second),
		)
		if err != nil {
			return nil, err
		}
	}

	var carLinkNodes []*cdp.Node
	err = chromedp.Run(cdpCtx,
		chromedp.Nodes(".scct--make-model-info-link", &carLinkNodes, chromedp.ByQueryAll),
	)
	if err != nil {
		return nil, err
	}
	for i, carLinkNode := range carLinkNodes {
		carHref, hrefExists := carLinkNode.Attribute("href")
		if !hrefExists {
			return nil, fmt.Errorf("can't find link to car %d", i+1)
		}
		carLinks = append(carLinks, fmt.Sprintf("%s%s", carmaxBaseUrl, carHref))
	}

	return carLinks, nil
}

// Scrape each car
func scrapeCar(cdpCtx context.Context, carLink string) (object.CarInfo, error) {
	// Navigate to the page and immediately scrape the make, model, year

	_, err := chromedp.RunResponse(cdpCtx, chromedp.Navigate(carLink))
	if err != nil {
		return object.CarInfo{}, err
	}

	var carTitle string
	err = chromedp.Run(cdpCtx,
		chromedp.Text("#car-header-car-basic-info", &carTitle),
	)
	if err != nil {
		return object.CarInfo{}, err
	}

	// Parse the make, model, and year
	titleParts := strings.Split(carTitle, " ")
	year := titleParts[0]
	make := titleParts[1]
	model := strings.Join(titleParts[2:], " ")

	// Scrape the price and milage
	var price, milage string

	priceSel := "#default-price-display"
	if !elementExists(cdpCtx, priceSel) {
		// Car whose page shows the drop of price
		priceSel = "#price-drop-header-display .css-pff6mx"
	}
	err = chromedp.Run(cdpCtx, chromedp.Text(priceSel, &price))
	if err != nil {
		return object.CarInfo{}, err
	}
	// Scrape the milage
	err = chromedp.Run(cdpCtx, chromedp.Text(".car-header-milage", &milage))
	if err != nil {
		return object.CarInfo{}, err
	}

	digitsRegex := regexp.MustCompile(`\d+`)
	// Parse the milage and price
	milage = digitsRegex.FindString(milage)
	price = digitsRegex.FindString(strings.ReplaceAll(price, ",", ""))

	return object.CarInfo{
		Id:      primitive.NewObjectID(),
		Make:    make,
		Model:   model,
		Year:    strToInt32(year),
		Mileage: strToFloat32(milage),
		Price:   strToFloat32(price),
	}, nil
}

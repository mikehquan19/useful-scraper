package scrapeinternal

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

// Get all the car links of the city
func scrapeCarLinks(cdpCtx context.Context, cityId string) ([]string, error) {
	var carLinks []string
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(fmt.Sprintf("%s/cars/%s", carmaxBaseUrl, cityId)),
		chromedp.ActionFunc(func(ctx context.Context) error {
			for elementExists(ctx, "#see-more-button") {
				err := chromedp.Run(ctx,
					chromedp.ScrollIntoView("#see-more-button", chromedp.ByQuery),
					chromedp.Sleep(500*time.Millisecond),
					chromedp.Click("#see-more-button", chromedp.ByQuery),
					chromedp.Sleep(1500*time.Millisecond),
				)
				if err != nil {
					return err
				}
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}

	var carLinkNodes []*cdp.Node
	err = chromedp.Run(cdpCtx,
		chromedp.Nodes(".scct--make-model-info-link", &carLinkNodes, chromedp.ByQueryAll))
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
	var make, model, year string
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(carLink),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var carTitle string
			err := chromedp.Text("#car-header-car-basic-info", &carTitle).Do(ctx)
			if err != nil {
				return err
			}
			// Parse the make, model, and year
			splittedTitle := strings.Split(carTitle, " ")
			make, model = splittedTitle[1], strings.Join(splittedTitle[2:], " ")
			year = splittedTitle[0]
			return nil
		}),
	)
	if err != nil {
		return object.CarInfo{}, err
	}

	// Scrape the price and milage
	var price, milage string
	err = chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var rawPrice, rawMilage string
			var priceSel string
			if elementExists(ctx, "#default-price-display") {
				priceSel = "#default-price-display"
			} else {
				// Car whose page shows the drop of price
				priceSel = "#price-drop-header-display .css-pff6mx"
			}
			err = chromedp.Text(priceSel, &rawPrice, chromedp.ByQuery).Do(ctx)
			if err != nil {
				return err
			}
			// Scrape the milage, same for every car
			err = chromedp.Text(".car-header-milage", &rawMilage).Do(ctx)
			if err != nil {
				return err
			}
			// Parse the milage and price
			digitsRegex := regexp.MustCompile(`\d+`)
			milage = digitsRegex.FindString(rawMilage)
			price = digitsRegex.FindString(strings.ReplaceAll(rawPrice, ",", ""))
			return nil
		}),
	)
	if err != nil {
		return object.CarInfo{}, err
	}

	return object.CarInfo{
		Id:     primitive.NewObjectID(),
		Make:   make,
		Model:  model,
		Year:   strToInt32(year),
		Milage: strToFloat32(milage),
		Price:  strToFloat32(price),
	}, nil
}

func ScrapeCars(cityId string, test bool) {
	ctx, cancel := getChromedpContext(getHeader)
	defer cancel()

	var carLinks []string
	var err error
	if test {
		carLinks = []string{
			"https://www.carmax.com/car/27760742", "https://www.carmax.com/car/27326038",
		}
	} else {
		carLinks, err = scrapeCarLinks(ctx, cityId)
		if err != nil {
			panic(err)
		}
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

	if test {
		writeToFile(carInfos, "car_test")
	} else {
		writeToFile(carInfos, "car_garland")
	}
}

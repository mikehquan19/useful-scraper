/* Housing data scraper from Redfin.com */

package scrapeinternal

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
	"github.com/mikehquan19/useful-scraper/object"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const baseUrl = "https://www.redfin.com"

// Convert area as string to Area struct
func parseArea(rawArea string) object.Area {
	if rawArea == "" {
		return object.Area{} // Edge case: Just empty Area
	}
	splittedArea := strings.Split(rawArea, " ")
	var unit string
	if splittedArea[1] == "sq" {
		unit = splittedArea[1] + " " + splittedArea[2] // sq + ft
	} else {
		unit = splittedArea[1] // acres
	}
	return object.Area{
		Unit:  unit,
		Value: strToFloat32(splittedArea[0]),
	}
}

// Convert price from string to float32
func parsePrice(rawPrice string) float32 {
	parsedPrice := strings.Split(rawPrice, " ")[0]
	if string(parsedPrice[len(parsedPrice)-1]) == "+" {
		// For some reason, some prices have "+" or "-" after it
		return strToFloat32(parsedPrice[1 : len(parsedPrice)-1])
	}
	return strToFloat32(parsedPrice[1:])
}

// Convert HOA dues & parking field of the detail map
func parseHoaAndParking(homeDetailMap map[string]string) {
	// Parse HOA dues
	hoaDues, exists := homeDetailMap["HOA Dues"]
	if !exists {
		homeDetailMap["HOA Dues"] = "0"
	} else {
		// Find the sequence of digits that will be hoa
		digitsSeqRegex := regexp.MustCompile(`\d+`)
		homeDetailMap["HOA Dues"] = digitsSeqRegex.FindString(hoaDues)
	}

	// Parse parking lot
	_, exists = homeDetailMap["Parking"]
	if !exists {
		// Find the default value of the parking lot
		homeDetailMap["Parking"] = "Not provided"
	}
}

// Parse the price per unit to make sure the unit is correct
func parsePricePerUnit(homeDetailMap map[string]string) float32 {
	// Parse the price per unit since it can differ in units
	var pricePerUnit float32
	_, exists := homeDetailMap["Price/Sq.Ft."]
	if exists {
		pricePerUnit = parsePrice(homeDetailMap["Price/Sq.Ft."])
	} else {
		pricePerUnit = parsePrice(homeDetailMap["Price/Acres"])
	}
	return pricePerUnit
}

// Get the list of links to the each home
func scrapeHomeLinks(cdpCtx context.Context, baseHref string) ([]string, error) {
	var homeLinks []string
	fmt.Println("Scraping home links...")

	// Get the all the page links
	var pageLinkNodes []*cdp.Node
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(baseUrl+baseHref),
		chromedp.QueryAfter(".PageNumbers__page",
			func(
				tx context.Context, _ runtime.ExecutionContextID, nodes ...*cdp.Node,
			) error {
				pageLinkNodes = nodes
				return nil
			},
		),
	)
	if err != nil {
		return nil, err
	}

	// Navigate to each page and get all the home links
	for _, pageLinkNode := range pageLinkNodes {
		pageHref, hrefExists := pageLinkNode.Attribute("href")
		if !hrefExists {
			return nil, errors.New("can't have access to the href of the page")
		}

		_, err = chromedp.RunResponse(cdpCtx,
			chromedp.Sleep(1000*time.Millisecond),
			chromedp.Navigate(baseUrl+pageHref),
			chromedp.Sleep(1500*time.Millisecond),
			chromedp.QueryAfter(".bp-InteractiveHomecard",
				func(
					ctx context.Context, _ runtime.ExecutionContextID, nodes ...*cdp.Node,
				) error {
					for _, homeLinkNode := range nodes {
						homeHref, exists := homeLinkNode.Attribute("href")
						if !exists {
							return errors.New("can't have access to the href to home")
						}
						homeLinks = append(homeLinks, baseUrl+homeHref)
					}
					return nil
				},
			),
		)
		if err != nil {
			return nil, err
		}
	}

	fmt.Printf("Sucessfully scaped %d homes", len(homeLinks))
	return homeLinks, nil
}

// Scrape the number of bedrooms and bathrooms
func scrapeRooms(cdpCtx context.Context) (float32, float32, error) {
	// By default, let's assume the house or land doesn't have any rooms
	bedRooms, bathRooms := "0", "0"

	err := chromedp.Run(cdpCtx,
		chromedp.Text(".beds-section .statsValue", &bedRooms),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var nodes []*cdp.Node
			// Check if there's an element before extracting text
			bathSel := ".baths-section .bath-flyout"
			nodeErr := chromedp.Nodes(bathSel, &nodes, chromedp.AtLeast(0)).Do(ctx)
			if nodeErr != nil {
				return nodeErr
			}
			if len(nodes) > 0 {
				var bathText string
				scrapeErr := chromedp.Text(bathSel, &bathText).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				bathRooms = strings.Split(bathText, " ")[0]
			}
			return nil
		}),
	)
	if err != nil {
		return 0, 0, err
	}
	return strToFloat32(bedRooms), strToFloat32(bathRooms), nil
}

// Scrape the home area
func scrapeHomeArea(cdpCtx context.Context) (object.Area, error) {
	var homeArea object.Area
	err := chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var unit, value string
			// Extract unit and value separately and combine them
			scrapeErr := chromedp.Text(".sqft-section .statsValue", &value).Do(ctx)
			if scrapeErr != nil {
				return scrapeErr
			}
			scrapeErr = chromedp.Text(".sqft-section .statsLabel", &unit).Do(ctx)
			if scrapeErr != nil {
				return scrapeErr
			}
			homeArea = object.Area{Unit: unit, Value: strToFloat32(value)}
			return nil
		}),
	)
	if err != nil {
		return object.Area{}, err
	}
	return homeArea, nil
}

// Scrape the address and description
func scrapeAddressAndDescription(cdpCtx context.Context) (string, string, error) {
	var address, description string
	err := chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var nodes []*cdp.Node
			nodeErr := chromedp.Nodes(".street-address", &nodes, chromedp.AtLeast(0)).Do(ctx)
			if nodeErr != nil {
				return nodeErr
			}
			if len(nodes) > 0 {
				// The more common way of scraping address from the website
				var street, city string
				scrapeErr := chromedp.Text(".street-address", &street).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				scrapeErr = chromedp.Text(".bp-cityStateZip", &city).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				// Combine both parts of the address
				address = fmt.Sprintf("%s %s", street, city)
			} else {
				// Some houses have only one tag showing an entire address
				scrapeErr := chromedp.Text(".streetAddress", &address).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
			}
			return nil
		}),
		chromedp.Text(".sectionContent p", &description),
	)
	if err != nil {
		return "", "", err
	}
	return address, description, nil
}

// Scrape the extra information
func scrapeDetails(cdpCtx context.Context) (map[string]string, error) {
	homeDetailMap := make(map[string]string)

	var fieldNodes []*cdp.Node
	err := chromedp.Run(cdpCtx,
		chromedp.Nodes(".keyDetails-row", &fieldNodes, chromedp.ByQueryAll),
	)
	if err != nil {
		return nil, err
	}
	err = chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Going through each field as the tag
			for i := 0; i < len(fieldNodes)-1; i++ {
				var key, value string
				fieldTag := fmt.Sprintf(".keyDetails-row:nth-child(%d)", i+2)
				scrapeErr := chromedp.Text(fieldTag+" .valueType", &key).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				scrapeErr = chromedp.Text(fieldTag+" .valueText", &value).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}

				homeDetailMap[key] = value
				// Parse the default value of HOA values and parking
				parseHoaAndParking(homeDetailMap)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, err
	}
	return homeDetailMap, nil
}

// Main function to scrape the housing info
func ScrapeHousing(cityHref string, test bool) {
	ctx, cancel := getChromedpContext(getRedfinHeader)
	defer cancel()

	var homeLinks []string
	var homeInfos []object.RedfinHomeInfo
	var err error
	if test {
		// Scraping based on the test datas and not the other stuffs
		homeLinks = []string{
			"https://www.redfin.com/TX/Richardson/308-Lawndale-Dr-75080/home/31858127",
			"https://www.redfin.com/TX/Richardson/1133-Pacific-Dr-75081/home/31951493",
		}
	} else {
		// Example: "/city/30861/TX/Richardson"
		homeLinks, err = scrapeHomeLinks(ctx, cityHref)
		if err != nil {
			panic(err)
		}
	}

	for _, homeLink := range homeLinks {
		// Navigate to the page and immediately scrape the price
		var rawPrice string
		_, err := chromedp.RunResponse(ctx,
			chromedp.Sleep(1000*time.Millisecond),
			chromedp.Navigate(homeLink),
			chromedp.Sleep(2000*time.Millisecond),
			chromedp.Text(".price-section > .price", &rawPrice),
		)
		if err != nil {
			panic(err)
		}
		bedRooms, bathRooms, err := scrapeRooms(ctx)
		if err != nil {
			panic(err)
		}
		homeArea, err := scrapeHomeArea(ctx)
		if err != nil {
			panic(err)
		}
		address, description, err := scrapeAddressAndDescription(ctx)
		if err != nil {
			panic(err)
		}
		rawDetailMap, err := scrapeDetails(ctx)
		if err != nil {
			panic(err)
		}

		homeInfos = append(homeInfos, object.RedfinHomeInfo{
			Id:           primitive.NewObjectID(),
			Address:      address,
			Description:  description,
			Bedrooms:     bedRooms,
			Bathrooms:    bathRooms,
			HomeArea:     homeArea,
			Price:        parsePrice(rawPrice),
			PropertyType: rawDetailMap["Property Type"],
			YearBuilt:    strToInt32(rawDetailMap["Year Built"]),
			PricePerUnit: parsePricePerUnit(rawDetailMap),
			LotArea:      parseArea(rawDetailMap["Lot Size"]),
			HOADues:      strToFloat32(rawDetailMap["HOA Dues"]),
			Parking:      rawDetailMap["Parking"],
			Url:          homeLink,
		})
		fmt.Println(homeLink)
	}

	if test {
		writeToFile(homeInfos, "housing_test")
	} else {
		// Example: "/city/30861/TX/Richardson" -> Richardson
		splittedHref := strings.Split(cityHref, "/")
		writeToFile(homeInfos, fmt.Sprintf("housing_%s", splittedHref[len(splittedHref)-1]))
	}
}

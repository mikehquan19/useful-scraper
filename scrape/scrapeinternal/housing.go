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

// Main function to scrape the housing info
func ScrapeHousing(cityHref string, test bool) {
	ctx, cancel := getChromedpContext(getHeader)
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
		homeLinks, err = getHomeLinks(ctx, cityHref)
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
		bedRooms, bathRooms, err := getRooms(ctx)
		if err != nil {
			panic(err)
		}
		homeArea, err := getHomeArea(ctx)
		if err != nil {
			panic(err)
		}
		address, description, err := getAddressAndDescription(ctx)
		if err != nil {
			panic(err)
		}
		detailMap, err := getDetails(ctx)
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
			PropertyType: detailMap["Property Type"],
			YearBuilt:    strToInt32(detailMap["Year Built"]),
			PricePerUnit: parsePricePerUnit(detailMap),
			LotArea:      parseArea(detailMap["Lot Size"]),
			HOADues:      strToFloat32(detailMap["HOA Dues"]),
			Parking:      detailMap["Parking"],
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

// Get the list of links to the each home
func getHomeLinks(cdpCtx context.Context, baseHref string) ([]string, error) {
	var homeLinks []string
	fmt.Println("Scraping home links...")

	// Get the all the page links
	var pageLinkNodes []*cdp.Node
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(baseUrl+baseHref),
		chromedp.Nodes(".PageNumbers__page", &pageLinkNodes, chromedp.ByQueryAll),
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
				func(ctx context.Context, _ runtime.ExecutionContextID, nodes ...*cdp.Node) error {
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
func getRooms(cdpCtx context.Context) (float32, float32, error) {
	// By default, let's assume the house or land doesn't have any rooms
	bedRooms, bathRooms := "0", "0"

	err := chromedp.Run(cdpCtx,
		chromedp.Text(".beds-section .statsValue", &bedRooms),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if elementExists(cdpCtx, ".baths-section .bath-flyout") {
				var bathText string

				err := chromedp.Text(".baths-section .bath-flyout", &bathText).Do(ctx)
				if err != nil {
					return err
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
func getHomeArea(cdpCtx context.Context) (object.Area, error) {
	var homeArea object.Area

	err := chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Contains unit and value respectively
			var areaParts [2]string
			for i, tag := range [2]string{
				".sqft-section .statsLabel", ".sqft-section .statsValue",
			} {
				if err := chromedp.Text(tag, &areaParts[i]).Do(ctx); err != nil {
					return err
				}
			}
			homeArea = object.Area{
				Unit:  areaParts[0],
				Value: strToFloat32(areaParts[1]),
			}
			return nil
		}),
	)
	if err != nil {
		return object.Area{}, err
	}
	return homeArea, nil
}

// Scrape the address and description
func getAddressAndDescription(cdpCtx context.Context) (string, string, error) {
	var address, description string
	err := chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			if elementExists(cdpCtx, ".street-address") {
				// Contains street and city
				var addressParts [2]string
				for i, tag := range [2]string{
					".street-address", ".bp-cityStateZip",
				} {
					if err := chromedp.Text(tag, &addressParts[i]).Do(ctx); err != nil {
						return err
					}
				}
				address = fmt.Sprintf("%s %s", addressParts[0], addressParts[1])
			} else {
				err := chromedp.Text(".streetAddress", &address).Do(ctx)
				if err != nil {
					return err
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
func getDetails(cdpCtx context.Context) (map[string]string, error) {
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
			for i := 0; i < len(fieldNodes)-1; i++ {
				// Contains both the key and info
				var parts [2]string

				fieldTag := fmt.Sprintf(".keyDetails-row:nth-child(%d)", i+2)
				for j, addTag := range [2]string{
					"valueType", "valueText",
				} {
					err := chromedp.Text(fieldTag+" ."+addTag, &parts[j]).Do(ctx)
					if err != nil {
						return err
					}
				}

				homeDetailMap[parts[0]] = parts[1]
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

// Convert area as string to Area struct
func parseArea(rawArea string) object.Area {
	if rawArea == "" {
		return object.Area{} // Edge case: Just empty Area
	}
	splittedArea := strings.Split(rawArea, " ")
	unit := splittedArea[1] // Default: arces
	if splittedArea[1] == "sq" {
		unit += " " + splittedArea[2] // sq + ft
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
		homeDetailMap["Parking"] = "Not provided"
	}
}

// Parse the price per unit to make sure the unit is correct
func parsePricePerUnit(homeDetailMap map[string]string) float32 {
	var pricePerUnit float32
	_, exists := homeDetailMap["Price/Sq.Ft."]
	if exists {
		pricePerUnit = parsePrice(homeDetailMap["Price/Sq.Ft."])
	} else {
		pricePerUnit = parsePrice(homeDetailMap["Price/Acres"])
	}
	return pricePerUnit
}

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

// getHomeLinks gets the list of links to the each home
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
			chromedp.Sleep(1*time.Second),
			chromedp.Navigate(baseUrl+pageHref),
			chromedp.Sleep(1*time.Second),
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

	// Get the bed rooms
	bedRoomTag := ".beds-section .statsValue"
	err := chromedp.Run(cdpCtx, chromedp.Text(bedRoomTag, &bedRooms))
	if err != nil {
		return 0, 0, err
	}

	// Get the bath rooms
	bathRoomTag := ".baths-sections .bath-flyout"
	if elementExists(cdpCtx, bathRoomTag) {
		var bathRoomText string
		err = chromedp.Run(cdpCtx, chromedp.Text(bathRoomTag, &bathRoomText))
		if err != nil {
			return 0, 0, err
		}
		bathRooms = strings.Split(bathRoomText, " ")[0]
	}
	return strToFloat32(bedRooms), strToFloat32(bathRooms), nil
}

// Scrape the home area
func getHomeArea(cdpCtx context.Context) (object.Area, error) {
	var homeArea object.Area
	var err error

	// Contains the unit and value respectively
	areaTags := [2]string{".sqft-section .statsLabel", ".sqft-section .statsValue"}
	var areaParts [2]string
	for idx, areaTag := range areaTags {
		err = chromedp.Run(cdpCtx, chromedp.Text(areaTag, &areaParts[idx]))
		if err != nil {
			return homeArea, err
		}
	}
	homeArea.Unit = areaParts[0]
	homeArea.Value = strToFloat32(areaParts[1])
	return homeArea, nil
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

// getAddressAndDescription scrapes the address and description
func getAddressAndDescription(cdpCtx context.Context) (string, string, error) {
	var address, description string
	var err error

	// Get the address
	// For some reason, some houses have different address
	addressTags := [2]string{".street-address", ".bp-cityStateZip"}
	addressTag2 := ".streetAddress"
	if elementExists(cdpCtx, addressTags[0]) {
		// Contains street and city
		var addressParts [2]string
		for i, tag := range addressTags {
			err = chromedp.Run(cdpCtx, chromedp.Text(tag, &addressParts[i]))
			if err != nil {
				return "", "", err
			}
		}
		address = addressParts[0] + " " + addressParts[1]
	} else {
		err = chromedp.Run(cdpCtx, chromedp.Text(addressTag2, &address))
		if err != nil {
			return "", "", err
		}
	}

	// Get the description
	err = chromedp.Run(cdpCtx, chromedp.Text(".sectionContent p", &description))
	if err != nil {
		return "", "", err
	}
	return address, description, nil
}

// Scrape the extra information
func getDetails(cdpCtx context.Context) (map[string]string, error) {
	homeDetailMap := make(map[string]string)
	var err error

	var nodes []*cdp.Node
	err = chromedp.Run(cdpCtx,
		chromedp.Nodes(".keyDetails-row", &nodes, chromedp.ByQueryAll))
	if err != nil {
		return nil, err
	}

	for i := range len(nodes) {
		// Contains both the key and info
		var nodeParts [2]string
		tag := fmt.Sprintf(".keyDetails-row:nth-child(%d)", i+2)

		for j, tagType := range [2]string{"valueType", "valueText"} {
			err = chromedp.Run(cdpCtx, chromedp.Text(tag+" ."+tagType, &nodeParts[j]))
			if err != nil {
				return nil, err
			}
		}
		// Parse the default value of HOA values and parking
		homeDetailMap[nodeParts[0]] = nodeParts[1]
		parseHoaAndParking(homeDetailMap)
	}
	return homeDetailMap, nil
}

// Convert HOA dues & parking field of the detail map
func parseHoaAndParking(homeDetailMap map[string]string) {
	// Parse HOA dues
	hoaDues, exists := homeDetailMap["HOA Dues"]
	if !exists {
		homeDetailMap["HOA Dues"] = "0"
	} else {
		// Find the sequence of digits that will be HOA
		homeDetailMap["HOA Dues"] = regexp.MustCompile(`\d+`).FindString(hoaDues)
	}
	// Parse parking lot
	if _, exists = homeDetailMap["Parking"]; !exists {
		homeDetailMap["Parking"] = "Not provided"
	}
}

// Convert price from string to float32
func parsePrice(rawPrice string) float32 {
	price := strings.Split(rawPrice, " ")[0]
	if string(price[len(price)-1]) == "+" {
		// For some reason, some prices have "+" or "-" after it
		return strToFloat32(price[1 : len(price)-1])
	} else {
		return strToFloat32(price[1:])
	}
}

// Parse the price per unit to make sure the unit is correct
func parsePricePerUnit(homeDetailMap map[string]string) float32 {
	var pricePerUnit float32

	if _, exists := homeDetailMap["Price/Sq.Ft."]; exists {
		pricePerUnit = parsePrice(homeDetailMap["Price/Sq.Ft."])
	} else {
		pricePerUnit = parsePrice(homeDetailMap["Price/Acres"])
	}
	return pricePerUnit
}

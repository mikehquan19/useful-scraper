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

const baseUrl string = "https://www.redfin.com"

// Main function to scrape the housing info
func ScrapeHousing(cityHref string) {
	ctx, cancel := getChromedpContext(getHeader)
	defer cancel()

	var homeLinks []string
	var homeInfos []object.RedfinHomeInfo
	var err error

	if homeLinks, err = getHomeLinks(ctx, cityHref); err != nil {
		panic(err)
	}
	for _, homeLink := range homeLinks {
		// Navigate to the page
		_, err := chromedp.RunResponse(ctx,
			chromedp.Sleep(1500*time.Millisecond),
			chromedp.Navigate(homeLink),
			chromedp.Sleep(1500*time.Millisecond),
		)
		if err != nil {
			panic(err)
		}

		var rawPrice string
		err = chromedp.Run(ctx, chromedp.Text(".price-section > .price", &rawPrice))
		if err != nil {
			panic(err)
		}
		price := convertPrice(rawPrice)

		bedRooms, bathRooms := getRooms(ctx)
		homeArea := getHomeArea(ctx)
		address, description := getAddressAndDescription(ctx)
		detailMap := getDetails(ctx)

		homeInfos = append(homeInfos, object.RedfinHomeInfo{
			Id:           primitive.NewObjectID(),
			Address:      address,
			Description:  description,
			Bedrooms:     bedRooms,
			Bathrooms:    bathRooms,
			HomeArea:     homeArea,
			Price:        price,
			PropertyType: detailMap["Property Type"],
			YearBuilt:    strToInt32(detailMap["Year Built"]),
			PricePerUnit: getPricePerUnit(detailMap),
			LotArea:      convertArea(detailMap["Lot Size"]),
			HOADues:      strToFloat32(detailMap["HOA Dues"]),
			Parking:      detailMap["Parking"],
			Url:          homeLink,
		})
	}

	// Example: "/city/30861/TX/Richardson" -> Richardson
	splittedHref := strings.Split(cityHref, "/")
	writeToFile(homeInfos, fmt.Sprintf("housing_%s", splittedHref[len(splittedHref)-1]))
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

	fmt.Printf("Sucessfully scaped %d homes!", len(homeLinks))
	return homeLinks, nil
}

// Scrape the number of bedrooms and bathrooms
func getRooms(cdpCtx context.Context) (float32, float32) {
	// By default, let's assume the house or land doesn't have any rooms
	bedRooms, bathRooms := "0", "0"

	// Get the bed rooms
	err := chromedp.Run(cdpCtx, chromedp.Text(".beds-section .statsValue", &bedRooms))
	if err != nil {
		panic(err)
	}

	// Get the bath rooms
	if elementExists(cdpCtx, ".baths-sections .bath-flyout") {
		var bathRoomText string
		err = chromedp.Run(cdpCtx,
			chromedp.Text(".baths-sections .bath-flyout", &bathRoomText))
		if err != nil {
			panic(err)
		}
		bathRooms = strings.Split(bathRoomText, " ")[0]
	}
	return strToFloat32(bedRooms), strToFloat32(bathRooms)
}

// Scrape the home area
func getHomeArea(cdpCtx context.Context) object.Area {
	var homeArea object.Area
	var err error

	var areaParts [2]string // Contains the unit and value respectively
	for idx, areaTag := range [2]string{
		".sqft-section .statsLabel", ".sqft-section .statsValue",
	} {
		err = chromedp.Run(cdpCtx, chromedp.Text(areaTag, &areaParts[idx]))
		if err != nil {
			panic(err)
		}
	}
	homeArea.Unit = areaParts[0]
	homeArea.Value = strToFloat32(areaParts[1])
	return homeArea
}

// getAddressAndDescription scrapes the address and description
func getAddressAndDescription(cdpCtx context.Context) (string, string) {
	var address, description string
	var err error

	// Get the address
	if elementExists(cdpCtx, ".street-address") {
		var addressParts [2]string // Contains street and city
		for i, tag := range [2]string{".street-address", ".bp-cityStateZip"} {
			err = chromedp.Run(cdpCtx, chromedp.Text(tag, &addressParts[i]))
			if err != nil {
				panic(err)
			}
		}
		address = addressParts[0] + " " + addressParts[1]
	} else {
		// For some reason, some houses have different address
		err = chromedp.Run(cdpCtx, chromedp.Text(".streetAddress", &address))
		if err != nil {
			panic(err)
		}
	}

	// Get the description
	err = chromedp.Run(cdpCtx, chromedp.Text(".sectionContent p", &description))
	if err != nil {
		panic(err)
	}
	return address, description
}

// Scrape the extra information
func getDetails(cdpCtx context.Context) map[string]string {
	homeDetailMap := make(map[string]string)

	var nodes []*cdp.Node
	err := chromedp.Run(cdpCtx, chromedp.Nodes(".keyDetails-row", &nodes, chromedp.ByQueryAll))
	if err != nil {
		panic(err)
	}
	for i := range len(nodes) {
		var parts [2]string // Contains both the key and info
		tag := fmt.Sprintf(".keyDetails-row:nth-child(%d)", i+2)

		for j, addPart := range [2]string{" .valueType", " .valueText"} {
			err = chromedp.Run(cdpCtx, chromedp.Text(tag+addPart, &parts[j]))
			if err != nil {
				panic(err)
			}
		}
		homeDetailMap[parts[0]] = parts[1]
	}

	// Convert HOA dues & parking field of the detail map
	// Because sometimes they are not provided
	hoaDues, exists := homeDetailMap["HOA Dues"]
	if exists {
		homeDetailMap["HOA Dues"] = regexp.MustCompile(`\d+`).FindString(hoaDues)
	} else {
		homeDetailMap["HOA Dues"] = "0"
	}
	_, exists = homeDetailMap["Parking"]
	if !exists {
		homeDetailMap["Parking"] = "Not provided"
	}
	return homeDetailMap
}

// Get the price per unit to make sure the unit is correct
func getPricePerUnit(homeDetailMap map[string]string) float32 {
	var pricePerUnit float32

	_, exists := homeDetailMap["Price/Sq.Ft."]
	if exists {
		pricePerUnit = convertPrice(homeDetailMap["Price/Sq.Ft."])
	} else {
		pricePerUnit = convertPrice(homeDetailMap["Price/Acres"])
	}
	return pricePerUnit
}

// Convert area as string to Area struct
func convertArea(rawArea string) object.Area {
	if rawArea == "" {
		return object.Area{} // Edge case: Just empty Area
	}
	splittedArea := strings.Split(rawArea, " ")
	unit := splittedArea[1] // Default: arces
	if unit == "sq" {
		unit += " " + splittedArea[2] // sq + ft
	}
	return object.Area{
		Unit:  unit,
		Value: strToFloat32(splittedArea[0]),
	}
}

// Convert price from string to float32
func convertPrice(rawPrice string) float32 {
	price := strings.Split(rawPrice, " ")[0]

	if string(price[len(price)-1]) == "+" {
		// For some reason, some prices have "+" or "-" after it
		return strToFloat32(price[1 : len(price)-1])
	}
	return strToFloat32(price[1:])
}

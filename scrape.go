package main

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Get the list of links to the each home
const (
	baseUrl = "https://www.redfin.com"
)

func scrapeHomeLinks(cdpCtx context.Context, baseHref string) ([]string, error) {
	var homeLinks []string
	fmt.Println("Scraping home links...")

	// Get the all the page links
	var pageLinkNodes []*cdp.Node
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(baseUrl+baseHref),
		chromedp.Nodes(".PageNumbers__page", &pageLinkNodes),
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

		var homeLinkNodes []*cdp.Node
		_, err = chromedp.RunResponse(cdpCtx,
			chromedp.Sleep(1*time.Second),
			chromedp.Navigate(baseUrl+pageHref),
			chromedp.Nodes(".bp-InteractiveHomecard", &homeLinkNodes),
		)
		if err != nil {
			return nil, err
		}

		for _, homeLinkNode := range homeLinkNodes {
			homeHref, ok := homeLinkNode.Attribute("href")
			if !ok {
				return nil, errors.New("can't have access to the href to home")
			}
			homeLinks = append(homeLinks, baseUrl+homeHref)
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
			var bathRoomNodes []*cdp.Node
			bathRoomTag := ".baths-section .bath-flyout"
			_ = chromedp.Nodes(bathRoomTag, &bathRoomNodes, chromedp.AtLeast(0)).Do(ctx)
			if len(bathRoomNodes) > 0 {
				var bathRoomText string
				scrapeErr := chromedp.Text(bathRoomTag, &bathRoomText).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				// Parse the number of bathrooms
				bathRooms = strings.Split(bathRoomText, " ")[0]
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
func scrapeHomeArea(cdpCtx context.Context) (Area, error) {
	var homeArea Area
	err := chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var unit, value string
			err := chromedp.Text(".sqft-section .statsValue", &value).Do(ctx)
			if err != nil {
				return err
			}
			err = chromedp.Text(".sqft-section .statsLabel", &unit).Do(ctx)
			if err != nil {
				return err
			}
			// Get the unit and value separately and combine them together
			homeArea = Area{Unit: unit, Value: strToFloat32(value)}
			return nil
		}),
	)
	if err != nil {
		return Area{}, err
	}
	return homeArea, nil
}

// Scrape the address and description
func scrapeAddrAndDesc(cdpCtx context.Context) (string, string, error) {
	var address, description string
	err := chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			var nodes []*cdp.Node
			nodeErr := chromedp.Nodes(".street-address", &nodes, chromedp.AtLeast(0)).Do(ctx)
			if nodeErr != nil {
				return nodeErr
			}
			if len(nodes) > 0 {
				// The more traditional way of scraping from the website
				var street, cityState string
				scrapeErr := chromedp.Text(".address .street-address", &street).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				scrapeErr = chromedp.Text(".address .bp-cityStateZip", &cityState).Do(ctx)
				if scrapeErr != nil {
					return scrapeErr
				}
				// Combine both parts of the address
				address = fmt.Sprintf("%s %s", street, cityState)
			} else {
				// Some houses have different address tags
				scrapeErr := chromedp.Text(".address .streetAddress", &address).Do(ctx)
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
		chromedp.Nodes(".keyDetails-row", &fieldNodes, chromedp.ByQueryAll))
	if err != nil {
		return nil, err
	}
	err = chromedp.Run(cdpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i := 0; i < len(fieldNodes)-1; i++ {
				fieldTag := fmt.Sprintf(".keyDetails-row:nth-child(%d)", i+2)
				var key, value string
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

// Convert string area to Area struct
func parseArea(rawArea string) Area {
	if rawArea == "" {
		return Area{} // Edge case: Just empty stuff
	}
	splittedSlice := strings.Split(rawArea, " ")
	var unit string
	if splittedSlice[1] == "sq" {
		unit = splittedSlice[1] + " " + splittedSlice[2]
	} else {
		unit = splittedSlice[1]
	}
	return Area{
		Unit:  unit,
		Value: strToFloat32(splittedSlice[0]),
	}
}

// Convert price from string to float32
func parsePrice(rawPrice string) float32 {
	parsedPrice := strings.Split(rawPrice, " ")[0]
	if string(parsedPrice[len(parsedPrice)-1]) == "+" {
		return strToFloat32(parsedPrice[1 : len(parsedPrice)-1])
	}
	return strToFloat32(parsedPrice[1:])
}

// Convert HOA dues & parking field of the detail map
func parseHoaAndParking(detailMap map[string]string) {
	// Parse HOA dues
	hoaDues, exists := detailMap["HOA Dues"]
	if !exists {
		detailMap["HOA Dues"] = "0"
	} else {
		// Find the sequence of digits that will be hoa
		digitsSeqRegex := regexp.MustCompile(`\d+`)
		detailMap["HOA Dues"] = digitsSeqRegex.FindString(hoaDues)
	}

	// Parse parking lot
	_, exists = detailMap["Parking"]
	if !exists {
		// Find the default value of the parking lot
		detailMap["Parking"] = "Not provided"
	}
}

// Parse the price per unit to make sure the unit is correct
func parsePricePerUnit(detailMap map[string]string) float32 {
	// Parse the price per unit since it can differ in units
	var pricePerUnit float32
	_, sqftExists := detailMap["Price/Sq.Ft."]
	if sqftExists {
		pricePerUnit = parsePrice(detailMap["Price/Sq.Ft."])
	} else {
		pricePerUnit = parsePrice(detailMap["Price/Acres"])
	}
	return pricePerUnit
}

func main() {
	opts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), opts...)
	defer cancel()
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	// Set the strong header so that they will bypass cloudfont 403
	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := network.Headers(getRedfinHeader())
			return network.SetExtraHTTPHeaders(headers).Do(ctx)
		}),
	)
	if err != nil {
		panic(err)
	}

	homeLinks, err := scrapeHomeLinks(ctx, "/city/30821/TX/Garland")
	if err != nil {
		panic(err)
	}
	var homeInfos []RedfinHomeInfo

	for _, homeLink := range homeLinks {
		// Navigate to the page and immediately scrape the price
		var price string
		_, err = chromedp.RunResponse(ctx,
			chromedp.Sleep(1*time.Second),
			chromedp.Navigate(homeLink),
			chromedp.Sleep(2*time.Second),
			chromedp.Text(".price-section > .price", &price),
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
		address, description, err := scrapeAddrAndDesc(ctx)
		if err != nil {
			panic(err)
		}
		homeDetailMap, err := scrapeDetails(ctx)
		if err != nil {
			panic(err)
		}

		homeInfos = append(homeInfos, RedfinHomeInfo{
			Id:           primitive.NewObjectID(),
			Address:      address,
			Description:  description,
			Bedrooms:     bedRooms,
			Bathrooms:    bathRooms,
			HomeArea:     homeArea,
			Price:        parsePrice(price),
			PropertyType: homeDetailMap["Property Type"],
			YearBuilt:    strToInt32(homeDetailMap["Year Built"]),
			PricePerUnit: parsePricePerUnit(homeDetailMap),
			LotArea:      parseArea(homeDetailMap["Lot Size"]),
			HOADues:      strToFloat32(homeDetailMap["HOA Dues"]),
			Parking:      homeDetailMap["Parking"],
			Url:          homeLink,
		})
		fmt.Println(homeLink)
	}
	writeToFile(homeInfos, "garland")
}

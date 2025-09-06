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
)

// Get the list of links to the each home
const (
	baseUrl = "https://www.redfin.com"
)

func getHomeLinks(chromedpCtx context.Context, baseHref string) ([]string, error) {
	var homeLinks []string
	fmt.Println("Scraping home links...")

	// Get the all the page links
	var pageLinkNodes []*cdp.Node
	_, err := chromedp.RunResponse(chromedpCtx,
		chromedp.Navigate(baseUrl+baseHref),
		chromedp.Nodes(".PageNumbers__page", &pageLinkNodes),
	)
	if err != nil {
		return nil, err
	}

	// Navigate to each page and get all the home links
	for _, pageLinkNode := range pageLinkNodes {
		pageHref, ok := pageLinkNode.Attribute("href")
		if !ok {
			return nil, errors.New("can't have access to the href of the page")
		}
		var homeLinkNodes []*cdp.Node

		_, err = chromedp.RunResponse(chromedpCtx,
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

	homeLinks, err := getHomeLinks(ctx, "/city/30861/TX/Richardson")
	if err != nil {
		panic(err)
	}
	//homeLinks := []string{"https://www.redfin.com/TX/Richardson/2648-Custer-Pkwy-75080/unit-D/home/31860462"}
	var homeInfos []RedfinHomeInfo

	for _, homeLink := range homeLinks {
		// Navigate to the page and immediately scrape the price
		var price string
		_, err = chromedp.RunResponse(ctx,
			chromedp.Sleep(2*time.Second),
			chromedp.Navigate(homeLink),
			chromedp.Text(".price-section > .price", &price),
		)
		if err != nil {
			panic(err)
		}

		// Scrape the bedrooms, bathrooms
		bedRooms, bathRooms := "0", "0" // By default, let's assume house doesn't have rooms
		err = chromedp.Run(ctx,
			chromedp.Text(".beds-section .statsValue", &bedRooms),
			chromedp.ActionFunc(func(ctx context.Context) error {
				var bathRoomNodes []*cdp.Node
				bathRoomTag := ".baths-section .bath-flyout"
				_ = chromedp.Nodes(bathRoomTag, &bathRoomNodes, chromedp.AtLeast(0)).Do(ctx)
				if len(bathRoomNodes) > 0 {
					var bathRoomText string
					err = chromedp.Text(bathRoomTag, &bathRoomText).Do(ctx)
					if err != nil {
						return err
					}
					// Parse the number of bathrooms
					bathRooms = strings.Split(bathRoomText, " ")[0]
				}
				return nil
			}),
		)
		if err != nil {
			panic(err)
		}

		// Scrape the home area
		var homeArea Area
		err = chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var unit, value string
				err = chromedp.Text(".sqft-section .statsValue", &value).Do(ctx)
				if err != nil {
					return err
				}
				err = chromedp.Text(".sqft-section .statsLabel", &unit).Do(ctx)
				if err != nil {
					return err
				}
				homeArea = Area{Unit: unit, Value: strToFloat32(value)}
				return nil
			}),
		)

		// Scrape the address and description
		var address, description string
		err = chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				var street, cityStateZip string
				err = chromedp.Text(".street-address", &street).Do(ctx)
				if err != nil {
					return err
				}
				err = chromedp.Text(".bp-cityStateZip", &cityStateZip).Do(ctx)
				if err != nil {
					return err
				}
				// Combine both parts of the address
				address = fmt.Sprintf("%s %s", street, cityStateZip)
				return nil
			}),
			chromedp.Text(".sectionContent p", &description),
		)
		if err != nil {
			panic(err)
		}

		// Scrape the additional information
		homeDetailMap := make(map[string]string)
		var fieldNodes []*cdp.Node

		err = chromedp.Run(ctx,
			chromedp.Nodes(".keyDetails-row", &fieldNodes, chromedp.ByQueryAll))
		if err != nil {
			panic(err)
		}
		err = chromedp.Run(ctx,
			chromedp.ActionFunc(func(ctx context.Context) error {
				// Set default for parking and also HOA dues
				for i := 0; i < len(fieldNodes)-1; i++ {
					fieldTag := fmt.Sprintf(".keyDetails-row:nth-child(%d)", i+2)
					var fieldType, fieldValue string
					err = chromedp.Text(fieldTag+" .valueType", &fieldType).Do(ctx)
					if err != nil {
						return err
					}
					err = chromedp.Text(fieldTag+" .valueText", &fieldValue).Do(ctx)
					if err != nil {
						return err
					}
					homeDetailMap[fieldType] = fieldValue
					parseHoaAndParking(homeDetailMap)
				}
				return nil

			}),
		)
		if err != nil {
			panic(err)
		}
		// Parse the price per unit since it can differ in units
		var pricePerUnit string
		_, ok := homeDetailMap["Price/Sq.Ft."]
		if ok {
			pricePerUnit = homeDetailMap["Price/Sq.Ft."]
		} else {
			pricePerUnit = homeDetailMap["Price/Acres"]
		}

		homeInfos = append(homeInfos, RedfinHomeInfo{
			Address:      address,
			Description:  description,
			Bedrooms:     strToFloat32(bedRooms),
			Bathrooms:    strToFloat32(bathRooms),
			HomeArea:     homeArea,
			Price:        parsePrice(price),
			PropertyType: homeDetailMap["Property Type"],
			YearBuilt:    strToInt32(homeDetailMap["Year Built"]),
			PricePerUnit: parsePrice(pricePerUnit),
			LotArea:      parseArea(homeDetailMap["Lot Size"]),
			HOADues:      strToFloat32(homeDetailMap["HOA Dues"]),
			Parking:      homeDetailMap["Parking"],
			Url:          homeLink,
		})
		fmt.Println(homeLink)
	}
	writeToFile(homeInfos, "homes.json")
}

// Convert string area to Area struct
func parseArea(rawArea string) Area {
	if rawArea == "" {
		// Edge case: Just empty stuff
		return Area{}
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
	return strToFloat32(strings.Split(rawPrice, " ")[0][1:])
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
		detailMap["Parking"] = "Not provided"
	}
}

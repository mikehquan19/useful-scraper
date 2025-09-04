package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

func main() {
	ctx, cancel := chromedp.NewExecAllocator(
		context.Background(),
		append(chromedp.DefaultExecAllocatorOptions[:],
			// This is important to avoid being blocked by CloudFont
			chromedp.Flag("headless", false),
			chromedp.Flag("disable-blink-features", "AutomationControlled"),
		)...,
	)
	defer cancel()

	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	// List of nodes in the page containing home infos
	var homeNodes []*cdp.Node
	fmt.Println("Parsing the nodes containing home info...")

	err := chromedp.Run(ctx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			headers := getRedfinHeader()
			return network.SetExtraHTTPHeaders(network.Headers(headers)).Do(ctx)
		}),
		chromedp.Navigate("https://www.redfin.com/city/30861/TX/Richardson"),
	)
	if err != nil {
		log.Fatal("ddd")
	}

	err = chromedp.Run(ctx,
		chromedp.WaitVisible(".HomeCardsContainer"),
		chromedp.Nodes(".bp-InteractiveHomecard", &homeNodes, chromedp.ByQueryAll),
	)
	if err != nil {
		log.Fatal("Error: ", err)
	}
	fmt.Printf("Parsed successfully %v house nodes! \n", len(homeNodes))

	fmt.Println("Scraping the home info completely...")
	var homeInfoList []RedfinHomeInfo

	// auxiliary arrays (slices) to support things
	var infoValues [6]string
	var actions []chromedp.Action

	// fill list of home info with first 5 data fields
	for i, node := range homeNodes {
		actions = []chromedp.Action{
			chromedp.Sleep(1 * time.Second), // Wait a bit for it to load

			// Scroll down for lazy-loading
			chromedp.Evaluate("window.scrollBy(0, window.innerHeight);", nil),
			chromedp.WaitVisible(".bp-Homecard__Content", chromedp.FromNode(node)),
		}
		for i, tag := range []string{
			"Address", "Stats--beds", "Stats--baths", "LockedStat--value", "Price--value",
		} {
			actions = append(actions, chromedp.Text(
				fmt.Sprintf(".bp-Homecard__%s", tag),
				&infoValues[i],
				chromedp.ByQuery, chromedp.FromNode(node)),
			)
		}
		actions = append(actions, chromedp.AttributeValue(
			".bp-Homecard__Photo > address > a",
			"href",
			&infoValues[5],
			nil,
			chromedp.ByQuery, chromedp.FromNode(node)),
		)
		err = chromedp.Run(ctx, actions...)
		if err != nil {
			log.Fatal("Error: ", err)
		}

		homeInfoList = append(homeInfoList, RedfinHomeInfo{
			Address:   infoValues[0],
			Bedrooms:  strToFloat32(strings.Split(infoValues[1], " ")[0]),
			Bathrooms: strToFloat32(strings.Split(infoValues[2], " ")[0]),
			Area:      strToFloat32(strings.Join(strings.Split(infoValues[3], ","), "")),
			Price:     strToFloat32(strings.Join(strings.Split(infoValues[4][1:], ","), "")),
			URL:       fmt.Sprintf("https://www.redfin.com%s", infoValues[5]),
		})
		fmt.Printf("Processed first 6 fields of home %v! \n", i+1)

	}

	// Run to get the remaining 5 fields of current list of home info
	for i := range len(homeInfoList) {
		err = chromedp.Run(ctx,
			chromedp.Sleep(2*time.Second),
			chromedp.Navigate(homeInfoList[i].URL),
		)
		if err != nil {
			log.Fatal("Error: ", err)
		}

		actions = []chromedp.Action{}
		for j := range 6 {
			// Start from index 2 because index 1 only tells num of days listed on Redfin
			tag := fmt.Sprintf(".keyDetails-row:nth-of-type(%s) .valueText", strconv.Itoa(j+1))
			actions = append(actions, chromedp.Text(
				tag, &infoValues[j],
				chromedp.ByQuery,
			))
		}

		fmt.Printf("Run for %v ...\n", i+1)
		err = chromedp.Run(ctx, actions...)
		if err != nil {
			log.Fatal("Error: ", err)
		}

		homeInfoList[i].PropertyType = infoValues[1]
		homeInfoList[i].YearBuilt = strToFloat32(infoValues[2])
		homeInfoList[i].LotSize = infoValues[3]
		homeInfoList[i].PricePerUnit = strToFloat32(strings.Join(strings.Split(infoValues[4][1:], ","), ""))
		homeInfoList[i].Parking = infoValues[5]

		fmt.Printf("Proccessed last 5 fields of home %v!\n!", i+1)
	}

	writeToFile(homeInfoList, "redfin_house")
	fmt.Println("Scraped info successfully!")
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

// TODO: Given the URL, we need to navigate to each page and get more info. Then upload to mongodb

// home info data to be uploaded to the database
type HomeInfo struct {
	Address   string
	Bedrooms  float32
	Bathrooms float32
	Area      float32
	Price     float32
}

// convert string to float64
func strToFloat32(str string) float32 {
	var convertedValue float32
	err := json.Unmarshal([]byte(str), &convertedValue)
	if err != nil {
		fmt.Println(str)
		log.Fatal("Error: ", err)
	}

	return convertedValue
}

func main() {
	// define the proxy settings
	options := append(
		chromedp.DefaultExecAllocatorOptions[:],
		chromedp.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/115.0.0.0 Safari/537.36"),
	)
	ctx, cancel := chromedp.NewExecAllocator(context.Background(), options...)
	defer cancel()

	fmt.Println("Set up user agent ")

	// initialize the chrome instance
	ctx, cancel = chromedp.NewContext(ctx)
	defer cancel()

	// slice of nodes in the page containing home infos
	var homeCardNodes []*cdp.Node

	fmt.Println("Parsing the nodes containing home info...")
	err := chromedp.Run(ctx,
		chromedp.Navigate("https://www.redfin.com/city/30861/TX/Richardson"),
		chromedp.WaitVisible(".HomeCardsContainer"),
		chromedp.Nodes(".bp-InteractiveHomecard", &homeCardNodes, chromedp.ByQueryAll),
	)
	if err != nil {
		log.Fatal("Error while performing the automation logic: ", err)
	}

	fmt.Printf("Parsed successfully %v nodes! \n", len(homeCardNodes))

	fmt.Println("Scraping the home info completely...")
	var homeInfoList []HomeInfo
	var homeURLList []string

	// auxiliary arrays (slices) to support things
	var infoStrings [6]string
	var textActions []chromedp.Action

	for index, node := range homeCardNodes {
		// run to get first 5 data fields
		textActions = []chromedp.Action{chromedp.WaitVisible(".bp-Homecard__Content", chromedp.FromNode(node))}
		for i, tag := range []string{"Address", "Stats--beds", "Stats--baths", "LockedStat--value", "Price--value"} {
			textActions = append(textActions,
				chromedp.Text(".bp-Homecard__"+tag, &infoStrings[i], chromedp.ByQuery, chromedp.FromNode(node)),
			)
		}
		textActions = append(textActions, chromedp.AttributeValue(".bp-Homecard__Photo", "href", &infoStrings[5], nil, chromedp.FromNode(node)))

		err = chromedp.Run(ctx, textActions...)
		if err != nil {
			log.Fatal("Error: ", err)
		}

		// process the numerical value and append
		homeInfoList = append(homeInfoList, HomeInfo{
			Address:   infoStrings[0],
			Bedrooms:  strToFloat32(strings.Split(infoStrings[1], " ")[0]),
			Bathrooms: strToFloat32(strings.Split(infoStrings[2], " ")[0]),
			Area:      strToFloat32(strings.Join(strings.Split(infoStrings[3], ","), "")),
			Price:     strToFloat32(strings.Join(strings.Split(infoStrings[4][1:], ","), "")),
		})
		// add the URL (which will be used later)
		homeURLList = append(homeURLList, "https://www.redfin.com/"+infoStrings[5])

		fmt.Printf("Processed home %v! \n", index+1)
	}
	fmt.Println("Scraped info successfully!")

	// print the list
	for i, homeInfo := range homeInfoList {
		fmt.Println(homeInfo)
		fmt.Println(homeURLList[i])
	}
}

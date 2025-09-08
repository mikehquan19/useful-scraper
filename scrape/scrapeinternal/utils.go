package scrapeinternal

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/chromedp"
)

// Generate the chromedp context with strong header to scrape sites using cloudfont
// or other anti-bot algorithms
func getChromedpContext(getHeaderFunc func() map[string]interface{}) (context.Context, context.CancelFunc) {
	// By default, the context are headful
	allocOptions := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.Flag("headless", false),
		chromedp.Flag("disable-blink-features", "AutomationControlled"),
	)
	chromedpCtx, _ := chromedp.NewExecAllocator(context.Background(), allocOptions...)
	chromedpCtx, cancel := chromedp.NewContext(chromedpCtx)

	// Set the strong header so that they will bypass cloudfont 403 of website
	err := chromedp.Run(chromedpCtx,
		chromedp.ActionFunc(func(ctx context.Context) error {
			// Enable the network domain
			if enableErr := network.Enable().Do(ctx); enableErr != nil {
				return enableErr
			}

			headers := network.Headers(getHeaderFunc())
			return network.SetExtraHTTPHeaders(headers).Do(ctx)
		}),
	)
	if err != nil {
		panic(err)
	}

	return chromedpCtx, cancel
}

// Generate the custom header for Redfin scraper
// TODO: Needs to be fixed a little bit
func getRedfinHeader() map[string]interface{} {
	return map[string]interface{}{
		"Accept":                    "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8",
		"Accept-Encoding":           "gzip, deflate, br",
		"Accept-Language":           "en-US,en;q=0.9",
		"Cache-Control":             "no-cache",
		"Pragma":                    "no-cache",
		"Sec-CH-UA":                 `"Not_A Brand";v="8", "Chromium";v="117", "Google Chrome";v="117"`,
		"Sec-CH-UA-Mobile":          "?0",
		"Sec-CH-UA-Platform":        `"macOS"`,
		"Sec-Fetch-Dest":            "document",
		"Sec-Fetch-Mode":            "navigate",
		"Sec-Fetch-Site":            "none",
		"Sec-Fetch-User":            "?1",
		"Upgrade-Insecure-Requests": "1",
		"User-Agent":                "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/117.0.0.0 Safari/537.36",
	}
}

// Generate the custom header for Carmax scraper
func getCarmaxHeader() map[string]interface{} {
	return getRedfinHeader()
}

// Write the list of objects to file
func writeToFile[T any](objectList []T, fileName string) {
	// Convert the list to the json
	jsonValue, err := json.Marshal(objectList)
	if err != nil {
		fmt.Println(err)
	}

	// Write to JSON file,
	err = os.WriteFile(fmt.Sprintf("./data/%s.json", fileName), jsonValue, 0644)
	if err != nil {
		fmt.Println(err)
	}
}

// Convert string to float32
func strToFloat32(str string) float32 {
	var convertedValue float32
	cleanedStr := strings.ReplaceAll(str, ",", "")
	err := json.Unmarshal([]byte(cleanedStr), &convertedValue)
	if err != nil {
		// Any problem with parsing string will just result in 0.
		// Works for "-"
		return 0
	}
	return float32(convertedValue)
}

// Convert string to int32
func strToInt32(str string) int32 {
	cleanedStr := strings.ReplaceAll(str, ",", "")
	convertedValue, err := strconv.ParseInt(cleanedStr, 10, 32)
	if err != nil {
		return 0
	}
	return int32(convertedValue)
}

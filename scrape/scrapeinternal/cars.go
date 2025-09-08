package scrapeinternal

import (
	"context"
	"fmt"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/chromedp"
)

const carmaxBaseUrl = "https://www.carmax.com"

// Check if there's an element visible on the page
func elementExists(ctx context.Context, sel string) bool {
	var bttns []*cdp.Node
	nodeErr := chromedp.Nodes(sel, &bttns, chromedp.AtLeast(0)).Do(ctx)
	if nodeErr != nil {
		panic(nodeErr)
	}
	return len(bttns) > 0
}

// Get all the car links of the city
func scrapeCarLinks(cdpCtx context.Context, cityId string) ([]string, error) {
	var carLinks []string
	_, err := chromedp.RunResponse(cdpCtx,
		chromedp.Navigate(fmt.Sprintf("%s/cars/%s", carmaxBaseUrl, cityId)),
		chromedp.ActionFunc(func(ctx context.Context) error {

			// Keep click the see-more button to get more cars as long as there's one
			for elementExists(ctx, "#see-more-button") {
				clickErr := chromedp.Run(ctx,
					chromedp.ScrollIntoView("#see-more-button", chromedp.ByQuery),
					chromedp.Sleep(1500*time.Millisecond),
					chromedp.Click("#see-more-button", chromedp.ByQuery),
					chromedp.Sleep(2500*time.Millisecond),
				)
				if clickErr != nil {
					return clickErr
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

func ScrapeCars(cityId string) {
	ctx, cancel := getChromedpContext(getCarmaxHeader)
	defer cancel()

	carLinks, err := scrapeCarLinks(ctx, cityId)
	if err != nil {
		panic(err)
	}
	fmt.Println(len(carLinks))
}

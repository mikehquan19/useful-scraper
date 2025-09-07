package scrapeinternal

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Convert string to float32
func strToFloat32(str string) float32 {
	var convertedValue float32
	cleanedStr := strings.ReplaceAll(str, ",", "")
	err := json.Unmarshal([]byte(cleanedStr), &convertedValue)
	if err != nil {
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

// Generate the headers for Redfin scraper
// TODO: Needs to be fixed a little bit
func getRedfinHeader() map[string]any {
	return map[string]any{
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

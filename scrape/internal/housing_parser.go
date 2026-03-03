package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mikehquan19/useful-scraper/object"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const MAPBOX_URL = "https://api.mapbox.com/search/geocode/v6/batch"

var NUMBER_REGEX = regexp.MustCompile(`[\d.]+`)

type MapboxPayload struct {
	Types []string `json:"type"`
	Q     string   `json:"q"`
	Limit int      `json:"limit"`
}
type Geometry struct {
	Coordinates []float32 `json:"coordinates"`
}
type Feature struct {
	Geometry Geometry `json:"geometry"`
}
type BatchGeocodingResponse struct {
	Batch []struct {
		Type        string    `json:"type"`
		Features    []Feature `json:"features"`
		Attribution string    `json:"attribution"`
	} `json:"batch"`
}

// ParseHouse gets housing info in HTML from files and parses them to JSON
func ParseHouse() error {
	var homeInfos []*object.HomeInfo
	fmt.Println("Parsing home infos...")

	dirName := "./data/house"
	err := filepath.WalkDir(dirName, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() {
			return nil
		}
		if !d.IsDir() && !strings.HasSuffix(path, ".html") {
			return fmt.Errorf("%s must have only HTML files", dirName)
		}

		// Get the HTML doc from the file
		htmlFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer htmlFile.Close()
		htmlContent, err := goquery.NewDocumentFromReader(htmlFile)
		if err != nil {
			return err
		}

		// Parse the value from the HTML doc
		address, err := getAddress(htmlContent)
		if err != nil {
			// Skip this house iteration if it contains some invalid information
			return nil
		}
		bedrooms, bathrooms, err := getRooms(htmlContent)
		if err != nil {
			return nil
		}
		area, err := getArea(htmlContent)
		if err != nil {
			return nil
		}
		price, err := getPrice(htmlContent)
		if err != nil {
			return nil
		}
		schools, err := getSchools(htmlContent)
		if err != nil {
			return nil
		}

		detailsMap := getDetails(htmlContent)
		homeInfos = append(homeInfos, &object.HomeInfo{
			Id:           primitive.NewObjectID(),
			Address:      address,
			Description:  htmlContent.Find(".remarks").Text(),
			Bedrooms:     bedrooms,
			Bathrooms:    bathrooms,
			HomeArea:     area,
			Price:        price,
			PropertyType: detailsMap["Property Type"].(string),
			YearBuilt:    detailsMap["Year Built"].(string),
			PricePerUnit: detailsMap["Price/Sq.Ft."].(float32),
			LotArea:      detailsMap["Lot Size"].(object.Area),
			HOADues:      detailsMap["HOA Dues"].(float32),
			Parking:      detailsMap["Parking"].(string),
			Schools:      schools,
		})

		return nil
	})

	// Get the coordinates for the parsed houses
	if err = getCoordinates(homeInfos); err != nil {
		return err
	}

	fmt.Printf("Parsed %d home infos completely!\n", len(homeInfos))
	err = writeToFile(homeInfos, "./data/housing.json")
	return err
}

func getAddress(content *goquery.Document) (object.Address, error) {
	addrParts := strings.Split(content.Find(".full-address").Text(), ", ")
	if len(addrParts) < 3 {
		return object.Address{}, fmt.Errorf("Address missing info, which is non-parsable")
	}
	stateAndZip := strings.Split(addrParts[2], " ")

	return object.Address{
		Street:  addrParts[0],
		City:    addrParts[1],
		State:   stateAndZip[0],
		Zipcode: stateAndZip[1],
	}, nil
}

func getRooms(content *goquery.Document) (float32, float32, error) {
	text := content.Find(".beds-section .statsValue").Text()
	bedrooms, err := strconv.ParseFloat(text, 32)
	if err != nil {
		// Houses must have bedrooms and bathrooms
		return 0, 0, err
	}
	text = content.Find(".baths-section .bath-flyout").Text()
	// Extract num because it is displayed together with labels
	bathrooms, err := strconv.ParseFloat(strings.Split(text, " ")[0], 32)
	if err != nil {
		return 0, 0, err
	}

	return float32(bedrooms), float32(bathrooms), nil
}

func getArea(content *goquery.Document) (object.Area, error) {
	// Remove any space among the unit
	text := content.Find(".sqft-section .statsLabel").Text()
	unit := strings.ReplaceAll(content.Find(".sqft-section .statsLabel").Text(), " ", "")

	text = content.Find(".sqft-section .statsValue").Text()
	// Remove the "," from the number to parse
	value, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 32)
	if err != nil {
		// This house's listing has invalid area
		return object.Area{}, err
	}

	return object.Area{
		Unit:  unit,
		Value: float32(value),
	}, nil
}

func getPrice(content *goquery.Document) (float32, error) {
	text := content.Find(".price").Text()
	price, err := strconv.ParseFloat(strings.ReplaceAll(text[1:], ",", ""), 32)
	if err != nil {
		// This house's listing has invalid price
		return 0, err
	}

	return float32(price), nil
}

func getDetails(content *goquery.Document) map[string]any {
	detailsMap := make(map[string]any)
	content.Find(".keyDetails-value").Each(func(i int, s *goquery.Selection) {
		detailsMap[s.Find(".valueType").Text()] = s.Find(".valueText").Text()
	})

	// NOTE: Home is allowed to have missing or invalid details, we will ignore it
	// Parse and cast type of the lot size
	_, ok := detailsMap["Lot Size"]
	if !ok {
		detailsMap["Lot Size"] = object.Area{}
	} else {
		lotSizeParts := strings.Split(detailsMap["Lot Size"].(string), " ")

		// "sqft" is separated as "sq ft" in HTML but "acres" is not
		unit := lotSizeParts[1]
		if len(lotSizeParts) > 2 {
			unit += lotSizeParts[2]
		}
		value, err := strconv.ParseFloat(strings.ReplaceAll(lotSizeParts[0], ",", ""), 32)
		if err != nil {
			detailsMap["Lot Size"] = object.Area{}
		}

		detailsMap["Lot Size"] = object.Area{
			Unit:  unit,
			Value: float32(value),
		}
	}

	// Parse the price per unit
	_, ok = detailsMap["Price/Sq.Ft."]
	if !ok {
		detailsMap["Price/Sq.Ft."] = float32(0)
	} else {
		value, err := strconv.ParseFloat(detailsMap["Price/Sq.Ft."].(string)[1:], 32)
		if err != nil {
			detailsMap["Price/Sq.Ft."] = float32(0.0)
		} else {
			detailsMap["Price/Sq.Ft."] = float32(value)
		}
	}

	// Parse the HOA
	_, ok = detailsMap["HOA Dues"]
	if !ok {
		detailsMap["HOA Dues"] = float32(0)
	} else {
		// Extract the float32 number from it
		numberRegex := regexp.MustCompile(`[\d.]+`)
		text := numberRegex.FindString(detailsMap["HOA Dues"].(string))
		value, err := strconv.ParseFloat(text, 32)
		if err != nil {
			detailsMap["HOA Dues"] = float32(0)
		} else {
			detailsMap["HOA Dues"] = float32(value)
		}
	}

	// Parse the parking
	_, ok = detailsMap["Parking"]
	if !ok {
		detailsMap["Parking"] = ""
	}

	return detailsMap
}

func getSchools(content *goquery.Document) ([]object.School, error) {
	var nearbySchools []object.School
	var err error

	content.Find(".ListItem__content").Each(func(i int, s *goquery.Selection) {
		schoolDescription := strings.Split(s.Find(".ListItem__description").Text(), " • ")
		if len(schoolDescription) < 3 {
			err = fmt.Errorf("Description missing information")
			return
		}
		nearbySchools = append(nearbySchools, object.School{
			Name:     s.Find(".ListItem__heading").Text(),
			Type:     schoolDescription[0],
			Distance: schoolDescription[2],
		})
	})

	return nearbySchools, err
}

// Get coordinates from Mapbox's geocoding service
func getCoordinates(homeInfos []*object.HomeInfo) error {
	var payload []MapboxPayload
	for _, h := range homeInfos {
		addrText := fmt.Sprintf(
			"%s, %s, %s %s",
			h.Address.Street, h.Address.City, h.Address.State, h.Address.Zipcode,
		)
		payload = append(payload, MapboxPayload{
			Types: []string{"address"},
			Q:     addrText,
			Limit: 1,
		})
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	accessToken, ok := os.LookupEnv("MAPBOX_ACCESS_TOKEN")
	if !ok {
		return fmt.Errorf("Mapbox access token not available.")
	}

	url := fmt.Sprintf("%s?access_token=%s", MAPBOX_URL, accessToken)
	response, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	if response != nil && response.StatusCode != 200 {
		return fmt.Errorf("ERROR: Non-200 status is returned, %s", response.Status)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return err
	}

	var results BatchGeocodingResponse
	if err = json.Unmarshal(body, &results); err != nil {
		return err
	}
	for i, homeInfo := range homeInfos {
		coordinates := results.Batch[i].Features[0].Geometry.Coordinates
		homeInfo.Lon = coordinates[0]
		homeInfo.Lat = coordinates[1]
	}
	return nil
}

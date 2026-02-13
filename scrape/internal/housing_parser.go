package scrapeinternal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/mikehquan19/useful-scraper/object"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func parseHomeData() error {
	var homeInfos []object.HomeInfo
	fmt.Println("Parsing home infos...")

	err := filepath.WalkDir("./data", func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() || !strings.HasSuffix(path, ".html") {
			// The entry is a directory or a non-HTML file, we will skip
			// TODO: Find a stronger way to enforce the HTML-only policy in ./data
			return nil
		}
		// Get the HTML doc from the file
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		htmlContent, err := goquery.NewDocumentFromReader(file)
		if err != nil {
			return err
		}

		// Parse the value from the HTML doc
		address, err := getAddress(htmlContent)
		if err != nil {
			// Skip this house if it contains some invalid information
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
		detailsMap := getDetails(htmlContent)
		schools, err := getSchools(htmlContent)
		if err != nil {
			return nil
		}

		homeInfos = append(homeInfos, object.HomeInfo{
			Id:           primitive.NewObjectID(),
			Address:      address,
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
			Contact:      getAgent(htmlContent),
		})

		return nil
	})

	fmt.Printf("Parsed %d home infos completely!\n", len(homeInfos))
	writeToFile(homeInfos, "./data/housing.json")
	return err
}

func getAddress(content *goquery.Document) (object.Address, error) {
	text := content.Find(".full-address").Text()

	addressParts := strings.Split(text, ", ")
	if len(addressParts) < 3 {
		return object.Address{}, fmt.Errorf("Address missing info, which is non-parsable")
	}
	lastParts := strings.Split(addressParts[2], " ")

	return object.Address{
		Street:  addressParts[0],
		City:    addressParts[1],
		State:   lastParts[0],
		Zipcode: lastParts[1],
	}, nil
}

func getRooms(content *goquery.Document) (float32, float32, error) {
	bedrooms, err := strconv.ParseFloat(
		content.Find(".beds-section .statsValue").Text(), 32,
	)
	if err != nil {
		// Absolutely ridiculous that some houses have not bedrooms or bathrooms
		return 0, 0, err
	}
	text := content.Find(".baths-section .bath-flyout").Text()
	bathrooms, err := strconv.ParseFloat(
		// Extract num because it is displayed together with labels
		strings.Split(text, " ")[0], 32,
	)
	if err != nil {
		return 0, 0, err
	}

	return float32(bedrooms), float32(bathrooms), nil
}

func getArea(content *goquery.Document) (object.Area, error) {
	unit := strings.ReplaceAll(
		// Remove any space among the unit
		content.Find(".sqft-section .statsLabel").Text(), " ", "",
	)

	text := content.Find(".sqft-section .statsValue").Text()
	// Remove the , from the number to parse
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
	text := content.Find(".price").Text()[1:]
	price, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", ""), 32)
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
	// Parse type of the lot size
	_, ok := detailsMap["Lot Size"]
	if !ok {
		detailsMap["Lot Size"] = object.Area{}
	} else {
		lotSizeParts := strings.Split(detailsMap["Lot Size"].(string), " ")

		// sqft is separated but acres is not
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
			detailsMap["Price/Sq.Ft."] = float32(0)
		} else {
			detailsMap["Price/Sq.Ft."] = float32(value)
		}
	}

	// Parse the HOA
	_, ok = detailsMap["HOA Dues"]
	if !ok {
		detailsMap["HOA Dues"] = float32(0)
	} else {
		value, err := strconv.ParseFloat(
			// Extract the number from it
			regexp.MustCompile(`[\d.]+`).FindString(detailsMap["HOA Dues"].(string)), 32,
		)
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
		schoolName := s.Find(".ListItem__heading").Text()
		schoolDescription := strings.Split(
			s.Find(".ListItem__description").Text(), " • ",
		)
		if len(schoolDescription) < 3 {
			err = fmt.Errorf("Description missing information")
			return
		}
		nearbySchools = append(nearbySchools, object.School{
			Name:     schoolName,
			Type:     schoolDescription[0],
			Distance: schoolDescription[2],
		})
	})

	return nearbySchools, err
}

func getAgent(content *goquery.Document) object.HomeContact {
	companyContent := content.Find(".agent-basic-details--broker span")
	companyContent.Find(".font-dot").Remove()
	company := strings.TrimSpace(companyContent.Not(".font-dot").Text())

	phoneRegex := regexp.MustCompile(`\b\d{3}-\d{3}-\d{4}\b`)
	phoneNumber := phoneRegex.FindString(
		content.Find(".listingContactSection").Text(),
	)
	return object.HomeContact{
		Realtor:     content.Find(".agent-basic-details--heading span").Text(),
		Company:     company,
		PhoneNumber: phoneNumber,
	}
}

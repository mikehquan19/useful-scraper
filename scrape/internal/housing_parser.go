package internal

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

// ParseHouse gets housing info in HTML from files and parses them to JSON
func ParseHouse(city string) error {
	var homeInfos []object.HomeInfo
	fmt.Println("Parsing home infos...")

	dirName := fmt.Sprintf("./data/house/%s", city)
	err := filepath.WalkDir(dirName, func(path string, d fs.DirEntry, err error) error {
		if d.IsDir() && path == dirName {
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
		// Skip this house iteration if it contains some invalid information
		address, err := getAddress(htmlContent)
		if err != nil {
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
			Contact:      getAgents(htmlContent),
		})

		return nil
	})

	fmt.Printf("Parsed %d home infos completely!\n", len(homeInfos))
	writeToFile(homeInfos, "./data/housing.json")
	return err
}

func getAddress(content *goquery.Document) (object.Address, error) {
	splitAddress := strings.Split(content.Find(".full-address").Text(), ", ")
	if len(splitAddress) < 3 {
		return object.Address{}, fmt.Errorf("Address missing info, which is non-parsable")
	}
	stateAndZip := strings.Split(splitAddress[2], " ")

	return object.Address{
		Street:  splitAddress[0],
		City:    splitAddress[1],
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
	unit := strings.ReplaceAll(content.Find(".sqft-section .statsLabel").Text(), " ", "")

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
		// Extract the number from it
		numberRegex := regexp.MustCompile(`[\d.]+`)
		value, err := strconv.ParseFloat(
			numberRegex.FindString(detailsMap["HOA Dues"].(string)), 32,
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
	var parseErr error

	content.Find(".ListItem__content").Each(func(i int, s *goquery.Selection) {
		schoolDescription := strings.Split(s.Find(".ListItem__description").Text(), " • ")
		if len(schoolDescription) < 3 {
			parseErr = fmt.Errorf("Description missing information")
			return
		}
		nearbySchools = append(nearbySchools, object.School{
			Name:     s.Find(".ListItem__heading").Text(),
			Type:     schoolDescription[0],
			Distance: schoolDescription[2],
		})
	})

	return nearbySchools, parseErr
}

func getAgents(content *goquery.Document) object.HomeContact {
	var realtors, companies string
	content.Find(".listing-agent-item").Each(
		func(i int, s *goquery.Selection) {
			realtors += s.Find(".agent-basic-details--heading span").Text() + ", "

			companyContent := s.Find(".agent-basic-details--broker span")
			companyContent.Find(".font-dot").Remove()

			text := companyContent.Not(".font-dot").Text()
			companies += strings.TrimSpace(text) + ", "
		},
	)

	phoneNumberRegex := regexp.MustCompile(`\b\d{3}-\d{3}-\d{4}\b`)
	phoneNumber := phoneNumberRegex.FindString(content.Find(".listingContactSection").Text())
	return object.HomeContact{
		Realtor:     strings.TrimRight(realtors, ", "),
		Company:     strings.TrimRight(companies, ", "),
		PhoneNumber: phoneNumber,
	}
}

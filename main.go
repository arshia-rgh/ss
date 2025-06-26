package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

type MoodInfo struct {
	Name string
	Link string
}

type FullModel struct {
	Mood  string `json:"mood"`
	Items []Item `json:"items"`
}
type Item struct {
	Name       string `json:"name"`
	ArtistName string `json:"artist_name"`
	Type       string `json:"type"`
	Genre      string `json:"genre"`
	Date       string `json:"date"`
	ItemURL    string `json:"item_url"`
	ImageURL   string `json:"image_url"`
}

const (
	initialSongSaraURL = "https://songsara.net/moods"

	selectTagClassName = "box-i"
	sectionTagName     = "section"
	aTagName           = "a"
	h3TagName          = "h3"
	hrefTagName        = "href"
	imageTagName       = "img"
	srcAtrr            = "src"
)

func main() {
	rabbitUrl := os.Getenv("RABBITMQ_URL")

	ch, conn, err := InitRabbit(rabbitUrl)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	defer ch.Close()
	service, err := NewChromeService()
	if err != nil {
		panic(err)
	}
	defer service.Stop()
	driver, err := InitSelenium(service)
	if err != nil {
		panic(err)
	}
	defer driver.Close()

	err = driver.Get(initialSongSaraURL)
	if err != nil {
		log.Fatal("could not navigate to song", err)
	}

	selectElement, err := driver.FindElement(selenium.ByClassName, selectTagClassName)
	if err != nil {
		log.Fatalf("could not find select element: %v", err)
	}

	moods, err := selectElement.FindElements(selenium.ByTagName, aTagName)
	if err != nil {
		log.Fatalf("could not find 'a' tags within select element: %v", err)
	}

	var moodInfos []MoodInfo
	for _, moodElement := range moods {
		moodNameElement, err := moodElement.FindElement(selenium.ByTagName, h3TagName)
		if err != nil {
			log.Printf("could not find h3 tag for an 'a' tag: %v", err)
			continue
		}
		moodNameText, err := moodNameElement.Text()
		if err != nil {
			log.Printf("could not get text of h3 tag: %v", err)
			continue
		}
		moodLink, err := moodElement.GetAttribute(hrefTagName)
		if err != nil {
			log.Printf("could not get href attribute: %v", err)
			continue
		}
		moodInfos = append(moodInfos, MoodInfo{Name: moodNameText, Link: moodLink})

		for i := 2; ; i++ {
			paginatedURL := moodLink + "/page/" + strconv.Itoa(i) + "/"
			resp, httpErr := http.Get(paginatedURL)
			if httpErr != nil {
				log.Printf("Error checking page %s: %v. Assuming no more pages.", paginatedURL, httpErr)
				break
			}
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				log.Printf("Found paginated URL: %s for mood: %s", paginatedURL, moodNameText)
				moodInfos = append(moodInfos, MoodInfo{Name: moodNameText, Link: paginatedURL})
			} else {
				log.Printf("Page %s not found (status: %d). Stopping pagination for mood: %s.", paginatedURL, resp.StatusCode, moodNameText)
				break
			}
		}

	}

	for _, moodInfo := range moodInfos {
		var mood FullModel
		mood.Mood = moodInfo.Name
		log.Printf("Processing mood: %s, Link: %s", moodInfo.Name, moodInfo.Link)

		err = driver.Get(moodInfo.Link)
		if err != nil {
			log.Printf("could not navigate to mood link %s: %v", moodInfo.Link, err)
			continue
		}

		itemSelection, err := driver.FindElement(selenium.ByClassName, selectTagClassName)
		if err != nil {
			log.Printf("could not find select element on mood page %s: %v", moodInfo.Name, err)
			continue
		}

		itemClasses := "posting col-6 col-sm-4 col-md-3 col-lg-2 col-xl-2"
		cssSelectorForItem := "." + strings.ReplaceAll(itemClasses, " ", ".")
		items, err := itemSelection.FindElements(selenium.ByCSSSelector, cssSelectorForItem)
		if err != nil {
			log.Printf("could not find item elements in mood %s: %v", moodInfo.Name, err)
			continue
		}
		var itemObjects []Item
		for _, itemElement := range items {
			var itemObj Item
			img, imgErr := itemElement.FindElement(selenium.ByTagName, imageTagName)
			if imgErr != nil {
				log.Printf("could not find image tag for item in mood %s: %v", moodInfo.Name, imgErr)
			} else {
				imageURL, attrErr := img.GetAttribute(srcAtrr)
				if attrErr != nil {
					log.Printf("could not get image attribute for item in mood %s: %v", moodInfo.Name, attrErr)
				}
				itemObj.ImageURL = imageURL
			}

			typeClass, err2 := itemElement.FindElements(selenium.ByClassName, "TSale-txt")
			if err2 != nil {
				log.Printf("could not find type class for item in mood %s: %v", moodInfo.Name, typeClass)
				panic(err2)
			}
			spanTypeName, err := typeClass[1].FindElement(selenium.ByTagName, "span")
			if err != nil {
				log.Printf("could not find span class for item in mood %s: %v", moodInfo.Name, typeClass)
				panic(err)
			}
			typeText, err := spanTypeName.Text()
			if err != nil {
				log.Printf("could not get text for item in mood %s: %v", moodInfo.Name, typeClass)
				panic(err)
			}
			itemObj.Type = typeText

			a, aErr := itemElement.FindElement(selenium.ByTagName, aTagName)
			if aErr != nil {
				log.Printf("could not find 'a' tag within item in mood %s: %v", moodInfo.Name, aErr)
			} else {
				itemURL, attrErr := a.GetAttribute(hrefTagName)
				if attrErr != nil {
					log.Printf("could not get href attribute for item in mood %s: %v", moodInfo.Name, attrErr)
				}
				itemObj.ItemURL = itemURL
			}

			detailsSection, sectionErr := itemElement.FindElement(selenium.ByTagName, sectionTagName)
			if sectionErr != nil {
				log.Printf("could not find 'details' section tag within item in mood %s: %v", moodInfo.Name, sectionErr)
			} else {
				details, liErr := detailsSection.FindElements(selenium.ByTagName, "li")
				if liErr != nil {
					log.Printf("could not find 'li' details within item in mood %s: %v", moodInfo.Name, liErr)
				} else {
					if len(details) > 0 {
						itemObj.Name, _ = details[0].Text()
					}
					if len(details) > 1 {
						itemObj.ArtistName, _ = details[1].Text()
					}
					if len(details) > 2 {
						itemObj.Genre, _ = details[2].Text()
					}
					if len(details) > 3 {
						itemObj.Date, _ = details[3].Text()
					}
				}
			}
			log.Printf("Found item: %+v, in mood: %s", itemObj, moodInfo.Name)
			itemObjects = append(itemObjects, itemObj)
		}
		mood.Items = itemObjects
		log.Printf("Publishing mood: %v ", mood)
		err = Publish(ch, "moods", mood, 5*time.Hour)
		if err != nil {
			log.Printf("could not publish moods: %v", err)
		}
	}
}

func InitSelenium(service *selenium.Service) (selenium.WebDriver, error) {
	caps := selenium.Capabilities{}
	chromeCaps := chrome.Capabilities{
		Args: []string{
			"--headless",
			"--no-sandbox",
			"--disable-dev-shm-usage",
			"--disable-gpu",
			"--remote-debugging-port=9222",
		},
	}
	caps.AddChrome(chromeCaps)

	driver, err := selenium.NewRemote(caps, "")
	if err != nil {
		return nil, err
	}

	return driver, nil
}

func NewChromeService() (*selenium.Service, error) {
	return selenium.NewChromeDriverService("./chromedriver", 4444)

}

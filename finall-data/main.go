package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tebeka/selenium"
	"github.com/tebeka/selenium/chrome"
)

const resultsDir = "results"

type Message struct {
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

	channel, err := Consume(ch, "moods", 5*time.Hour)
	if err != nil {
		panic(err)
	}

	for {
		msg, ok := <-channel
		if !ok {
			log.Printf("channel closed")
			if ch.IsClosed() {
				log.Printf("channel is closed too")
			}
			if conn.IsClosed() {
				log.Printf("connection is closed too")
			}
			return
		}
		var message Message
		err := json.Unmarshal(msg.Body, &message)
		if err != nil {
			log.Println("error marshalling message", err)
			_ = msg.Nack(false, true)
			continue
		}

		if message.Mood == "" {
			log.Println("Message has an empty Mood field, skipping.")
			_ = msg.Ack(false)
			continue
		}

		sanitizedMood := strings.ReplaceAll(message.Mood, "/", "-")
		moodPath := filepath.Join(resultsDir, sanitizedMood)

		if err := os.MkdirAll(moodPath, 0755); err != nil {
			log.Printf("Failed to create directory '%s': %v", moodPath, err)
			_ = msg.Nack(false, true)
			continue
		}

		log.Printf("Directory for mood '%s' is ready at '%s'", message.Mood, moodPath)

		var processingFailed bool
		for _, item := range message.Items {
			if err := driver.Get(item.ItemURL); err != nil {
				log.Printf("failed to go to the item url %s: %v", item.ItemURL, err)
				processingFailed = true
				break
			}

			sanitizedType := strings.ReplaceAll(item.Type, "/", "-")
			typePath := filepath.Join(moodPath, sanitizedType)
			if err = os.MkdirAll(typePath, 0755); err != nil {
				log.Printf("failed to create directory `%s`: %v", typePath, err)
				processingFailed = true
				break
			}

			sanitizedItemName := strings.ReplaceAll(item.Name, "/", "-")
			itemPath := filepath.Join(typePath, sanitizedItemName)
			if err = os.MkdirAll(itemPath, 0755); err != nil {
				log.Printf("failed to create directory `%s`: %v", itemPath, err)
				processingFailed = true
				break
			}

			divContains, err := driver.FindElement(selenium.ByID, "aramplayer")
			if err != nil {
				log.Printf("failed to find aramplayer for item %s: %v", item.Name, err)
				processingFailed = true
				break
			}
			ulElement, err := divContains.FindElement(selenium.ByTagName, "ul")
			if err != nil {
				log.Printf("failed to find ul element for item %s: %v", item.Name, err)
				processingFailed = true
				break
			}
			liElements, err := ulElement.FindElements(selenium.ByTagName, "li")
			if err != nil {
				log.Printf("failed to find li elements for item %s: %v", item.Name, err)
				processingFailed = true
				break
			}

			for _, liElement := range liElements {
				title, _ := liElement.GetAttribute("data-title")
				artist, _ := liElement.GetAttribute("data-artist")
				album, _ := liElement.GetAttribute("data-album")
				info, _ := liElement.GetAttribute("data-info")
				image, _ := liElement.GetAttribute("data-image")
				duration, _ := liElement.GetAttribute("data-duration")
				mp3Link, _ := liElement.GetAttribute("data-src")

				track := Track{
					Title:    title,
					Artist:   artist,
					Album:    album,
					Type:     item.Type,
					Genre:    item.Genre,
					Mood:     message.Mood,
					Info:     info,
					Image:    image,
					Duration: duration,
					MP3Link:  mp3Link,
				}

				trackBytes, err := json.Marshal(track)
				if err != nil {
					log.Printf("failed to marshal track: %v", err)
					continue
				}
				sanitizedTitle := strings.ReplaceAll(title, "/", "-")

				fileName := filepath.Join(itemPath, sanitizedTitle+".json")
				if err = os.WriteFile(fileName, trackBytes, 0644); err != nil {
					log.Printf("failed to write to file `%s`: %v", fileName, err)
				}
			}
			if processingFailed {
				break
			}
		}
		if processingFailed {
			log.Printf("Failed to process message for mood '%s', requeueing.", message.Mood)
			_ = msg.Nack(false, true)
		} else {
			log.Printf("Successfully processed message for mood '%s'.", message.Mood)
			_ = msg.Ack(false)
		}
	}
}

type Track struct {
	Title    string `json:"name"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	Type     string `json:"type"`
	Genre    string `json:"genre"`
	Mood     string `json:"mood"`
	Info     string `json:"info"`
	Image    string `json:"img"`
	Duration string `json:"duration"`
	MP3Link  string `json:"mp3_link"`
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

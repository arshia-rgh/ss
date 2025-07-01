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

const resultsDir = "songs"
const dataDir = "data"

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
		publisherPath := filepath.Join(dataDir, "publishers")
		artistPath := filepath.Join(dataDir, "artists")
		genrePath := filepath.Join(dataDir, "genres")
		instrumentPath := filepath.Join(dataDir, "instruments")
		moodDataPath := filepath.Join(dataDir, "mooddata")
		os.MkdirAll(publisherPath, 0755)
		os.MkdirAll(artistPath, 0755)
		os.MkdirAll(genrePath, 0755)
		os.MkdirAll(instrumentPath, 0755)
		os.MkdirAll(moodDataPath, 0755)

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
			artists, err := GetAndSaveArtists(driver, artistPath)
			if err != nil {
				log.Printf("failed to find artists for item %s: %v", item.Name, err)
			}
			_ = driver.Get(item.ItemURL)
			genres, err := GetAndSaveGenre(driver, genrePath)
			if err != nil {
				log.Printf("failed to find genres for item %s: %v", item.Name, err)
			}
			//_ = driver.Get(item.ItemURL)
			moodsName, err := GetAndSaveMood(driver, moodDataPath)
			if err != nil {
				log.Printf("failed to find mood data for item %s: %v", item.Name, err)
			}
			//_ = driver.Get(item.ItemURL)
			pub, err := GetAndSavePublisher(driver, publisherPath)
			if err != nil {
				log.Printf("failed to find publisher for item %s: %v", item.Name, err)
			}

			instruments, err := GetAndSaveInstrument(driver, instrumentPath)
			if err != nil {
				log.Printf("failed to find instruments for item %s: %v", item.Name, err)
			}

			_ = driver.Get(item.ItemURL)

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

			if item.Type == "آلبوم" {
				tracks := make([]AlbumTracks, 0)
				for _, liElement := range liElements {
					title, _ := liElement.GetAttribute("data-title")
					info, _ := liElement.GetAttribute("data-info")
					duration, _ := liElement.GetAttribute("data-duration")
					mp3Link, _ := liElement.GetAttribute("data-src")
					albumTrack := AlbumTracks{
						Title:    title,
						Info:     info,
						Duration: duration,
						MP3Link:  mp3Link,
					}
					tracks = append(tracks, albumTrack)
				}
				album := Album{
					Name:        item.Name,
					Artists:     artists,
					Type:        "album",
					Genres:      genres,
					Moods:       moodsName,
					Instruments: instruments,
					Publisher:   pub,
					Image:       item.ImageURL,
					Tracks:      tracks,
				}
				bytes, err := json.Marshal(album)
				if err != nil {
					log.Printf("failed to marshal album: %v", err)
					continue
				}
				sanitizedName := strings.ReplaceAll(item.Name, "/", "-")

				fileName := filepath.Join(itemPath, sanitizedName+".json")
				err = os.WriteFile(fileName, bytes, 0644)
				if err != nil {
					log.Printf("failed to write file: %v", err)
					continue
				}

			} else {
				for _, liElement := range liElements {
					title, _ := liElement.GetAttribute("data-title")
					artist, _ := liElement.GetAttribute("data-artist")
					album, _ := liElement.GetAttribute("data-album")
					info, _ := liElement.GetAttribute("data-info")
					image, _ := liElement.GetAttribute("data-image")
					duration, _ := liElement.GetAttribute("data-duration")
					mp3Link, _ := liElement.GetAttribute("data-src")

					track := Track{
						Title:       title,
						Artist:      artist,
						Album:       album,
						Type:        item.Type,
						Genres:      genres,
						Moods:       moodsName,
						Instruments: instruments,
						Publisher:   pub,
						Info:        info,
						Image:       image,
						Duration:    duration,
						MP3Link:     mp3Link,
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

type Album struct {
	Name        string        `json:"name"`
	Artists     []string      `json:"artists"`
	Type        string        `json:"type"`
	Genres      []string      `json:"genres"`
	Moods       []string      `json:"moods"`
	Instruments []string      `json:"instruments"`
	Publisher   string        `json:"publisher"`
	Image       string        `json:"img"`
	Tracks      []AlbumTracks `json:"tracks"`
}

type AlbumTracks struct {
	Title    string `json:"title"`
	Info     string `json:"info"`
	Duration string `json:"duration"`
	MP3Link  string `json:"mp3_link"`
}

type Track struct {
	Title       string   `json:"name"`
	Artist      string   `json:"artist"`
	Album       string   `json:"album"`
	Type        string   `json:"type"`
	Genres      []string `json:"genres"`
	Moods       []string `json:"moods"`
	Instruments []string `json:"instruments"`
	Publisher   string   `json:"publisher"`
	Info        string   `json:"info"`
	Image       string   `json:"img"`
	Duration    string   `json:"duration"`
	MP3Link     string   `json:"mp3_link"`
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

type Artist struct {
	NameEN      string `json:"name_en"`
	NameFA      string `json:"name_fa"`
	Description string `json:"description"`
	Img         string `json:"img"`
}

func GetAndSaveArtists(driver selenium.WebDriver, path string) ([]string, error) {
	artistDiv, err := driver.FindElement(selenium.ByClassName, "AR-Si")
	if err != nil {
		return nil, err
	}
	artistAtags, err := artistDiv.FindElements(selenium.ByTagName, "a")
	if err != nil {
		return nil, err
	}
	artistENTitles := make([]string, 0)
	for _, artist := range artistAtags {
		artistTitleFA, _ := artist.GetAttribute("title")
		artistTitleEN, _ := artist.Text()

		artistLink, err := artist.GetAttribute("href")
		if err != nil {
			return nil, err
		}
		err = driver.Get(artistLink)
		if err != nil {
			return nil, err
		}
		imageDiv, err := driver.FindElement(selenium.ByClassName, "artist-img")
		if err != nil {
			return nil, err
		}
		imgTag, err := imageDiv.FindElement(selenium.ByTagName, "img")
		if err != nil {
			return nil, err
		}
		img, err := imgTag.GetAttribute("src")
		if err != nil {
			return nil, err
		}

		descriptionTag, err := driver.FindElement(selenium.ByClassName, "h3-artist")
		if err != nil {
			return nil, err
		}
		var description string
		descriptionP, err := descriptionTag.FindElement(selenium.ByTagName, "p")
		if err != nil {
			log.Printf("description is empty for the artist")
		} else {
			description, err = descriptionP.Text()
			if err != nil {
				return nil, err
			}
		}

		artistENTitles = append(artistENTitles, artistTitleEN)

		artistOBJ := Artist{
			NameEN:      artistTitleEN,
			NameFA:      artistTitleFA,
			Description: description,
			Img:         img,
		}

		artistByte, err := json.Marshal(artistOBJ)
		if err != nil {
			return nil, err
		}
		sanitizedName := strings.ReplaceAll(artistTitleEN, "/", "-")

		fileName := filepath.Join(path, sanitizedName+".json")
		err = os.WriteFile(fileName, artistByte, 0644)
		if err != nil {
			return nil, err
		}

	}
	return artistENTitles, nil

}

type Instrument struct {
	NameEN      string `json:"name_en"`
	NameFA      string `json:"name_fa"`
	Description string `json:"description"`
}

func GetAndSaveInstrument(driver selenium.WebDriver, path string) ([]string, error) {
	instrumentDiv, err := driver.FindElement(selenium.ByClassName, "instrument-Si")
	if err != nil {
		return nil, err
	}
	instrumentAtags, err := instrumentDiv.FindElements(selenium.ByTagName, "a")
	if err != nil {
		return nil, err
	}
	instrumentENTitles := make([]string, 0)
	for _, instrument := range instrumentAtags {
		instrumentTitleFA, _ := instrument.GetAttribute("title")
		instrumentTitleEN, err := instrument.Text()
		if err != nil {
			log.Printf("instrument text is empty for the instrument %s, %s", instrumentTitleFA, err)
		}
		instrumentLink, err := instrument.GetAttribute("href")
		if err != nil {
			return nil, err
		}
		err = driver.Get(instrumentLink)
		if err != nil {
			return nil, err
		}

		descriptionTag, err := driver.FindElement(selenium.ByClassName, "h3-artist")
		if err != nil {
			return nil, err
		}
		descriptionP, err := descriptionTag.FindElement(selenium.ByTagName, "p")
		if err != nil {
			return nil, err
		}
		description, err := descriptionP.Text()
		if err != nil {
			return nil, err
		}
		//splited := strings.Split(strings.TrimSuffix(instrumentLink, "/"), "/")

		instrumentENTitles = append(instrumentENTitles, instrumentTitleEN)

		instrumentOBJ := Instrument{
			NameEN:      instrumentTitleEN,
			NameFA:      instrumentTitleFA,
			Description: description,
		}

		instrumentByte, err := json.Marshal(instrumentOBJ)
		if err != nil {
			return nil, err
		}
		sanitizedName := strings.ReplaceAll(instrumentTitleEN, "/", "-")

		fileName := filepath.Join(path, sanitizedName+".json")
		err = os.WriteFile(fileName, instrumentByte, 0644)
		if err != nil {
			return nil, err
		}

	}
	return instrumentENTitles, nil

}

type Genre struct {
	NameEN string `json:"name_en"`
	NameFA string `json:"name_fa"`
}

func GetAndSaveGenre(driver selenium.WebDriver, path string) ([]string, error) {
	genreDiv, err := driver.FindElement(selenium.ByClassName, "genre-Si")
	if err != nil {
		return nil, err
	}
	genreAtags, err := genreDiv.FindElements(selenium.ByTagName, "a")
	if err != nil {
		return nil, err
	}
	genreENTitles := make([]string, 0)
	for _, genre := range genreAtags {
		genreFA, err := genre.GetAttribute("title")
		if err != nil {
			return nil, err
		}
		genreEN, err := genre.Text()
		if err != nil {
			return nil, err
		}
		genreENTitles = append(genreENTitles, genreEN)
		genreOBJ := Genre{
			NameEN: genreFA,
			NameFA: genreEN,
		}
		bytes, err := json.Marshal(genreOBJ)
		if err != nil {
			return nil, err
		}
		sanitizedName := strings.ReplaceAll(genreEN, "/", "-")
		fileName := filepath.Join(path, sanitizedName+".json")

		err = os.WriteFile(fileName, bytes, 0644)
		if err != nil {
			return nil, err
		}

	}

	return genreENTitles, nil
}

type Publisher struct {
	NameEN string `json:"name_en"`
}

func GetAndSavePublisher(driver selenium.WebDriver, path string) (string, error) {
	elements, err := driver.FindElements(selenium.ByClassName, "pub-Si")
	if err != nil {
		return "", err
	}
	if len(elements) == 0 {
		log.Printf("no publishers ")
		return "", err
	}
	text, err := elements[0].Text()
	if err != nil {
		return "", err
	}
	sanitizedName := strings.ReplaceAll(text, "/", "-")
	fileName := filepath.Join(path, sanitizedName+".json")
	pub := Publisher{NameEN: text}
	bytes, err := json.Marshal(pub)
	if err != nil {
		return "", nil
	}
	err = os.WriteFile(fileName, bytes, 0644)
	if err != nil {
		return "", err
	}
	return text, nil
}

type MoodData struct {
	NameFA string `json:"name_fa"`
	NameEN string `json:"name_en"`
}

func GetAndSaveMood(driver selenium.WebDriver, path string) ([]string, error) {
	moodDiv, err := driver.FindElement(selenium.ByClassName, "mood-Si")
	if err != nil {
		return nil, err
	}
	moodAtags, err := moodDiv.FindElements(selenium.ByTagName, "a")
	if err != nil {
		return nil, err
	}
	moodENTitles := make([]string, 0)
	for _, mood := range moodAtags {
		moodFA, err := mood.GetAttribute("title")
		if err != nil {
			return nil, err
		}
		moodEN, err := mood.Text()
		if err != nil {
			return nil, err
		}
		moodENTitles = append(moodENTitles, moodEN)
		moodOBJ := MoodData{
			NameEN: moodFA,
			NameFA: moodEN,
		}
		bytes, err := json.Marshal(moodOBJ)
		if err != nil {
			return nil, err
		}
		sanitizedName := strings.ReplaceAll(moodEN, "/", "-")
		fileName := filepath.Join(path, sanitizedName+".json")

		err = os.WriteFile(fileName, bytes, 0644)
		if err != nil {
			return nil, err
		}

	}

	return moodENTitles, nil
}

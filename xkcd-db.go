package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
)

const (
	xkcdURL  = "https://xkcd.com/"
	jsonFile = "info.0.json"
)

// Transcript and Alt are needed for searching.
type Comic struct {
	Num        int
	Img        string
	Transcript string
	Alt        string
}

func main() {
	rateLimit := flag.Int64("r", 20, "Set the maximum number of parallel downloads")
	dbPath := flag.String("d", "./xkcdDB/", "Specify the path where the database should be built")
	flag.Parse()

	// Add trailing /
	if (*dbPath)[len(*dbPath)-1] != '/' {
		*dbPath = *dbPath + "/"
	}

	// The latest comic is used to find the number of comics.
	numComics, err := latestComicNum()
	if err != nil {
		log.Fatalln(err)
	}

	_, err = os.Stat(*dbPath)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist. Creating...\n", *dbPath)
		err = os.Mkdir(*dbPath, 0755)
		if err != nil {
			log.Fatalln(err)
		}
	}

	missing := missingComics(numComics, *dbPath)

	if len(missing) == 0 {
		fmt.Println("Found no missing comics")
		return
	}

	// Counting semaphore.
	tokens := make(chan struct{}, *rateLimit)

	getComic(missing, *dbPath, tokens)
	fmt.Printf("Downloaded %d missing comics\n", len(missing))
}

func latestComicNum() (int, error) {
	url := xkcdURL + jsonFile
	resp, err := http.Get(url)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	decoder := json.NewDecoder(resp.Body)
	var comicData Comic
	err = decoder.Decode(&comicData)
	if err != nil {
		return 0, err
	}

	return comicData.Num, nil
}

func missingComics(numComics int, dbPath string) []string {
	dlList := make([]string, 0, numComics)

	for i := 1; i <= numComics; i++ {
		// xkcd 404 doesn't exist.
		if i == 404 {
			continue
		}

		comicPath := dbPath + strconv.Itoa(i)

		_, err := os.Stat(comicPath)
		if os.IsNotExist(err) {
			dlList = append(dlList, strconv.Itoa(i))
		}
	}

	return dlList
}

// Tokens is a channel that acts as a counting semaphore.
func getComic(dlList []string, dbPath string, tokens chan struct{}) {
	var wg sync.WaitGroup

	for _, item := range dlList {

		// Start data fetching workers for missing comics.
		wg.Add(1)
		go func(item string) {
			// Aquire a token.
			tokens <- struct{}{}
			// Release the token.
			defer func() { <-tokens }()
			defer wg.Done()

			fmt.Printf("Fetching Comic #%s ...\n", item)

			// Fetch comic metadata.
			var comicData Comic
			url := xkcdURL + item + "/" + jsonFile

			resp, err := http.Get(url)
			if err != nil {
				log.Println(err)
				return
			}
			defer resp.Body.Close()

			decoder := json.NewDecoder(resp.Body)
			err = decoder.Decode(&comicData)
			if err != nil {
				fmt.Printf("JSON decoding error: Comic %s\n", item)
				log.Println(err)
				return
			}

			// Write metadata files.
			savePath := dbPath + item + "/"

			err = os.Mkdir(savePath, 0755)
			if err != nil {
				log.Fatalln(err)
			}

			// Write alt data if it exists.
			if comicData.Alt != "" {
				alt, err := os.Create(savePath + item + "-alt")
				if err != nil {
					log.Fatalln(err)
				}
				defer alt.Close()

				_, writeErr := alt.WriteString(comicData.Alt)
				if writeErr != nil {
					log.Fatalln(err)
				}
			}

			// Write transcript data if it exists.
			if comicData.Transcript != "" {
				transcript, err := os.Create(savePath + item + "-transcript")
				if err != nil {
					log.Fatalln(err)
				}
				defer transcript.Close()

				_, writeErr := transcript.WriteString(comicData.Transcript)
				if writeErr != nil {
					log.Fatalln(err)
				}
			}

			// Write image files.
			imgResp, err := http.Get(comicData.Img)
			if err != nil {
				log.Println(err)
				return
			}
			defer imgResp.Body.Close()

			splitUrl := strings.Split(comicData.Img, "/")
			imgName := splitUrl[len(splitUrl)-1]

			if imgName == "" {
				fmt.Printf("Comic %s has no image.\n", item)
				return
			}

			imgPath := savePath + imgName

			img, err := os.Create(imgPath)
			if err != nil {
				log.Fatalln(err)
			}
			defer img.Close()

			_, err = io.Copy(img, imgResp.Body)
			if err != nil {
				log.Fatalln(err)
			}
		}(item)
	}

	wg.Wait()
}

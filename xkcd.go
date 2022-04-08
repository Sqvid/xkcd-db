package main

import (
	"encoding/json"
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
	dbPath   = "./xkcdDB/"
)

// Transcript and Alt are needed for searching.
type Comic struct {
	Num        int
	Img        string
	Transcript string
	Alt        string
}

func main() {
	log.SetFlags(log.Llongfile)
	// The latest comic is used to find the number of comics.
	numComics, err := latestComicNum()
	if err != nil {
		log.Fatalln(err)
	}

	rateLimit := 200

	_, err = os.Stat(dbPath)
	if os.IsNotExist(err) {
		fmt.Printf("%s does not exist. Creating...\n", dbPath)
		err = os.Mkdir(dbPath, 0755)
		if err != nil {
			log.Fatalln(err)
		}
	}

	missing := missingComics(numComics)

	if len(missing) == 0 {
		fmt.Println("Found no missing comics")
		return
	}

	fmt.Printf("Found %d missing comics\n", len(missing))

	// Divide missing comic numbers into blocks. This is prevent exceeding
	// the socket limit.
	dlList := splitSlice(missing, rateLimit)

	for _, comics := range dlList {
		getComic(comics)
	}
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

func missingComics(numComics int) []string {
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

// Split a slice into smaller blocks and return them.
func splitSlice(input []string, blockSize int) [][]string {
	var nFullBlocks int = len(input) / blockSize
	rem := len(input) % blockSize

	output := make([][]string, 0, nFullBlocks+1)

	for block := 0; block < nFullBlocks; block++ {
		index := block * blockSize
		output = append(output, input[index:index+blockSize])
	}

	// Handle remainder if any
	if rem != 0 {
		index := nFullBlocks * blockSize
		output = append(output, input[index:index+rem])
	}

	return output
}

func getComic(dlList []string) {
	var wg sync.WaitGroup

	for _, item := range dlList {
		wg.Add(1)

		// Start data fetching workers for missing comics.
		go func(item string) {
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

			alt, err := os.Create(savePath + item + "-alt")
			if err != nil {
				log.Fatalln(err)
			}
			defer alt.Close()

			_, writeErr := alt.WriteString(comicData.Alt)
			if writeErr != nil {
				log.Fatalln(err)
			}

			transcript, err := os.Create(savePath + item + "-transcript")
			if err != nil {
				log.Fatalln(err)
			}
			defer transcript.Close()

			_, writeErr = transcript.WriteString(comicData.Transcript)
			if writeErr != nil {
				log.Fatalln(err)
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

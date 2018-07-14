package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/lfourky/definitely-not-an-illegal-book-downloader/workers"
)

var (
	numWorkers   = flag.Uint("workerCount", 10, "Number of concurrent workers")
	allCores     = flag.Bool("allCores", false, "Set to true if you want to use all processor cores for this.")
	startingPage = flag.Int("page", 1, "Default is to start from first page, set it to some other page if you'd like to continue where you left of.")
	continueWork = flag.Bool("continue", false, "Set to true if you want to continue from where you left of (last saved page number).")
	fastMode     = flag.Bool("fast", false, "Set this to true if you want to risk some books not persisting if you violently stop the program.")
)

const (
	// Once we get to a page that contains this sentence, then we know we've hit the end
	endReachedSentence = "No Posts Found."

	bookLimitPerPage = 10

	// This is where the books will be stored
	baseDirPath = "allitebooks"

	lastPageNumberFile = "lastpagenumber.txt"

	defaultCategoryName = "Random"
)

var (
	// Provide page urls to this channel in order for them to get scanned for the target (download) links
	bookPages chan string

	workPool chan workers.Work

	slowModeWG sync.WaitGroup
	workWG     sync.WaitGroup
)

var (
	// This will provide us with individual book page links (not the actual download link this time,
	// but the actual download link for a single book will be on that page).
	rgxBookPage = regexp.MustCompile(`<h2 class="entry-title"><a href="(.*)" rel="bookmark"`)

	// This will provide us with the actual download link
	rgxBookDownloadLink = regexp.MustCompile(`<a href="(http://file.allitebooks.com/.*.pdf)" target="_blank">`)

	// This will provide us with the category for the book we found
	rgxBookCategoryFinder = regexp.MustCompile(`<dt>Category:</dt>.*>(.*)</a></dd>`)
)

func main() {
	flag.Parse()

	if *allCores {
		log.Print("Using all cores! /flex")
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	if *continueWork {
		data, err := ioutil.ReadFile(lastPageNumberFile)
		if err == nil {
			lastPageNumber, err := strconv.Atoi(string(data))
			if err != nil {
				log.Printf("Invalid content in %s: %s", lastPageNumberFile, data)
				os.Exit(1)
			}

			// If we've made it this far, override the startingPage param
			*startingPage = lastPageNumber
		}
	}

	if _, err := os.Stat(baseDirPath); os.IsNotExist(err) {
		if err := os.Mkdir(baseDirPath, os.ModePerm); err != nil {
			log.Printf("Error creating base directory: %s", err)
			os.Exit(1)
		}
	}

	currentPageFile, err := os.OpenFile(lastPageNumberFile, os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Printf("Error creating / reading from %s: %s", lastPageNumberFile, err)
		os.Exit(1)
	}
	defer currentPageFile.Close()

	workPool = workers.InitializeWorkers(int(*numWorkers), bookLimitPerPage)

	bookPages = make(chan string, bookLimitPerPage)
	go processBookPages()

	baseURL := "http://www.allitebooks.com"

	// Let's get that pagination going
	baseURL += "/page/"

	// Infinite loop to get through all the pages; stop only on errors or if previously defined page end content is encountered
	for currentPageNumber := *startingPage; ; currentPageNumber++ {
		currentURL := fmt.Sprintf("%s%d", baseURL, currentPageNumber)
		response, err := http.Get(currentURL)
		if err != nil {
			//Shouldn't really continue if we get an error here
			log.Printf("Error fetching the main page: %s", err)
			os.Exit(1)
		}

		log.Printf("\n\n")
		log.Printf("----------------------")
		log.Printf("Currently at %s", currentURL)
		log.Printf("----------------------\n\n")

		sourceBytes, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Printf("Error reading the source code from the main page: %s", err)
			continue
		}

		response.Body.Close()

		sourceCode := string(sourceBytes)
		if strings.Contains(sourceCode, endReachedSentence) {
			log.Printf("The website reports that this page (%s) doesn't contain any books.", currentURL)
			break
		}

		if !*fastMode {
			// Before we proceed (and only in slow mode), let's update the lastPageNumber file
			err = ioutil.WriteFile(lastPageNumberFile, []byte(strconv.Itoa(currentPageNumber)), os.ModePerm)
			if err != nil {
				log.Printf("Error writing to %s: %s", lastPageNumberFile, err)
				break
			}
		}

		// Alright, it seems like we might have some books here!
		match := rgxBookPage.FindAllSubmatch(sourceBytes, -1)
		individualBookPages := make([]string, 0, 10)
		for i := 0; i < 10 && i < len(match); i++ {
			if len(match[i]) > 0 {
				individualBookPages = append(individualBookPages, string(match[i][1]))
			}
		}

		if len(individualBookPages) < 1 {
			log.Printf("Couldn't find any books on this page: (%s)", currentURL)
			break
		}

		// Cool! We've made it this far, which means we've found some book pages.

		// First, let's allow only 1 page to be downloaded at time, to ensure corectness if a program is stopped at some point
		// and then rerun with --continue flag
		if !*fastMode {
			slowModeWG.Add(len(individualBookPages))
		}

		// Increment this counter, since we want to wait for all the book to finish downloading before the program exits.
		workWG.Add(len(individualBookPages))

		// Let's send them off to a bookPage channel and process them from there, but continue
		// fetching more book pages here as well (unless we're processing 10 books at this very moment)
		for _, bookPage := range individualBookPages {
			bookPages <- bookPage
		}

		if !*fastMode {
			log.Printf("Waiting for page [%d] to finish downloading all the books...", currentPageNumber)
			slowModeWG.Wait()
		}
	}

	log.Print("Waiting for all the book to finish downloading.")
	workWG.Wait()
	log.Print("All done! Enjoy and don't be lazy. Actually read some of them. :)")
}

func processBookPages() {
	for {
		bookPage, ok := <-bookPages
		if !ok {
			return
		}

		//Process them further untill we get the download link (which is our goal here)
		response, err := http.Get(bookPage)
		if err != nil {
			//Shouldn't really continue if we get an error here
			log.Printf("Error fetching a book page %s - %s", bookPage, err)
			continue
		}

		sourceBytes, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Printf("Error reading the source code the main page: %s", err)
			continue
		}

		response.Body.Close()

		match := rgxBookDownloadLink.FindSubmatch(sourceBytes)
		if len(match) == 0 {
			continue
		}

		// Bingo! We've got a book download URL. Let's download/process it in another place, and continue with what we're doing here.
		downloadURL := string(match[1])

		// Wait! Before that, let's also discover the category so we can place it in the corresponding directory.
		category := defaultCategoryName
		categoryMatch := rgxBookCategoryFinder.FindSubmatch(sourceBytes)
		if len(categoryMatch) != 0 {
			category = strings.Replace(string(categoryMatch[1]), "&amp;", "&", -1)
		}

		// Create the directory structure for a book to be stored. For example: allitebooks/databases/some-book-file.pdf
		bookDirPath := path.Join(baseDirPath, category)
		if _, err := os.Stat(bookDirPath); os.IsNotExist(err) {
			if err := os.Mkdir(bookDirPath, os.ModePerm); err != nil {
				log.Printf("Error creating book (sub)directory: %s", err)
				os.Exit(1)
			}
		}

		lastSlashIndex := strings.LastIndex(downloadURL, "/")

		bookFilePath := path.Join(bookDirPath, downloadURL[lastSlashIndex:])

		workPool <- workers.Work{URL: downloadURL, Filename: bookFilePath, SlowModeSync: getSlowModeWaitGroup(), WorkSync: &workWG}
	}
}

func getSlowModeWaitGroup() *sync.WaitGroup {
	if *fastMode {
		return nil
	}
	return &slowModeWG
}

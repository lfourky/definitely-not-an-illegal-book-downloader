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
	"strings"
	"sync"
)

type book struct {
	url           string
	category      string
	downloadLinks []string
}

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

	baseURL = "http://www.allitebooks.com/page/"

	// This is where the books will be stored
	baseDirPath = "allitebooks"

	// lastPageNumberFile = "lastpagenumber.txt"

	defaultCategoryName = "Random"
)

var (
	// workPool chan Work

	slowModeWG sync.WaitGroup
	workWG     sync.WaitGroup
)

var (
	// This will provide us with individual book page links (not the actual download link this time,
	// but the actual download link for a single book will be on that page).
	rgxBookPage = regexp.MustCompile(`<h2 class="entry-title"><a href="(?P<PAGE>.*?)"`)

	// This will provide us with the actual download link.
	rgxBookDownloadLink = regexp.MustCompile(`<a href="(?P<LINK>http://file\.allitebooks\.com/.+?(\.(?:pdf|epub)))" target="_blank">`)

	// This will provide us with the category for the book we found.
	rgxBookCategoryFinder = regexp.MustCompile(`rel="category"\s*?>(?P<CATEGORY>.*?)</a>`)
)

func main() {
	flag.Parse()

	if *allCores {
		log.Print("Using all cores! /flex")
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	if err := os.MkdirAll(baseDirPath, os.ModePerm); err != nil {
		log.Fatalf("Error creating base directory: %s", err)
	}

	updateLinkDatabaseHeaders()

	chanBooks := make(chan book, 100)
	quitChanBooks := make(chan bool)
	go processBooks(chanBooks, nil, quitChanBooks)

	quitChanBookPages := make(chan bool)
	chanBookPages := make(chan string, 100)
	go processIndividualBookPages(chanBookPages, chanBooks, quitChanBookPages)

	fetchIndividualBookPages(*startingPage, chanBookPages, []chan bool{quitChanBookPages, quitChanBooks})
}

// Goes through each page on the website and gather individual book pages by sending them to bookPages channel.
func fetchIndividualBookPages(startFrom int, bookPages chan<- string, quitChans []chan bool) {
	// Infinite loop that breaks if no books were found on a page.
	for currentPageNumber := startFrom; ; currentPageNumber++ {
		currentURL := fmt.Sprintf("%s%d", baseURL, currentPageNumber)

		// Book index page contains around 10 individual books.
		bookIndexPage, err := http.Get(currentURL)
		if err != nil {
			log.Fatalf("Error fetching the book index page: %s", err)
		}

		sourceBytes, err := ioutil.ReadAll(bookIndexPage.Body)
		if err != nil {
			log.Fatalf("Error reading the source code from the book index page: %s", err)
		}

		if err := bookIndexPage.Body.Close(); err != nil {
			log.Printf("Error closing bookIndexPage's response body: %s", err)
		}

		// TODO: extract named capture group
		match := rgxBookPage.FindAllSubmatch(sourceBytes, -1)
		individualBookPages := make([]string, 0, 10)
		for i := 0; i < 10 && i < len(match); i++ {
			if len(match[i]) > 0 {
				individualBookPages = append(individualBookPages, string(match[i][1]))
			}
		}

		if len(individualBookPages) < 1 {
			log.Printf("Couldn't find any books on page: (%s)", currentURL)
			break
		}

		for _, page := range individualBookPages {
			bookPages <- page
		}
	}

	for _, quitChan := range quitChans {
		quitChan <- true
	}
}

func processIndividualBookPages(bookPages <-chan string, books chan<- book, quitChan <-chan bool) {
	for {
		select {
		case page := <-bookPages:
			b := book{
				url: page,
			}

			response, err := http.Get(b.url)
			if err != nil {
				log.Printf("Error fetching book page %s - %s", b.url, err)
				continue
			}

			sourceBytes, err := ioutil.ReadAll(response.Body)
			if err != nil {
				log.Printf("Error reading the source code from book page: %s", err)
				continue
			}

			if err := response.Body.Close(); err != nil {
				log.Printf("Error closing response body for book page %s - %s", b.url, err)
			}

			downloadLinks := extractBookDownloadLinks(string(sourceBytes), rgxBookDownloadLink)
			if len(downloadLinks) == 0 {
				continue
			}

			category := defaultCategoryName
			categoryMatch := rgxBookCategoryFinder.FindSubmatch(sourceBytes)
			if len(categoryMatch) != 0 {
				category = strings.Replace(string(categoryMatch[1]), "&amp;", "&", -1)
			}
			b.category = category

			b.downloadLinks = make([]string, len(downloadLinks))
			for i, link := range downloadLinks {
				b.downloadLinks[i] = link
			}

			// Send to books chan for further processing.
			books <- b

		case <-quitChan:
			log.Printf("QUIT done processing individual book pages")
			return // todo rethink?
		}
	}
}

func processBooks(books <-chan book, workers chan bool, quitChan <-chan bool) {
	createdSubdirectories := map[string]bool{}

	for {
		select {
		case b := <-books:
			bookDirPath := path.Join(baseDirPath, b.category)
			if exists := createdSubdirectories[bookDirPath]; !exists {
				if err := os.MkdirAll(bookDirPath, os.ModePerm); err != nil {
					log.Fatalf("Error creating book (sub)directory: %s", err)
				}
				createdSubdirectories[bookDirPath] = true
			}

			for _, link := range b.downloadLinks {
				// lastSlashIndex := strings.LastIndex(url, "/")
				// bookFilePath := path.Join(bookDirPath, url[lastSlashIndex:])
				updateLinkDatabase(b.url, link, b.category)
			}

		case <-quitChan:
			log.Printf("QUIT done processing books")
			return // todo rethink?
		}
	}
}

func updateLinkDatabaseHeaders() {
	if err := ioutil.WriteFile("links.csv", []byte(fmt.Sprintln(`"Page","Link","Category"`)), os.ModePerm); err != nil {
		log.Fatal(err)
	}
}

func updateLinkDatabase(bookPage, link, category string) {
	f, err := os.OpenFile("links.csv", os.O_APPEND|os.O_CREATE|os.O_WRONLY, os.ModePerm)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	if _, err := f.WriteString(fmt.Sprintln(fmt.Sprintf(`"%s","%s","%s"`, bookPage, link, category))); err != nil {
		log.Fatal(err)
	}
}

// (...)1a (...)2b (...)3c                    (turn a regex with named captures)
// => []string{"1a", "2b", "3c'"}             (into a slice of names of named captures)
// => map["1a"]=1, map["2b"]=2, map["3c"]=3   (and then into a map that maps the name to it's index in the slice)
func namedCaptureIndexes(r *regexp.Regexp) map[string]int {
	m := map[string]int{}
	for i, name := range r.SubexpNames() {
		if name == "" {
			continue
		}
		m[name] = i
	}
	return m
}

func extractBookCategory(pageSourceCode string, r *regexp.Regexp) string {
	category := defaultCategoryName

	match := rgxBookCategoryFinder.FindStringSubmatch(pageSourceCode)
	if len(match) != 0 {
		namedCapture := namedCaptureIndexes(r)
		category = match[namedCapture["CATEGORY"]]
		category = strings.Replace(category, "&amp;", "&", -1)
	}

	return category
}

func extractBookDownloadLinks(pageSourceCode string, r *regexp.Regexp) []string {
	match := r.FindAllStringSubmatch(pageSourceCode, -1)
	if len(match) == 0 {
		return []string{}
	}

	namedCapture := namedCaptureIndexes(r)

	downloadLinks := make([]string, len(match))
	for i := range match {
		downloadLinks[i] = match[i][namedCapture["LINK"]]
	}
	return downloadLinks
}

func getSlowModeWaitGroup() *sync.WaitGroup {
	if *fastMode {
		return nil
	}
	return &slowModeWG
}

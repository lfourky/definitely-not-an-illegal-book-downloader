package main

import (
	"io"
	"log"
	"net/http"
	"os"
	"sync"
)

type worker struct {
	work       chan Work
	workerPool chan *worker
}

// Work holds the necessary info for book downloading and persisting, but also a waitGroup so a corectness of the program is ensured
type Work struct {
	URL          string
	Filename     string
	SlowModeSync *sync.WaitGroup
	WorkSync     *sync.WaitGroup
}

func (w *worker) start() {
	for {
		work, ok := <-w.work
		if !ok {
			return
		}

		if err := download(work.URL, work.Filename); err != nil {
			log.Printf("Error downloading/saving url: %s err: %s", work.URL, err)
		}

		//After this worker is done working, we need to put him back and signalize the sync barrier that we completed our work
		if work.SlowModeSync != nil {
			work.SlowModeSync.Done()
		}

		work.WorkSync.Done()

		w.workerPool <- w
	}
}

var (
	//workerPool represents a pool of available workers
	workerPool chan *worker
)

// InitializeWorkers returns a job channel to which we can put data
func InitializeWorkers(numWorkers, maxWorkCount int) chan Work {
	workerPool = make(chan *worker, numWorkers)
	for i := 0; i < numWorkers; i++ {
		w := &worker{work: make(chan Work), workerPool: workerPool}
		workerPool <- w
		go w.start()
	}

	workPool := make(chan Work, maxWorkCount)
	go waitForWork(workPool)

	return workPool
}

func waitForWork(workPool <-chan Work) {
	for {
		work, ok := <-workPool
		if !ok {
			return
		}

		// Work arrived, so let's see if we have available workers to handle the work!
		worker := <-workerPool

		// There's a worker available! Send some work his way...
		worker.work <- work
	}
}

func download(url string, filename string) error {
	log.Printf("Downloading %s", url)
	res, err := http.Get(url)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	return save(res.Body, filename)
}

func save(data io.Reader, filePath string) error {
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, data)
	return err
}

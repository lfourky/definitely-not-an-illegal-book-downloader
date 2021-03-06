# definitely-not-an-illegal-book-downloader
This program does not download all 7593 tech-related books from http://www.allitebooks.com/ once you run it, that's for sure.

## Getting the program
In order to run this program, you'll have to have Go language installed & setup on your computer.
The next step is to open up a command prompt and type:
```
go get github.com/lfourky/definitely-not-an-illegal-book-downloader
```
and then move into your $GOPATH directory, and into the directory of the program:
```
cd $GOPATH/src/github.com/lfourky/definitely-not-an-illegal-book-downloader/
```
and after that, you can build the program with: 
```
go build main.go worker.go
```
And voila. You'll get a binary in your current directory, and you can run it and pass flags to it.
The next set of examples assume that you're running on Windows, but it's pretty much the same with other platforms.

## (Not) Running the program (not an example)
So, you've decided not to trust me, and you want to run the program? Fine. 
Be aware of this: if you use ```-fast``` mode, the pages will not be tracked internally, and it makes no sense to use ```-continue``` flag to continue from where you left of if you stop the program mid-execution. But it's fast. Really fast. Depending on the speed of your internet connection, of course.

If you want consistency and correctness, be sure not to use ```-fast``` flag, and then the program will keep track of which page you last tried to download books from. That way, if you have to stop the program mid-execution, next time you want to continue downloading and ensure that you will have all the books, use the ```-continue``` flag when running the program.

Run with all cores (make it parallel) and continue from where you left of:
```
./main.exe -continue -allCores
```

Start downloading books from page 30.
```
./main.exe -page 30
```

Use 30 concurrent workers. (It doesn't make much sense, but hey, it's your life and your computer)
```
./main.exe -workerCount 30
```

Feel free to mix the flags as you desire.

## (Un)Available flags
```
-workerCount uint
   Number of concurrent workers (default 10)
-allCores bool
    Set to true if you want to use all processor cores and make it parallel
-page int
    Default is to start from first page, set it to some other page if you'd like
-continue bool
   Set to true if you want to continue from where you left of (last saved page number in the file)
-fast bool
    Set this to true if you want to risk some books not persisting if you violently stop the program, meaning that you cannot use -continue flag and start again sometimes.
```



# Disclaimer
Even though this ```is not``` an illegal book downloader, because it says so in the title of this repo, I do have one thing to say about it, anyway...
### Some people can't afford stuff.
There, I said it. 


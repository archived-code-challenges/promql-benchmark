package main

/*
This file holds the code of a command line tool that can be used to benchmark PromQL query
performance across multiple workers/clients against a Promscale instance.
The tool should takes as its input a flag pointing to the input CSV file (promql_queries.csv), a flag
to specify the number of concurrent workers and a flag containing the url of the promscale server.

After processing all the queries specified by the parameters in the CSV file, the tool outputs a summary.
*/

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type HttpClient interface {
	Get(url string) (resp *http.Response, err error)
}

type Client struct {
	Client  HttpClient
	URL     *url.URL
	Version string
}

var schemeRegex = regexp.MustCompile(`^((http[s]?|ftp):\/)\/`)

func getScheme(text string) *string {
	if match := schemeRegex.FindString(text); match != "" {
		return &match
	}
	return nil
}

// newHTTPClient instantiates a new Client given a host url. The url can (optionally) contain the
// scheme, which will be set to 'https' otherwise.
func newHTTPClient(host string) *Client {
	scheme := "https"
	if s := getScheme(host); s != nil {
		host = strings.TrimLeft(host, *s)
		scheme = strings.TrimRight(*s, "://")
	}
	return &Client{
		Client: &http.Client{
			Timeout: time.Second,
		},
		URL:     &url.URL{Host: host, Scheme: scheme},
		Version: "v1",
	}
}

// getHTTPQuery builds an HTTP request given a query and returns a Response containing the elapsed
// time from the beginning to the end of the call to the target server.
func (c *Client) getHTTPQuery(q *Query) (*Response, error) {
	c.URL.Path = "/api/" + c.Version + "/query_range"

	var params = url.Values{}
	params.Add("query", q.Query)
	params.Add("start", time.Unix(0, q.Start*int64(time.Millisecond)).Format(time.RFC3339))
	params.Add("end", time.Unix(0, q.End*int64(time.Millisecond)).Format(time.RFC3339))
	params.Add("step", fmt.Sprintf("%d", q.Step))
	c.URL.RawQuery = params.Encode()

	start := time.Now()
	resp, err := c.Client.Get(c.URL.String())
	end := time.Now()

	if err != nil {
		return nil, fmt.Errorf("getHTTPQuery() sending request to server. error=%v", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getHTTPQuery() unexpected response status code: %d", resp.StatusCode)
	}

	return &Response{resp, Timestamp{Start: start, End: end}}, nil
}

// Timestamp the elapsed time from the beginning to the end of a specific Response.
type Timestamp struct {
	Start time.Time
	End   time.Time
}

type Response struct {
	*http.Response
	Timestamp struct {
		Start time.Time
		End   time.Time
	}
}

// Stats of the resulting from the execution of the command line tool.
type Stats struct {
	// Average query time
	Average float64
	// Errors is the error list for queries that encountered an error
	Errors []error
	// Fastest is the minimum query time (for a single query) in milliseconds
	Fastest int64
	// Median query time of all queries
	Median float64
	// Processed is the number of queries processed in milliseconds
	Processed int
	// Slowest is maximum query time (for a single query) in milliseconds
	Slowest int64
	// Total processing time across all queries in milliseconds
	Total int64
}

func (s *Stats) ToString() (output string) {
	output += fmt.Sprintf("Number of queries processed: %d\n", s.Processed)
	output += fmt.Sprintf("Total processing time across all queries: %dms\n", s.Total)
	output += fmt.Sprintf("Minimum query time (for a single query): %dms\n", s.Fastest)
	output += fmt.Sprintf("Maximum query time (for a single query): %dms\n", s.Slowest)
	output += fmt.Sprintf("Median query time: %fms\n", s.Median)
	output += fmt.Sprintf("Average query time: %fms\n", s.Average)
	return
}

// Query contains an individual row resulting from reading the csv file.
type Query struct {
	Query string
	// Start holds the staring time in unix format in milliseconds
	Start int64
	// Start holds the end time in unix format in milliseconds
	End  int64
	Step int
}

// readFile reads a csv file containing a list of queries written in the form provided in the
// specifications of this tool, which follows the following form: `PromQL_query,start_time,end_time,step_size`.
//
// This provided file should NOT have a header.
func readFile(file io.Reader) ([]Query, error) {
	csvReader := csv.NewReader(file)
	csvReader.Comma = '|'
	csvReader.LazyQuotes = true

	csvRecords, err := csvReader.ReadAll()
	if err != nil {
		log.Fatalf("unable to parse provided file as CSV. err=%v", err)
	}

	queries := make([]Query, len(csvRecords))
	for i, line := range csvRecords {
		start, err := strconv.ParseInt(line[1], 10, 64)
		if err != nil {
			return nil, err
		}

		end, err := strconv.ParseInt(line[2], 10, 64)
		if err != nil {
			return nil, err
		}

		step, err := strconv.Atoi(line[3])
		if err != nil {
			return nil, err
		}

		queries[i] = Query{
			Query: line[0],
			Start: start,
			End:   end,
			Step:  step,
		}
	}

	return queries, nil
}

// getQueriesStats calculates the slowest, fastest, average and median execution times of a given Query list.
func getQueriesStats(queryList []Query) *Stats {
	var slowest int64
	var average, median float64
	fastest := int64(math.MaxInt64)

	var timeDiffs []int64
	for i := range queryList {
		timeDiff := queryList[i].End - queryList[i].Start
		if timeDiff < fastest {
			fastest = timeDiff
		}
		if timeDiff > slowest {
			slowest = timeDiff
		}
		average += float64(timeDiff)
		timeDiffs = append(timeDiffs, timeDiff)
	}

	// Calculate median
	sort.Slice(timeDiffs, func(i, j int) bool { return timeDiffs[i] < timeDiffs[j] })
	mNumber := len(timeDiffs) / 2
	if len(timeDiffs)%2 != 0 { // if the number of elements is odd
		median = float64(timeDiffs[mNumber])
	} else {
		median = float64((timeDiffs[mNumber-1] + timeDiffs[mNumber])) / 2
	}

	// Calculate average
	average = float64(average) / float64(len(queryList))

	return &Stats{
		Average: average,
		Fastest: fastest,
		Median:  median,
		Slowest: slowest,
	}
}

func benchmark(c *Client, queries []Query, maxConcurrentWorkers int) *Stats {
	wg := sync.WaitGroup{}
	wg.Add(len(queries))
	// workers is a limiting channel to control number of concurrent goroutines used
	workers := make(chan struct{}, maxConcurrentWorkers)

	var errorList []error
	var queryList []Query
	start := time.Now()
	for i := range queries {
		go func(q Query) {
			workers <- struct{}{}

			defer func() {
				<-workers
				wg.Add(-1)
			}()

			resp, err := c.getHTTPQuery(&q)
			if err != nil {
				errorList = append(errorList, fmt.Errorf("query=%v, error=%v", q, err))
			}
			if resp.StatusCode > 200 {
				log.Printf("error: status=%d, query=%v", resp.StatusCode, q)
			}

			// This part reuses the query structure obtained from the csv and overwrites its time
			// values for start and end of execution.
			q.Start = resp.Timestamp.Start.UnixMilli()
			q.End = resp.Timestamp.End.UnixMilli()
			queryList = append(queryList, q)
		}(queries[i])
	}

	wg.Wait()
	end := time.Now()

	// Build stats using the queries processed
	stats := getQueriesStats(queryList)
	stats.Processed = len(queries) - len(errorList)
	stats.Total = end.Sub(start).Milliseconds()
	stats.Errors = errorList

	return stats
}

type Config struct {
	Filepath string
	Workers  int
	URL      string
}

func parseFlags() (*Config, error) {
	// Subcommands
	benchmarkCommand := flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	// List subcommand flag pointers
	filepath := benchmarkCommand.String("filepath", "", "CSV file to process. (Required).")
	workers := benchmarkCommand.Int("workers", 1, "Number of concurrent workers.")
	url := benchmarkCommand.String("promscale.url", "http://localhost:9201", "Promscale web address. The scheme defaults to 'https' if not provided in the URL.")

	// Switch on the subcommand
	switch os.Args[1] {
	case "benchmark":
		// Parse the flags for appropriate FlagSet
		benchmarkCommand.Parse(os.Args[2:])
	default:
		flag.PrintDefaults()
		os.Exit(1)
	}

	// Check which subcommand was Parsed using the FlagSet.Parsed() function. Handle each case accordingly.
	if benchmarkCommand.Parsed() {
		if *filepath == "" { // A non-empty file path is required
			benchmarkCommand.PrintDefaults()
			return nil, fmt.Errorf("required input file")
		}
	}

	return &Config{Filepath: *filepath, URL: *url, Workers: *workers}, nil
}

func main() {
	// Verify that a subcommand has been provided
	if len(os.Args) < 2 {
		log.Print("benchmark subcommand is required")
		os.Exit(1)
	}

	log.Print(os.Args)
	// Get flags from command line
	cfg, err := parseFlags()
	if err != nil {
		log.Printf("unable to retrieve config err=%v", err)
		os.Exit(1)
	}

	f, err := os.Open(cfg.Filepath)
	if err != nil {
		log.Print("unable to open input file "+cfg.Filepath, err)
		os.Exit(1)
	}
	defer f.Close()

	// Read the promql queries file
	queries, err := readFile(f)
	if err != nil {
		log.Print("unable to read input file "+cfg.Filepath, err)
	}
	cli := newHTTPClient(cfg.URL)
	stats := benchmark(cli, queries, cfg.Workers)

	log.Println(stats.ToString())
}

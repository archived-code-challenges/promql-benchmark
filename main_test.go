package main

import (
	"net/http"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
)

func Test_getQueriesStats(t *testing.T) {
	tests := []struct {
		name      string
		queryList []Query
		want      *Stats
	}{
		{
			name: "OK",
			queryList: []Query{
				{Query: "", Start: 0, End: 2}, // 2
				{Query: "", Start: 0, End: 2}, // 2
				{Query: "", Start: 0, End: 2}, // 2
			},
			want: &Stats{Average: 2, Fastest: 2, Median: 2, Slowest: 2},
		},
		{
			name: "Odd number of queries",
			queryList: []Query{
				{Query: "", Start: 0, End: 1}, // 1
				{Query: "", Start: 0, End: 2}, // 2
				{Query: "", Start: 0, End: 3}, // 3
			},
			want: &Stats{Average: 2, Fastest: 1, Median: 2, Slowest: 3},
		},
		{
			name: "Even number of queries",
			queryList: []Query{
				{Query: "", Start: 0, End: 1}, // 1
				{Query: "", Start: 0, End: 2}, // 2
				{Query: "", Start: 0, End: 3}, // 3
				{Query: "", Start: 0, End: 4}, // 4
			},
			want: &Stats{Average: 2.5, Fastest: 1, Median: 2.5, Slowest: 4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getQueriesStats(tt.queryList); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getQueriesStats() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_parseFlags(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    *Config
		wantErr bool
	}{
		{
			name:    "Required input file",
			args:    []string{"benchmark"},
			wantErr: true,
		},
		{
			name: "OK",
			args: []string{"benchmark", "--filepath=promql_queries.csv", "--workers=100", "--promscale.url=http://localhost:9201"},
			want: &Config{Filepath: "promql_queries.csv", Workers: 100, URL: "http://localhost:9201"},
		},
	}
	for _, tt := range tests {
		os.Args = os.Args[:1] // cleanup args
		for i := range tt.args {
			os.Args = append(os.Args, tt.args[i])
		}

		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got, err := parseFlags()
			if (err != nil) != tt.wantErr {
				t.Errorf("parseFlags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseFlags() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_getScheme(t *testing.T) {
	var newStringPtr = func(s string) *string {
		return &s
	}

	tests := []struct {
		name string
		text string
		want *string
	}{
		{
			name: "https scheme",
			text: "https://example.xyz",
			want: newStringPtr("https://"),
		},
		{
			name: "ftp scheme",
			text: "ftp://example.xyz",
			want: newStringPtr("ftp://"),
		},
		{
			name: "http scheme",
			text: "http://example.xyz",
			want: newStringPtr("http://"),
		},
		{
			name: "wrong scheme",
			text: "://example.xyz",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getScheme(tt.text); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getScheme() = %v, want %v", got, tt.want)
			}
		})
	}
}

type ClientMock struct{}

func (c *ClientMock) Get(url string) (resp *http.Response, err error) {
	return &http.Response{StatusCode: 200}, nil
}

// TestURL_getHTTPQuery checks the integrity of the url constructed by the method getHTTPQuery
func TestURL_getHTTPQuery(t *testing.T) {
	tests := []struct {
		name    string
		query   *Query
		url     *url.URL
		version string
		want    *url.URL
	}{
		{
			url: &url.URL{Scheme: "https", Host: "promscale.xyz"},
			query: &Query{
				Query: "some query",
				Start: 100000,
				End:   999999,
				Step:  50,
			},
			version: "v1",
			want: &url.URL{
				Scheme:   "https",
				Host:     "promscale.xyz",
				Path:     "/api/v1/query_range",
				RawQuery: "end=1970-01-01T01%3A16%3A39%2B01%3A00&query=some+query&start=1970-01-01T01%3A01%3A40%2B01%3A00&step=50",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{
				Client:  &ClientMock{},
				URL:     tt.url,
				Version: "v1",
			}
			got, err := c.getHTTPQuery(tt.query)
			if err != nil {
				t.Errorf("Client.getHTTPQuery() error = %v", err)
				return
			}
			if !reflect.DeepEqual(c.URL, tt.want) {
				t.Errorf("Client.getHTTPQuery() = %v, want %v", got, tt.want)
			}
		})
	}
}

func Test_readFile(t *testing.T) {
	tests := []struct {
		name         string
		fileContents string
		want         []Query
		wantErr      bool
	}{
		{
			name:         "empty file",
			fileContents: ``,
			want:         []Query{},
		},
		{
			name:         "one row",
			fileContents: `demo_cpu_usage_seconds_total{mode="idle"}|1597056698698|1597059548699|15000`,
			want: []Query{
				{
					Query: `demo_cpu_usage_seconds_total{mode="idle"}`,
					Start: 1597056698698,
					End:   1597059548699,
					Step:  15000,
				},
			},
		},
		{
			name: "multiple rows",
			fileContents: `demo_cpu_usage_seconds_total{mode="idle"}|1597056698698|1597059548699|15000
avg by(instance) (demo_cpu_usage_seconds_total)|1597057698698|1597058548699|60000`,
			want: []Query{
				{
					Query: `demo_cpu_usage_seconds_total{mode="idle"}`,
					Start: 1597056698698,
					End:   1597059548699,
					Step:  15000,
				},
				{
					Query: `avg by(instance) (demo_cpu_usage_seconds_total)`,
					Start: 1597057698698,
					End:   1597058548699,
					Step:  60000,
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := readFile(strings.NewReader(tt.fileContents))
			if (err != nil) != tt.wantErr {
				t.Errorf("readFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("readFile() = %v, want %v", got, tt.want)
			}
		})
	}
}

package buckyclient

import (
	// "errors"
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestFlushNoMetrics(t *testing.T) {
	client, err := NewClient("http://localhost:8888", 10)
	if err != nil {
		t.Errorf("Not expecting error: %v", err)
	}

	flushErr := client.flush()
	if ErrNoMetrics != flushErr {
		t.Errorf("Expecting error 'ErrNoMetrics', got %v", flushErr)
	}
}

func TestFlushWithMetrics(t *testing.T) {
	server, client := testTools(200, "")
	defer server.Close()

	client.Count("test", 1)
	client.flush()
}

func TestFlushReturnsErrorOnInvalidHostname(t *testing.T) {
	client := &Client{
		hostURL:    "localhost/url",
		http:       &http.Client{},
		metrics:    make(map[Metric]int),
		bufferPool: newBufferPool(),
	}

	client.Count("test", 1)

	flushErr := client.flush()
	if flushErr == nil {
		t.Errorf("Expecting error, got nil")
	}
}

func TestFormattedOutput(t *testing.T) {
	client := &Client{metrics: make(map[Metric]int)}

	client.Count("test", 1)
	client.Timer("timer", 10)

	// Need both for the test, since the order of the map isn't guaranteed
	expectedOutput := "test:1|c\ntimer:10|ms\n"
	expectedAlternateOutput := "timer:10|ms\ntest:1|c\n"

	b := new(bytes.Buffer)
	client.formatMetricsForFlush(b)
	output := b.String()

	if expectedOutput != output && expectedAlternateOutput != output {
		t.Errorf("Unxpected output, got '%s' instead", output)
	}
}

func TestReset(t *testing.T) {
	client := &Client{metrics: make(map[Metric]int)}

	client.Count("test", 1)
	client.Count("test2", 1)

	if len(client.metrics) != 2 {
		t.Errorf("Expecting the metrics count to be 2")
	}

	client.Reset()

	client.Count("test3", 1)

	if len(client.metrics) != 1 {
		t.Errorf("Expecting the metrics count to be 1")
	}
}

// From here: http://keighl.com/post/mocking-http-responses-in-golang/
func testTools(code int, body string) (*httptest.Server, *Client) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, body)
	}))

	transport := &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			return url.Parse(server.URL)
		},
	}

	httpClient := &http.Client{Transport: transport}

	client := &Client{
		hostURL:    "http://localhost:12345",
		http:       httpClient,
		metrics:    make(map[Metric]int),
		bufferPool: newBufferPool(),
	}

	return server, client
}

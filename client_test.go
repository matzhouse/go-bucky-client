package buckyclient

import (
	// "errors"
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

	client.Send("test", 1, "c")
	client.flush()

	return
}

func TestFlushReturnsErrorOnInvalidHostname(t *testing.T) {
	client := &Client{
		hostURL: "localhost/url",
		http:    &http.Client{},
	}

	client.Send("test", 1, "c")

	flushErr := client.flush()
	if flushErr == nil {
		t.Errorf("Expecting error, got nil")
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
		hostURL: "http://localhost:12345",
		http:    httpClient,
		metrics: make(map[Metric]int),
	}

	return server, client
}

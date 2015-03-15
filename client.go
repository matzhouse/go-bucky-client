package buckyclient

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

type Metric struct {
	name  string
	value string
	unit  string
}

type Client struct {
	host     string
	http     *http.Client
	logger   *log.Logger
	interval time.Duration
}

// NewClient returns a client that can send data to a bucky server
// It takes an interval value in seconds
func NewClient(host string, interval int) *Client {

	intSecond := fmt.Sprintf("%ds", interval)

	return &Client{
		http:     &http.Client{},                    // http client
		logger:   log.New(os.Stderr, "", LstdFlags), // Stderr logging
		interval: time.ParseDuration(intSecond),     // sends data ever x seconds
	}

}

// setLogger allows you to specify an external logger
// otherwise it uses the Stderr
func (c *Client) setLogger(logger *log.Logger) {
	c.logger = logger
}

// Listen starts the client listening for metrics on the chan
// The chan is returned from this func
func (c *Client) Listen() (ch chan *Metric) {

	ch = make(chan *Metric)

	go func() {

	}()

}

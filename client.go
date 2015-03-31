// buckyclient can send metrics to a buckyserver over http
// see https://github.com/HubSpot/BuckyServer for more information
package buckyclient

import (
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"sync"
	"time"
)

// Client contains all the data necessary for sending
// the metrics to the buckyserver
type Client struct {
	hostURL  string        // full URL of the buckyserver
	http     *http.Client  // Standard http client
	logger   *log.Logger   // logger
	interval time.Duration // Interval in seconds between sending metrics to buckyserver

	m       sync.Mutex     // mutex for protecting Metrics
	metrics map[Metric]int // Holds the current set of metrics ready for sending at every interval

	stop    chan bool
	stopped chan bool
}

var (
	ErrNoMetrics = errors.New("No metrics to flush")
)

// NewClient returns a client that can send data to a bucky server
// It takes an interval value in seconds
func NewClient(host string, interval int) (cl *Client, err error) {

	intSecond := fmt.Sprintf("%ds", interval)

	intDur, err := time.ParseDuration(intSecond)

	if err != nil {
		return nil, err
	}

	cl = &Client{
		hostURL:  host,
		http:     &http.Client{},
		logger:   log.New(os.Stderr, "", log.LstdFlags), // Stderr logging
		interval: intDur,
		stop:     make(chan bool, 1),
		stopped:  make(chan bool, 1),
		metrics:  make(map[Metric]int),
	}

	// start the sender
	cl.sender()

	return cl, nil

}

// Count returns nothing and allows a counter to be incremented by a value
func (c *Client) Count(name string, value int) {

	c.send(name, value, "c") // for a counter

}

// Send is used to record a metric and have it send to
// the bucky server - this is thread safe
func (c *Client) send(name string, value int, unit string) {

	// Protect c.Metrics!
	c.m.Lock()
	defer c.m.Unlock()

	key := Metric{name, unit}

	c.metrics[key] = c.metrics[key] + value

}

// setLogger allows you to specify an external logger
// otherwise it uses the Stderr
func (c *Client) setLogger(logger *log.Logger) {
	c.logger = logger
}

// flush actually sends the data. can be called after
// a specific time interval, or when stopping the client

func (c *Client) flush() error {
	if len(c.metrics) == 0 {
		return ErrNoMetrics
	}

	// collect all the metrics
	c.m.Lock()

	var metricstr string
	output := ""

	for k, v := range c.metrics {
		metricstr = fmt.Sprintf("%s:%d|%s", k.name, v, k.unit)
		output = output + metricstr + "\n"
	}

	c.m.Lock()

	log.Println("sending - ", output)

	b := bytes.NewBufferString(output)
	// The request will only accept a ReadCloser for the body - this method fakes it by adding a nop close method.
	body := ioutil.NopCloser(b)

	// Send the string on to the server
	resp, err := c.http.Post(c.hostURL, "text/plain", body)

	if err != nil {
		log.Println("http client - ", err)
		return err
	}

	if resp.StatusCode > 299 {
		log.Println("status code above 200 received - ", resp.StatusCode)
		// Could just drop the data here - not much point sending it on
		// but we should probably tweak the interval

		return fmt.Errorf("Non-success HTTP Status Code (%d)", resp.StatusCode)
	}

	return nil
}

// Listen starts the client listening for metrics on the chan
// The chan is returned from this func
func (c *Client) sender() (err error) {

	//currentInt := c.interval // for the backoff we'll need to use the initial value as a reset

	go func(c *Client) {

		for {

			select {
			case <-time.After(c.interval):

				c.flush()

			case <-c.stop:
				log.Println("Shutting down bucky client")

				log.Println("Flushing last remaining metrics because of shutdown")
				c.flush()
				log.Println("Metrics flushed")

				c.stopped <- true
				break
			}

		} // for

	}(c)

	return nil

}

func (c *Client) Stop() {
	log.Println("Stopping bucky client")
	c.stop <- true

	// Wait until it actually stops
	<-c.stopped
	log.Println("Client stopped")
}

// Metric represents a metric to be sent over the wire
type Metric struct {
	name string
	unit string
}

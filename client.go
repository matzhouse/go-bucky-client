// Package buckyclient can send metrics to a buckyserver over http
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

	m       sync.Mutex       // mutex for protecting Metrics
	metrics map[Metric]Value // Holds the current set of metrics ready for sending at every interval

	stop    chan bool
	stopped chan bool
}

var (
	// ErrNoMetrics is returned when there are not metrics to flush
	ErrNoMetrics = errors.New("No metrics to flush")
)

// Value holds the different types of values
type Value struct {
	Avg *Average
	Sum *Sum
}

// Average holds average data for a metric
type Average struct {
	Count int
	Total int
	Avg   int
}

// Sum holds sum data for a metric
type Sum struct {
	Value int
}

// NewClient returns a client that can send data to a bucky server
// It takes an interval value in seconds
func NewClient(host string, interval int) (cl *Client, err error) {

	// We should never send more often than once per minute
	if interval < 60 {
		interval = 60
	}

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
		metrics:  make(map[Metric]Value),
	}

	// start the sender
	cl.sender()

	return cl, nil

}

// Count returns nothing and allows a counter to be incremented by a value
func (c *Client) Count(name string, value int) {

	c.send(name, value, "c", "sum") // for a counter

}

// Timer returns nothing and allows a timer metric to be set
func (c *Client) Timer(name string, value int) {

	c.send(name, value, "ms", "sum") // timer, so count in milliseconds

}

// AverageTimer returns nothing and allows a timer metric to be set
func (c *Client) AverageTimer(name string, value int) {

	c.send(name, value, "ms", "avg") // timer, so count in milliseconds

}

// Send is used to record a metric and have it send to
// the bucky server - this is thread safe
func (c *Client) send(name string, value int, unit string, action string) {

	// Protect c.Metrics!
	c.m.Lock()
	defer c.m.Unlock()

	key := Metric{name, unit}

	switch {
	case action == "sum":

		if c.metrics[key].Avg != nil {
			// if this is already a sum metric then we can't use it any more
			c.logger.Printf("cannot use metric %s as an Average - it's already a sum", name)
			return
		}

		if val, ok := c.metrics[key]; ok {
			c.metrics[key].Sum.Value = val.Sum.Value + value
		} else {

			// New value needed
			c.metrics[key] = Value{
				Sum: &Sum{
					Value: value,
				},
			}

		}

	case action == "avg":

		if c.metrics[key].Sum != nil {
			// if this is already a sum metric then we can't use it any more
			c.logger.Printf("cannot use metric %s as an Avg - it's already a sum", name)
			return
		}

		var avgResult float32
		var newCount int

		if val, ok := c.metrics[key]; ok {
			// Update the average values
			newCount = c.metrics[key].Avg.Count + 1
			avgResult = (c.metrics[key].Avg.Avg*c.metrics[key].Avg.Count + value) / newCount

			c.metrics[key].Avg.Total = val.Avg.Total + value
			c.metrics[key].Avg.Count = newCount
			c.metrics[key].Avg.Avg = avgResult

		} else {

			// New value needed
			c.metrics[key] = Value{
				Avg: &Average{},
			}

			newCount = c.metrics[key].Avg.Count + 1
			avgResult = (c.metrics[key].Avg.Avg*float32(c.metrics[key].Avg.Count) + float32(value)) / float32(newCount)

			// Update the average values
			c.metrics[key].Avg.Total = c.metrics[key].Avg.Total + value
			c.metrics[key].Avg.Count = newCount
			c.metrics[key].Avg.Avg = avgResult

		}

	default:
		c.logger.Printf("unknown action type - %s", action)
	}

}

// SetLogger allows you to specify an external logger
// otherwise it uses the Stderr
func (c *Client) SetLogger(logger *log.Logger) {
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

		if v.Avg != nil {

			metricstr = fmt.Sprintf("%s:%f|%s", k.name, v.Avg.Avg, k.unit)
			output = output + metricstr + "\n"

		} else if v.Sum != nil {

			metricstr = fmt.Sprintf("%s:%d|%s", k.name, v.Sum.Value, k.unit)
			output = output + metricstr + "\n"

		}

	}

	c.Reset()
	c.m.Unlock()

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

// Reset resets the client map to nil after data has been sent
func (c *Client) Reset() {
	c.metrics = make(map[Metric]Value)
}

// Listen starts the client listening for metrics on the chan
// The chan is returned from this func
func (c *Client) sender() (err error) {

	//currentInt := c.interval // for the backoff we'll need to use the initial value as a reset

	go func(c *Client) {

		for {

			select {

			case <-c.stop:
				log.Println("Shutting down bucky client")

				log.Println("Flushing last remaining metrics because of shutdown")
				c.flush()
				log.Println("Metrics flushed")

				c.stopped <- true

			case <-time.After(c.interval):

				c.flush()
			}

		} // for

	}(c)

	return nil

}

// Stop nicely stops the client
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

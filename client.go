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
	"strconv"
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

	input chan MetricWithValue

	stop    chan bool
	stopped chan bool

	bufferPool *sync.Pool
}

var (
	// ErrNoMetrics is returned when there are not metrics to flush
	ErrNoMetrics = errors.New("No metrics to flush")
)

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
		hostURL:    host,
		http:       &http.Client{},
		logger:     log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile),
		interval:   intDur,
		input:      make(chan MetricWithValue),
		stop:       make(chan bool, 1),
		stopped:    make(chan bool, 1),
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
	}

	// start the sender
	cl.sender()

	// So we process the input channel
	go cl.inputProcessor()

	return cl, nil

}

func newBufferPool() *sync.Pool {
	return &sync.Pool{
		New: func() interface{} {
			return new(bytes.Buffer)
		},
	}
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
	// Send to channel so we never block
	m := Metric{
		name: name,
		unit: unit,
	}

	v := Value{}

	switch action {
	case "sum":
		v.Sum = &Sum{
			Value: value,
		}
	case "avg":
		v.Avg = &Average{}

		var avgResult int
		var newCount int

		if val, ok := c.metrics[m]; ok {
			// Update the average values
			newCount = c.metrics[m].Avg.Count + 1
			avgResult = (c.metrics[m].Avg.Avg*c.metrics[m].Avg.Count + value) / newCount

			c.metrics[m].Avg.Total = val.Avg.Total + value
			c.metrics[m].Avg.Count = newCount
			c.metrics[m].Avg.Avg = avgResult

		} else {
			newCount = v.Avg.Count + 1
			avgResult = (v.Avg.Avg*v.Avg.Count + value) / newCount

			// Update the average values
			v.Avg.Total = v.Avg.Total + value
			v.Avg.Count = newCount
			v.Avg.Avg = avgResult

			c.metrics[m] = v
		}
	}

	c.input <- MetricWithValue{m, v}
}

// SetLogger allows you to specify an external logger
// otherwise it uses the Stderr
func (c *Client) SetLogger(logger *log.Logger) {
	c.logger = logger
}

func (c *Client) formatMetricsForFlush(buf *bytes.Buffer) {
	for k, v := range c.metrics {
		buf.WriteString(k.name)
		buf.WriteRune(':')

		// I blame @bradfitz for this: http://yapcasia.org/2015/talk/show/6bde6c69-187a-11e5-aca1-525412004261
		if v.Avg != nil {
			buf.Write(strconv.AppendInt([]byte(""), int64(v.Avg.Avg), 10))
		} else if v.Sum != nil {
			buf.Write(strconv.AppendInt([]byte(""), int64(v.Sum.Value), 10))
		}

		buf.WriteRune('|')
		buf.WriteString(k.unit)
		buf.WriteRune('\n')
	}
}

// flush actually sends the data. It can be called after
// a specific time interval, or when stopping the client
func (c *Client) flush() error {
	if len(c.metrics) == 0 {
		return ErrNoMetrics
	}

	// collect all the metrics
	c.m.Lock()

	buf := c.bufferPool.Get().(*bytes.Buffer)
	buf.Reset()

	c.formatMetricsForFlush(buf)

	c.Reset()
	c.m.Unlock()

	// The request will only accept a ReadCloser for the body - this method
	// fakes it by adding a nop close method.
	body := ioutil.NopCloser(buf)

	// Send the string on to the server
	resp, err := c.http.Post(c.hostURL, "text/plain", body)

	c.bufferPool.Put(buf)

	if err != nil {
		c.logger.Println("http client - ", err)
		return err
	}

	if resp.StatusCode > 299 {
		c.logger.Println("status code above 200 received - ", resp.StatusCode)
		// Could just drop the data here - not much point sending it on
		// but we should probably tweak the interval

		return fmt.Errorf("Non-success HTTP Status Code (%d)", resp.StatusCode)
	}

	return nil
}

func (c *Client) flushInputChannel() {
	for {
		select {
		case metric := <-c.input:
			c.handleMetricWithValue(metric)
		default:
			return
		}
	}
}

// Reset resets the client map to nil after data has been sent
func (c *Client) Reset() {
	for k := range c.metrics {
		delete(c.metrics, k)
	}
}

func (c *Client) handleMetricWithValue(metric MetricWithValue) {
	// Protect c.Metrics!
	c.m.Lock()
	defer c.m.Unlock()

	c.metrics[metric.Metric] = metric.Value
}

func (c *Client) inputProcessor() {
	for metric := range c.input {
		c.handleMetricWithValue(metric)
	}
}

// Listen starts the client listening for metrics on the chan
// The chan is returned from this func
func (c *Client) sender() (err error) {

	//currentInt := c.interval // for the backoff we'll need to use the initial value as a reset

	go func(c *Client) {

		for {

			select {

			case <-c.stop:
				c.logger.Println("Shutting down bucky client")

				// Make sure we don't have things left on the channel that aren't in the metrics map
				c.flushInputChannel()

				c.logger.Println("Flushing last remaining metrics because of shutdown")
				c.flush()
				c.logger.Println("Metrics flushed")

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
	c.logger.Println("Stopping bucky client")
	c.stop <- true

	// Wait until it actually stops
	<-c.stopped
	c.logger.Println("Client stopped")
}

// Metric represents a metric to be sent over the wire
type Metric struct {
	name string
	unit string
}

type MetricWithValue struct {
	Metric
	Value
}

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

package buckyclient

import (
	// "errors"

	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// mockBuckyServer echoes out the contents it receives in its request body
func mockBuckyServer(code int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(code)
		if code > 299 {
			w.Write([]byte(http.StatusText(code)))
			return
		}

		written, err := io.Copy(w, r.Body)
		if err != nil {
			log.Printf("Error: %s", err.Error())
			return
		}

		log.Printf("Wrote %d bytes", written)
	}))
}

func TestClient_Client_NewClient(t *testing.T) {
	cl, err := NewClient("", 20)
	defer func() {
		close(cl.stop)
		close(cl.stopped)
		close(cl.input)
	}()

	assert.NoError(t, err)
	assert.Equal(t, cl.interval, 60*time.Second)
}

func TestClient_Client_NewClient_Error(t *testing.T) {
	cl, err := NewClient("", 1<<48) // Cause an integer overflow in time.ParseDuration

	assert.Error(t, err)
	assert.Nil(t, cl)
}

func TestClient_Client_Count(t *testing.T) {
	name := "myapp.facet"
	value := 1

	cl := &Client{
		http:       &http.Client{},
		logger:     log.New(ioutil.Discard, "", log.Ldate|log.Ltime|log.Lshortfile),
		interval:   3 * time.Second,
		input:      make(chan MetricWithAmount, 10),
		stop:       make(chan bool, 1),
		stopped:    make(chan bool, 1),
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
	}
	defer func() {
		close(cl.stop)
		close(cl.stopped)
		close(cl.input)
	}()

	cl.Count(name, value)

	time.Sleep(time.Millisecond * 20) // Give the goroutine a chance to run

	assert.Equal(t, len(cl.input), 1)

	metric := <-cl.input

	assert.Equal(t, metric.name, name)
	assert.Equal(t, metric.unit, "c")
	assert.Equal(t, metric.Amount.Value, value)
}

func TestClient_Client_Timer(t *testing.T) {
	name := "myapp.facet"
	value := 1

	cl := &Client{
		http:       &http.Client{},
		logger:     log.New(ioutil.Discard, "", log.Ldate|log.Ltime|log.Lshortfile),
		interval:   3 * time.Second,
		input:      make(chan MetricWithAmount, 10),
		stop:       make(chan bool, 1),
		stopped:    make(chan bool, 1),
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
	}
	defer func() {
		close(cl.stop)
		close(cl.stopped)
		close(cl.input)
	}()

	cl.Timer(name, value)

	time.Sleep(time.Millisecond * 20) // Give the goroutine a chance to run

	assert.Equal(t, len(cl.input), 1)

	metric := <-cl.input

	assert.Equal(t, metric.name, name)
	assert.Equal(t, metric.unit, "ms")
	assert.Equal(t, metric.Amount.Value, value)
}

func TestClient_Client_AverageTimer(t *testing.T) {
	name := "myapp.facet"
	value := 1

	cl := &Client{
		http:       &http.Client{},
		logger:     log.New(ioutil.Discard, "", log.Ldate|log.Ltime|log.Lshortfile),
		interval:   3 * time.Second,
		input:      make(chan MetricWithAmount, 10),
		stop:       make(chan bool, 1),
		stopped:    make(chan bool, 1),
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
	}
	defer func() {
		close(cl.stop)
		close(cl.stopped)
		close(cl.input)
	}()

	cl.AverageTimer(name, value)

	cl.AverageTimer(name, 3)

	time.Sleep(time.Millisecond * 20)

	metric := <-cl.input

	assert.Equal(t, metric.name, name)
	assert.Equal(t, metric.unit, "ms")
	assert.Equal(t, metric.Amount.Value, 3)
}

func TestClient_Client_SetLogger(t *testing.T) {
	logMessage := "Bucky bucky bucky!"

	cl := &Client{logger: log.New(os.Stderr, "", log.Ldate|log.Ltime|log.Lshortfile)}

	buf := &bytes.Buffer{}
	newLogger := log.New(buf, "", log.Ldate|log.Ltime|log.Lshortfile)

	cl.SetLogger(newLogger)

	cl.logger.Println(logMessage)

	assert.Contains(t, buf.String(), logMessage)
}

func TestClient_Client_formatMetricsForFlush(t *testing.T) {
	buf := &bytes.Buffer{}

	cl := &Client{metrics: make(map[Metric]Value)}

	cl.metrics[Metric{name: "myapp.facet.test1", unit: "ms"}] = Value{
		Avg: &Average{
			Count: 123,
			Total: 456,
			Avg:   2,
		},
	}
	cl.metrics[Metric{name: "myapp.facet.test2", unit: "c"}] = Value{
		Sum: &Sum{
			Value: 987,
		},
	}

	cl.formatMetricsForFlush(buf)

	// Order is non-determenistic!
	assert.Contains(t, buf.String(), "myapp.facet.test1:2|ms\n")
	assert.Contains(t, buf.String(), "myapp.facet.test2:987|c\n")
}

func TestClient_Client_flush(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(buf, "", log.Ldate|log.Ltime|log.Lshortfile)

	mockBucky := mockBuckyServer(http.StatusOK)
	defer mockBucky.Close()

	cl := &Client{
		hostURL:    mockBucky.URL,
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
		http:       &http.Client{},
		logger:     logger,
	}

	cl.metrics[Metric{name: "myapp.facet.test1", unit: "ms"}] = Value{
		Avg: &Average{
			Count: 123,
			Total: 456,
			Avg:   2,
		},
	}
	cl.metrics[Metric{name: "myapp.facet.test2", unit: "c"}] = Value{
		Sum: &Sum{
			Value: 987,
		},
	}

	err := cl.flush()

	assert.NoError(t, err)
}

func TestClient_Client_flush_NoMetricsError(t *testing.T) {
	cl := &Client{
		metrics: make(map[Metric]Value),
	}

	err := cl.flush()

	assert.Error(t, err)
}

func TestClient_Client_flush_HttpPostError(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := log.New(buf, "", log.Ldate|log.Ltime|log.Lshortfile)

	cl := &Client{
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
		http:       &http.Client{},
		logger:     logger,
	}

	cl.metrics[Metric{name: "myapp.facet.test1", unit: "ms"}] = Value{
		Avg: &Average{
			Count: 123,
			Total: 456,
			Avg:   2,
		},
	}
	cl.metrics[Metric{name: "myapp.facet.test2", unit: "c"}] = Value{
		Sum: &Sum{
			Value: 987,
		},
	}

	err := cl.flush()

	assert.Error(t, err)
}

func TestClient_Client_flush_NonSuccessfulStatusCode(t *testing.T) { // Non 2XX
	buf := &bytes.Buffer{}
	logger := log.New(buf, "", log.Ldate|log.Ltime|log.Lshortfile)

	mockBucky := mockBuckyServer(http.StatusTeapot)
	defer mockBucky.Close()

	cl := &Client{
		hostURL:    mockBucky.URL,
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
		http:       &http.Client{},
		logger:     logger,
	}

	cl.metrics[Metric{name: "myapp.facet.test1", unit: "ms"}] = Value{
		Avg: &Average{
			Count: 123,
			Total: 456,
			Avg:   2,
		},
	}
	cl.metrics[Metric{name: "myapp.facet.test2", unit: "c"}] = Value{
		Sum: &Sum{
			Value: 987,
		},
	}

	err := cl.flush()

	assert.Error(t, err)
}

func TestClient_Client_flushInputChannel(t *testing.T) {
	cl := &Client{
		metrics: make(map[Metric]Value),
		input:   make(chan MetricWithAmount, 1),
	}

	cl.input <- MetricWithAmount{Metric{name: "myapp.test"}, Amount{}, "sum"}

	cl.flushInputChannel()

	assert.Equal(t, len(cl.metrics), 1)
}

func TestClient_Client_Reset(t *testing.T) {
	cl := &Client{
		metrics: map[Metric]Value{
			Metric{name: "first"}:  Value{},
			Metric{name: "second"}: Value{},
		},
	}

	assert.Equal(t, len(cl.metrics), 2)

	cl.Reset()

	assert.Equal(t, len(cl.metrics), 0)
}

func TestClient_Client_handleMetricWithValue_Average(t *testing.T) {
	cl := &Client{
		metrics: make(map[Metric]Value),
	}

	metric := Metric{name: "m.et.ric"}
	amount := Amount{Value: 3}

	metricValue := MetricWithAmount{metric, amount, "avg"}

	cl.handleMetricWithValue(metricValue)

	cl.handleMetricWithValue(metricValue)

	assert.Equal(t, len(cl.metrics), 1)
	assert.Equal(t, cl.metrics[metric].Avg, &Average{
		Count: 2,
		Total: 6,
		Avg:   3,
	})
	assert.Nil(t, cl.metrics[metric].Sum)
}

func TestClient_Client_handleMetricWithValue_Sum(t *testing.T) {
	cl := &Client{
		metrics: make(map[Metric]Value),
	}

	metric := Metric{name: "m.et.ric"}
	amount := Amount{Value: 3}

	metricValue := MetricWithAmount{metric, amount, "sum"}

	cl.handleMetricWithValue(metricValue)

	assert.Equal(t, len(cl.metrics), 1)
	assert.Equal(t, cl.metrics[metric].Sum, &Sum{Value: 3})
	assert.Nil(t, cl.metrics[metric].Avg)
}

func TestClient_Client_inputProcessor(t *testing.T) {
	cl := &Client{
		metrics: make(map[Metric]Value),
		input:   make(chan MetricWithAmount, 5),
	}

	for i := 0; i < 5; i++ {
		cl.input <- MetricWithAmount{Metric{name: fmt.Sprintf("%d", i)}, Amount{}, "sum"}
	}
	close(cl.input) // Close or this will hang!

	cl.inputProcessor()

	assert.Equal(t, len(cl.metrics), 5)
}

func TestClient_Client_sender(t *testing.T) {
	cl := &Client{
		http:       &http.Client{},
		logger:     log.New(ioutil.Discard, "", log.Ldate|log.Ltime|log.Lshortfile),
		interval:   10 * time.Millisecond,
		input:      make(chan MetricWithAmount, 10),
		stop:       make(chan bool, 1),
		stopped:    make(chan bool, 1),
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
	}

	err := cl.sender()
	time.Sleep(time.Millisecond * 100) // Allow sender to actually execute
	assert.NoError(t, err)
}

func TestClient_Client_Stop(t *testing.T) {
	buf := &bytes.Buffer{}

	cl := &Client{
		logger:  log.New(buf, "", log.Ldate|log.Ltime|log.Lshortfile),
		stop:    make(chan bool, 1),
		stopped: make(chan bool, 1),
	}

	go func() {
		time.Sleep(time.Millisecond * 100)
		close(cl.stopped)
	}()
	cl.Stop()

	assert.Contains(t, buf.String(), "Stopping bucky client")
	assert.Contains(t, buf.String(), "Client stopped")
}

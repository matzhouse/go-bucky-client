package buckyclient

import (
	"bytes"
	"strconv"
	"testing"
)

func populateClient(c *Client, n int) {
	for i := 0; i < n; i++ {
		c.Count("test_"+strconv.Itoa(i), i)
	}
}

func BenchmarkFormatMetricsForFlush(b *testing.B) {
	b.ReportAllocs()

	client := &Client{
		metrics:    make(map[Metric]Value),
		bufferPool: newBufferPool(),
		input:      make(chan MetricWithValue, 1000),
	}

	populateClient(client, 10)

	client.flushInputChannel()

	for i := 0; i < b.N; i++ {
		buf := client.bufferPool.Get().(*bytes.Buffer)
		client.formatMetricsForFlush(buf)

		client.bufferPool.Put(buf)
	}
}

package buckyclient

import (
	"bytes"
	"strconv"
	"testing"
)

func BenchmarkReset(b *testing.B) {
	b.ReportAllocs()

	client := &Client{metrics: make(map[Metric]int)}

	for i := 0; i < b.N; i++ {
		client.Count("test", 1)
		client.Reset()
	}
}

func populateClient(c *Client, n int) {
	for i := 0; i < n; i++ {
		c.Count("test_"+strconv.Itoa(i), i)
	}
}

func BenchmarkFormatMetricsForFlush(b *testing.B) {
	b.ReportAllocs()

	client := &Client{metrics: make(map[Metric]int), bufferPool: newBufferPool()}

	populateClient(client, 10)

	for i := 0; i < b.N; i++ {
		buf := client.bufferPool.Get().(*bytes.Buffer)
		client.formatMetricsForFlush(buf)

		client.bufferPool.Put(buf)
	}
}

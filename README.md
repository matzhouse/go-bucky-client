# go-bucky-client
simple client to collect and send stuff to a bucky server

This client can be used to send metrics to a bucky server. Bucky is used to send metrics over http that can then be sent on to something like graphite.

## Example

```go
package main

import (
	"log"
	"time"

	"github.com/matzhouse/go-bucky-client"
)

func main() {

	bc, err := buckyclient.NewClient(
	        "http://localhost:8005/bucky/v1/send", 
	        1,
	    )

	if err != nil {
		log.Fatal(err)
	}

	bc.Count("test.metric", "1") // test counter

	time.Sleep(5 * time.Second) // simple timeout to let the client do it's work

	bc.Stop()

}
```

Please feel free to send pull requests for new stuff, bug fixes etc

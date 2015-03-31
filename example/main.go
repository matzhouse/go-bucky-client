package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"time"

	"github.com/matzhouse/go-bucky-client"
)

func main() {

	go func() {

		http.HandleFunc("/bucky/v1/send", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			body, err := ioutil.ReadAll(r.Body)

			if err != nil {
				log.Println(err)
			}

			log.Println(string(body))
		})

		log.Fatal(http.ListenAndServe(":8005", nil))

	}()

	bc, err := buckyclient.NewClient("http://localhost:8005/bucky/v1/send", 1)

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("sending..")

	bc.Count("test.metric", 1)   // test counter
	bc.Count("test.metric", 2)   // test counter
	bc.Count("test.metric.2", 4) // test counter
	bc.Count("test.metric", 3)   // test counter

	fmt.Println("finished sending")

	time.Sleep(10 * time.Second)

	fmt.Println("sleep over")

	bc.Stop()

}

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

	bc.Send("test.metric", "1", "c") // test counter
	bc.Send("test.metric", "2", "c") // test counter
	bc.Send("test.metric", "4", "c") // test counter
	bc.Send("test.metric", "3", "c") // test counter

	fmt.Println("finished sending")

	time.Sleep(10 * time.Second)

	fmt.Println("sleep over")

	bc.Stop()

}

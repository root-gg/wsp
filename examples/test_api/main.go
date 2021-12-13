package main

import (
	"flag"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// For testing purpose

var addr = flag.String("addr", "localhost:8081", "http service address")

func hello(w http.ResponseWriter, r *http.Request) {
	log.Println("hello")
	w.Write([]byte("hello world\n"))
}

func header(w http.ResponseWriter, r *http.Request) {
	log.Println("header")
	w.Header().Add("hello", "world")
	w.Write([]byte("hello world in header\n"))
}

func post(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Println(err)
	}
	r.Body.Close()

	log.Println("post")
	log.Println(string(body))

	w.Write(body)
}

func fail(w http.ResponseWriter, r *http.Request) {
	http.Error(w, "GO FUNK YOURSELF", 666)
}

func sleep(w http.ResponseWriter, r *http.Request) {
	time.Sleep(10 * time.Second)
	w.Write([]byte("ok"))
}

func main() {
	flag.Parse()
	log.SetFlags(0)
	http.HandleFunc("/hello", hello)
	http.HandleFunc("/header", header)
	http.HandleFunc("/fail", fail)
	http.HandleFunc("/post", post)
	http.HandleFunc("/sleep", sleep)
	log.Fatal(http.ListenAndServe(*addr, nil))
}

package api

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"strconv"
	"time"
)

var url = flag.String("url", "http://107.178.208.130:80/", "Url to serve traffic to.")
var initQps = flag.Int("init_qps", 1, "Number of queries per second to initially send.")

var nsInSecond = int64(1000 * 1000 * 1000)
var nanoDelay = nsInSecond
var uptime = "Initial Uptime (not updated.)"
var latency = "None."

func sendQueries() {
	for true {
		time.Sleep(1 * time.Second)
		updateStats()
	}
}

func updateStats() {
	sendTime := time.Now()
	resp, err := http.Get(*url)
	latency = time.Since(sendTime).String()
	if err != nil {
		log.Printf("%v", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v", err)
		return
	}
	uptime = string(body)
	log.Printf("New Uptime: %s", uptime)
}

func updateBurn(units int) {
	resp, err := http.Get(fmt.Sprintf("%sburn/%d", *url, units))
	if err != nil {
		log.Printf("%v", err)
		return
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		log.Printf("%v: %v", err, body)
		return
	}
}

type queryInfo struct {
	Uptime  string `json:"uptime"`
	Latency string `json:"latency"`
}

func RegisterServiceHandlers() {
	nanoDelay = nsInSecond / int64(*initQps)
	http.HandleFunc("/api/qps", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		qps, err := strconv.Atoi(r.FormValue("qps"))
		if err != nil {
			log.Printf("%v", err)
			return
		}
		updateBurn(qps)
		log.Printf("Request(/api/qps) took %s", time.Since(start))
	})
	http.HandleFunc("/api/uptime", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		encoder := json.NewEncoder(w)
		err := encoder.Encode(&queryInfo{
			Uptime:  uptime,
			Latency: latency,
		})
		// fmt.Fprintf(w, "{\"uptime\":%q \"latency\":%q}", uptime, latency)
		if err != nil {
			log.Printf("unalbe to marshal json: %v", err)
		}
		log.Printf("Request(/api/uptime) took %s", time.Since(start))
	})
	go sendQueries()
}

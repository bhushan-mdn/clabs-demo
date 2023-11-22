package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
)

var inputCh chan map[string]string

type Config struct {
	WebhookEndpoint string
}

var config Config

func main() {
	endpoint, ok := os.LookupEnv("WEBHOOK_ENDPOINT")
	if !ok {
		fmt.Println(
			`Visit https://webhook.site to get a webhook endpoint, and set the environment variable
WEBHOOK_ENDPOINT=https://webhook.site/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX`)
		log.Fatalln("Exiting as WEBHOOK_ENDPOINT env variable is not set...")
	}
	config.WebhookEndpoint = endpoint

	inputCh = make(chan map[string]string, 5)

	for i := 1; i <= 5; i++ {
		go worker(i, inputCh)
	}

	http.HandleFunc("/", rootHandler)

	log.Println("Starting server on http://localhost:8090")
	log.Fatal(http.ListenAndServe(":8090", nil))
}

func worker(id int, inputCh chan map[string]string) {
	fmt.Println("worker", id, "started")
	for input := range inputCh {
		log.Println("worker", id, "processing request")
		transformed := transformRequest(input)
		resp, err := json.Marshal(&transformed)
		if err != nil {
			fmt.Println("err marshalling", err)
			return
		}

		sendToWebhook(bytes.NewReader(resp))
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	m := map[string]string{}

	if err := json.NewDecoder(r.Body).Decode(&m); err != nil || len(m) == 0 {
		w.WriteHeader(http.StatusBadRequest)
		fmt.Fprintf(w, "Bad Request")
		return
	}

	fmt.Printf("%v\n", m)
	go func() {
		fmt.Println("sending request")
		inputCh <- m
		fmt.Println("done")
	}()

	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "Accepted")
}

type AttrSchema struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

func transformRequest(m map[string]string) map[string]interface{} {

	staticKeyMapping := map[string]string{
		"ev":  "event",
		"et":  "event_type",
		"id":  "app_id",
		"uid": "user_id",
		"mid": "message_id",
		"t":   "page_title",
		"p":   "page_url",
		"l":   "browser_language",
		"sc":  "screen_size",
	}

	resp := make(map[string]interface{})

	attrs := make(map[string]AttrSchema)
	uTraits := make(map[string]AttrSchema)

	for k, v := range m {
		if k2, ok := staticKeyMapping[k]; ok {
			resp[k2] = v
		}
		if strings.HasPrefix(k, "atr") {
			if strings.HasPrefix(k, "atrk") {
				indexStr := strings.Split(k, "atrk")[1]
				if exisiting, ok := attrs[indexStr]; ok {
					exisiting.Key = v
					attrs[indexStr] = exisiting
				} else {
					var a AttrSchema
					a.Key = v
					attrs[indexStr] = a
				}
			}

			if strings.HasPrefix(k, "atrt") {
				indexStr := strings.Split(k, "atrt")[1]
				if exisiting, ok := attrs[indexStr]; ok {
					exisiting.Type = v
					attrs[indexStr] = exisiting
				} else {
					var a AttrSchema
					a.Type = v
					attrs[indexStr] = a
				}
			}

			if strings.HasPrefix(k, "atrv") {
				indexStr := strings.Split(k, "atrv")[1]
				if exisiting, ok := attrs[indexStr]; ok {
					exisiting.Value = v
					attrs[indexStr] = exisiting
				} else {
					var a AttrSchema
					a.Value = v
					attrs[indexStr] = a
				}
			}
		}
		if strings.HasPrefix(k, "uatr") {
			if strings.HasPrefix(k, "uatrk") {
				indexStr := strings.Split(k, "uatrk")[1]
				if exisiting, ok := uTraits[indexStr]; ok {
					exisiting.Key = v
					uTraits[indexStr] = exisiting
				} else {
					var a AttrSchema
					a.Key = v
					uTraits[indexStr] = a
				}
			}
			if strings.HasPrefix(k, "uatrt") {
				indexStr := strings.Split(k, "uatrt")[1]
				if exisiting, ok := uTraits[indexStr]; ok {
					exisiting.Type = v
					uTraits[indexStr] = exisiting
				} else {
					var a AttrSchema
					a.Type = v
					uTraits[indexStr] = a
				}
			}
			if strings.HasPrefix(k, "uatrv") {
				indexStr := strings.Split(k, "uatrv")[1]
				if exisiting, ok := uTraits[indexStr]; ok {
					exisiting.Value = v
					uTraits[indexStr] = exisiting
				} else {
					var a AttrSchema
					a.Value = v
					uTraits[indexStr] = a
				}
			}
		}
	}

	attrMap := make(map[string]interface{})
	for _, v := range attrs {
		attrMap[v.Key] = map[string]string{
			"type":  v.Type,
			"value": v.Value,
		}
	}

	traitMap := make(map[string]interface{})
	for _, v := range uTraits {
		traitMap[v.Key] = map[string]string{
			"type":  v.Type,
			"value": v.Value,
		}
	}
	resp["attributes"] = attrMap
	resp["traits"] = traitMap

	return resp
}

func sendToWebhook(input *bytes.Reader) {
	resp, err := http.Post(config.WebhookEndpoint, "application/json", input)
	if err != nil {
		log.Println(err)
	}
	log.Println(resp.Status)

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
	log.Println(string(body))
}

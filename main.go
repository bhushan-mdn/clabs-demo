package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"os"
	// "strings"
)

type AppState struct {
	InputsProcessed int
	WebhookSuccess  int
	WebhookFailure  int
}

var inputCh chan map[string]string

type Config struct {
	WebhookEndpoint  string
	WorkerPoolSize   int
	InputChannelSize int
}

var config Config
var state AppState

const DEFAULT_WORKER_POOL_SIZE = 5
const DEFAULT_INPUT_CHANNEL_SIZE = 10

func setup() {

    endpoint, ok := os.LookupEnv("WEBHOOK_ENDPOINT")
    if !ok {
        fmt.Println(
            `Visit https://webhook.site to get a webhook endpoint, and set the environment variable
WEBHOOK_ENDPOINT=https://webhook.site/XXXXXXXX-XXXX-XXXX-XXXX-XXXXXXXXXXXX`)
        log.Fatalln("Exiting as WEBHOOK_ENDPOINT env variable is not set...")
    }
    config.WebhookEndpoint = endpoint
}

func main() {
	setup()

	inputCh = make(chan map[string]string, DEFAULT_INPUT_CHANNEL_SIZE)

	for i := 1; i <= DEFAULT_WORKER_POOL_SIZE; i++ {
		go worker(i, inputCh)
	}

	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/stats", statsHandler)

	log.Println("Starting server on http://localhost:8090")
	log.Fatal(http.ListenAndServe(":8090", mux))
}

func worker(id int, inputCh chan map[string]string) {
	fmt.Println("worker", id, "started")
	for input := range inputCh {
		log.Println("worker", id, "processing request")
		transformed := transformRequest(input)
        state.InputsProcessed += 1
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

func (a *AttrSchema) SetVal(keyType string, value string) {
	switch keyType {
	case "k":
		a.Key = value
	case "t":
		a.Type = value
	case "v":
		a.Value = value
	}
}

type PartialReq struct {
	Ev  string `json:"ev"`
	Et  string `json:"et"`
	ID  string `json:"id"`
	UID string `json:"uid"`
	Mid string `json:"mid"`
	T   string `json:"t"`
	P   string `json:"p"`
	L   string `json:"l"`
	Sc  string `json:"sc"`
}

type ResponseStruct struct {
	Event           string              `json:"event"`
	EventType       string              `json:"event_type"`
	AppID           string              `json:"app_id"`
	UserID          string              `json:"user_id"`
	MessageID       string              `json:"message_id"`
	PageTitle       string              `json:"page_title"`
	PageURL         string              `json:"page_url"`
	BrowserLanguage string              `json:"browser_language"`
	ScreenSize      string              `json:"screen_size"`
	Attributes      map[string]AttrDesc `json:"attributes"`
	Traits          map[string]AttrDesc `json:"traits"`
}

type AttrDesc struct {
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

	var rs ResponseStruct

	for k, v := range m {
		if k2, ok := staticKeyMapping[k]; ok {
			resp[k2] = v
		}
		switch k {
		case "ev":
			rs.Event = v
		case "et":
			rs.EventType = v
		case "id":
			rs.AppID = v
		case "uid":
			rs.UserID = v
		case "mid":
			rs.MessageID = v
		case "t":
			rs.PageTitle = v
		case "p":
			rs.PageURL = v
		case "l":
			rs.BrowserLanguage = v
		case "sc":
			rs.ScreenSize = v
		}

		r := regexp.MustCompile("([u]*)atr([k,t,v])(\\d+)")
		match := r.FindStringSubmatch(k)
		var t string
		var idx string
		var attrType string
		if len(match) > 3 {
			attrType = match[1]
			t = match[2]
			idx = match[3]

			var exi AttrSchema
			var ok bool
			if attrType == "u" {
				exi, ok = uTraits[idx]
			} else {
				exi, ok = attrs[idx]
			}

			if ok {
				exi.SetVal(t, v)
				if attrType == "u" {
					uTraits[idx] = exi
				} else {
					attrs[idx] = exi
				}
			} else {
				var a AttrSchema
				a.SetVal(t, v)
				if attrType == "u" {
					uTraits[idx] = a
				} else {
					attrs[idx] = a
				}
			}
		}
	}

	rs.Attributes = make(map[string]AttrDesc)
	for _, v := range attrs {
		rs.Attributes[v.Key] = AttrDesc{
			Type:  v.Type,
			Value: v.Value,
		}
	}

	rs.Traits = make(map[string]AttrDesc)
	for _, v := range uTraits {
		rs.Traits[v.Key] = AttrDesc{
			Type:  v.Type,
			Value: v.Value,
		}
	}

	frs, err := json.MarshalIndent(rs, "", "  ")
	tempMust(err)
	fmt.Println(string(frs))

	var again map[string]interface{}
	err = json.Unmarshal(frs, &again)
	tempMust(err)

	return again
}

func tempMust(err error) {
	if err != nil {
		fmt.Println(err)
	}
}

func sendToWebhook(input *bytes.Reader) {
	resp, err := http.Post(config.WebhookEndpoint, "application/json", input)
	if err != nil {
		log.Println(err)
	}
	log.Println(resp.Status)

    if resp.StatusCode == http.StatusOK {
        state.WebhookSuccess += 1
    } else {
        state.WebhookFailure += 1
    }

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println(err)
	}
    log.Println("Webhook response:", string(body))
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "inputs: %d\nsuccess: %d\nfailure: %d\n", state.InputsProcessed, state.WebhookSuccess, state.WebhookFailure)
}

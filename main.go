package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
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
		resp, err := transformRequest(input)
        if err != nil {
		    log.Printf("Encountered an error: %v", err)
            return
        }
		state.InputsProcessed += 1

		sendToWebhook(resp)
	}
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	var m map[string]string

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check if the request body contains valid JSON
	if !json.Valid(body) {
		http.Error(w, "Bad Request - Invalid JSON", http.StatusBadRequest)
		return
	}

	// Decode JSON into a map
	if err := json.Unmarshal(body, &m); err != nil || len(m) == 0 {
		http.Error(w, "Bad Request - Invalid JSON Format", http.StatusBadRequest)
		return
	}

	fmt.Printf("%v\n", m)

	// Perform asynchronous processing
	go func() {
		fmt.Println("Sending request")
		inputCh <- m
		fmt.Println("Done")
	}()

	// Respond with status 202 (Accepted)
	w.WriteHeader(http.StatusAccepted)
	fmt.Fprintf(w, "Accepted")
}

type AttrSchema struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Type  string `json:"type"`
}

func (a *AttrSchema) SetVal(fieldName string, value string) {
	switch fieldName {
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

func transformRequest(m map[string]string) ([]byte, error) {
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
	var req PartialReq

	b, err := json.Marshal(m)
    if err != nil {
		log.Printf("Encountered an error: %v", err)
    }
	json.Unmarshal(b, &req)

	var rs ResponseStruct

	rs.Event = req.Ev
	rs.EventType = req.Et
	rs.AppID = req.ID
	rs.UserID = req.UID
	rs.MessageID = req.Mid
	rs.PageTitle = req.T
	rs.PageURL = req.P
	rs.BrowserLanguage = req.L
	rs.ScreenSize = req.Sc

	for k := range staticKeyMapping {
		delete(m, k)
	}

	attrs := make(map[string]AttrSchema)
	uTraits := make(map[string]AttrSchema)

	for k, v := range m {
		r := regexp.MustCompile("([u]*)atr([k,t,v])(\\d+)")
		match := r.FindStringSubmatch(k)
		var fieldName string
		var idx string
		var attributeType string
		if len(match) > 3 {
			attributeType = match[1]
			fieldName = match[2]
			idx = match[3]

			var attributeMap map[string]AttrSchema

			if attributeType == "u" {
				attributeMap = uTraits
			} else {
				attributeMap = attrs
			}

			existingAttribute, found := attributeMap[idx]
			if !found {
				existingAttribute = AttrSchema{}
			}

			existingAttribute.SetVal(fieldName, v)
			attributeMap[idx] = existingAttribute

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

	resp, err := json.Marshal(&rs)
	if err != nil {
		log.Printf("Encountered an error: %v", err)
        return nil, err
	}

	log.Println("Response:", string(resp))

	return resp, nil
}

func sendToWebhook(input []byte) {
	resp, err := http.Post(config.WebhookEndpoint, "application/json", bytes.NewBuffer(input))
	if err != nil {
		log.Println("Error sending request to webhook:", err)
		return
	}
	defer resp.Body.Close()

	log.Println("Webhook response status:", resp.Status)

	if resp.StatusCode == http.StatusOK {
		state.WebhookSuccess++
	} else {
		state.WebhookFailure++
	}

	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Println("Error reading webhook response:", err)
		return
	}
	log.Println("Webhook response body:", string(responseBody))
}

func statsHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "inputs: %d\nsuccess: %d\nfailure: %d\n", state.InputsProcessed, state.WebhookSuccess, state.WebhookFailure)
}

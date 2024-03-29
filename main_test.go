package main

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

const TESTS_PATH = "./tests/"

func readInput(filename string) map[string]string {
	f, err := os.ReadFile(TESTS_PATH + filename)
	must(err)
	input := map[string]string{}
	err = json.Unmarshal(f, &input)
	must(err)

	return input
}

func readOutput(filename string) []byte {
	f, err := os.ReadFile(TESTS_PATH + filename)
	must(err)
	want := make(map[string]interface{})
	err = json.Unmarshal(f, &want)
	must(err)

    // Marshalling twice to order keys lexographically
	wantJson, err := json.Marshal(want)
	must(err)

	return wantJson
}

func TestTransformRequest(t *testing.T) {
	var tests = []struct {
		inputFilename string
		wantFilename  string
	}{
		{"input_1.json", "output_1.json"},
		{"input_2.json", "output_2.json"},
		{"input_3.json", "output_3.json"},
	}

	for _, tt := range tests {
		testname := fmt.Sprintf("%s", tt.inputFilename)
		t.Run(testname, func(t *testing.T) {

			input := readInput(tt.inputFilename)
			want := readOutput(tt.wantFilename)
            fmt.Println(string(want))

			output, err := transformRequest(input)
            // Marshalling twice to order keys lexographically
            var outputMap map[string]interface{}
            err = json.Unmarshal(output, &outputMap)
            must(err)
            outputJson, err := json.Marshal(outputMap)
            must(err)

			if string(want) != string(outputJson) {
				t.Fatalf("Test failed")
			}
		})
	}
}

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	// "github.com/aws/aws-lambda-go/lambda"
	// "github.com/aws/aws-sdk-go/aws"
)

// Services - Array of AWS Services and their IP ranges
type Services struct {
	Services []Service `json:"services"`
}

// Service - AWS Service and its IP Range used in by Amazon
type Service struct {
	IPPrefix           string `json:"ip_prefix"`
	Region             string `json:"region"`
	ServiceName        string `json:"service"`
	NetworkBorderGroup string `json:"network_border_group"`
}

// func main() {
//     lambda.Start(Handler)
// }

func main() {
	amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
	jsonFilePath := "ip-ranges.json"

	// Download file
	if err := downloadFile(jsonFilePath, amazonIPRangesURL); err != nil {
		panic(err)
	}

	fmt.Println("Downloaded: " + amazonIPRangesURL)

	// TODO: Verify date
	// var previousDate string
	// var createDate = "2014-11-19-23-29-02"
	// if previousDate != createDate {
	// 	fmt.Println("File has changed")
	// 	previousDate = &createDate
	// }

	// Parse file
	if err := parseFile(jsonFilePath); err != nil {
		panic(err)
	}

	fmt.Println("")
}

func downloadFile(downloadPath string, url string) error {
	// Get data
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Create file
	out, err := os.Create(downloadPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Write body to file
	_, err = io.Copy(out, resp.Body)
	return err
}

func parseFile(jsonFilePath string) error {
	// Open JSON file - https://tutorialedge.net/golang/parsing-json-with-golang/
	jsonFile, err := os.Open(jsonFilePath)
	if err != nil {
		return err
	}

	fmt.Println("Successfully opened: " + jsonFilePath)
	defer jsonFile.Close()

	// Read JSON file as byte array
	byteValue, _ := ioutil.ReadAll(jsonFile)

	// Initialize ipRanges data structure
	var services Services

	// Unmarshal byte array into ipRanges data structure
	json.Unmarshal(byteValue, &services)

	for i := 0; i < len(services.Services); i++ {
		fmt.Println("Service: " + services.Services[i].ServiceName)
	}
	return nil
}

// TODO: Get norified on changes
// SNS topic for updates on IP address list - AmazonIpSpaceChanged

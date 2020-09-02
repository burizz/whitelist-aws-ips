package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/ec2"
	// "github.com/aws/aws-lambda-go/lambda"
)

// Services - Array of AWS Services and their IP ranges
type Services struct {
	SyncToken    string    `json:"syncToken"`
	CreationDate string    `json:"createDate"`
	Prefixes     []Service `json:"prefixes"`
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
	securityGroupIDs := []string{"sg-0f467b0f6743bfc22", "sg-0ec48f26429e25bfe"}

	// amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
	// jsonFilePath := "ip-ranges.json"

	// Download file
	// if err := downloadFile(jsonFilePath, amazonIPRangesURL); err != nil {
	// 	panic(err)
	// }
	//
	// fmt.Println("Downloaded: " + amazonIPRangesURL)

	// TODO: Verify date
	// var previousDate string
	// var services Services
	// var createDate = services.CreationDate
	//
	// fmt.Println("Creation date: " + services.CreationDate)
	//
	// if previousDate != createDate {
	// 	fmt.Println("File has changed")
	// 	previousDate = createDate
	// }

	// Parse JSON file
	// if err := parseJSONFile(jsonFilePath); err != nil {
	// 	panic(err)
	// }

	if err := updateSecurityGroup(securityGroupIDs); err != nil {
		panic(err)
	}

	if err := describeSecurityGroup(securityGroupIDs); err != nil {
		panic(err)
	}
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

func parseJSONFile(jsonFilePath string) error {
	// Open JSON file - https://tutorialedge.net/golang/parsing-json-with-golang/
	jsonFile, err := os.Open(jsonFilePath)
	if err != nil {
		return err
	}

	fmt.Println("Successfully opened: " + jsonFilePath)
	defer jsonFile.Close()

	// Read JSON file as byte array
	byteValue, _ := ioutil.ReadAll(jsonFile)

	// Initialize Services data structure
	var services Services

	// Unmarshal byte array into ipRanges data structure
	json.Unmarshal(byteValue, &services)

	for i := 0; i < len(services.Prefixes); i++ {
		fmt.Printf("Service: %v - IP Prefix: %v", services.Prefixes[i].ServiceName, services.Prefixes[i].IPPrefix)
		fmt.Println()
	}

	return nil
}

func describeSecurityGroup(securityGroupIDs []string) error {
	// Expects array of SG IDs

	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("eu-central-1")},
	)

	if err != nil {
		panic(err)
	}

	// Create an EC2 service client
	svc := ec2.New(sess)

	// Describe current state of Security Groups
	describeResult, err := svc.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		GroupIds: aws.StringSlice(securityGroupIDs),
	})

	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case "InvalidGroupId.Malformed":
				fallthrough
			case "InvalidGroup.NotFound":
				exitErrorf("%s.", aerr.Message())
			}
		}
		exitErrorf("Unable to get descriptions for security groups, %v", err)
	}

	// Display each security group
	for _, group := range describeResult.SecurityGroups {
		fmt.Println("\n### Security Group : ")
		fmt.Println("\n", group)
	}
	return nil
}

func updateSecurityGroup(securityGroupIDs []string) error {
	// Expects array of SG IDs

	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("eu-central-1")},
	)

	if err != nil {
		panic(err)
	}

	// Create an EC2 service client
	svc := ec2.New(sess)

	// Iterate over each Security Group and update its Egress according to Input
	for _, securityGroup := range securityGroupIDs {
		// Define Egress rule Input
		input := &ec2.AuthorizeSecurityGroupEgressInput{
			GroupId: aws.String(securityGroup),
			IpPermissions: []*ec2.IpPermission{
				{
					FromPort:   aws.Int64(80),
					ToPort:     aws.Int64(80),
					IpProtocol: aws.String("tcp"),
					IpRanges: []*ec2.IpRange{
						{
							CidrIp: aws.String("10.11.0.0/16"),
						},
					},
				},
			},
		}

		// Update Security Group Egress rule from Input
		_, err := svc.AuthorizeSecurityGroupEgress(input)
		if err != nil {
			if aerr, ok := err.(awserr.Error); ok {
				switch aerr.Code() {
				default:
					fmt.Println(aerr.Error())
				}
			} else {
				// Print the error, cast err to awserr.Error to get the Code and
				// Message from an error.
				fmt.Println(err.Error())
			}
		}
	}
	return nil
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

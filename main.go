package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"

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
	// List of Security groups to be updated
	securityGroupIDs := []string{"sg-041c5e7daf95e16a3", "sg-00ffabccebd5efda2"}

	// List of services to be whitelisted - e.g. AMAZON, COUDFRONT, S3, EC2, API_GATEWAY, DYNAMODB, ROUTE53_HEALTHCHECKS, CODEBUILD
	servicesToBeWhitelist := []string{"S3"}

	// AWS JSON URL and local path to download it
	amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
	jsonFileLocalPath := "ip-ranges.json"

	// Download JSON file
	if err := downloadFile(jsonFileLocalPath, amazonIPRangesURL); err != nil {
		panic(err)
	}

	// Parse JSON file into Services data structure
	awsServices, err := parseJSONFile(jsonFileLocalPath)
	if err != nil {
		panic(err)
	}

	// Get IP ranges (/16) of all AWS Services that need to be whitelisted
	prefixesForWhitelisting, err := parseIPRanges(awsServices, servicesToBeWhitelist)
	if err != nil {
		panic(err)
	}

	var previousDate string

	// Verify if file has changes since last update
	var createDate = awsServices.CreationDate
	if previousDate != createDate {
		fmt.Println("AWS JSON file has changed since last run, updating creation date to " + createDate)
		previousDate = createDate
	}

	// Update Security Groups
	if err := updateSecurityGroup(securityGroupIDs, prefixesForWhitelisting); err != nil {
		panic(err)
	}

	// Print Security Group details
	if err := describeSecurityGroup(securityGroupIDs); err != nil {
		panic(err)
	}
}

func downloadFile(downloadPath, amazonIPRangesURL string) error {
	// Get data
	resp, err := http.Get(amazonIPRangesURL)
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
	defer fmt.Println("Downloaded: " + amazonIPRangesURL)
	return err
}

func parseJSONFile(jsonFilePath string) (Services, error) {
	// Initialize JSON data structure
	var awsServices Services

	// Open JSON file
	jsonFile, err := os.Open(jsonFilePath)
	if err != nil {
		return awsServices, err
	}

	fmt.Println("Successfully opened: " + jsonFilePath)
	defer jsonFile.Close()

	// Read JSON file as byte array
	byteValue, _ := ioutil.ReadAll(jsonFile)

	// Unmarshal byte array into ipRanges data structure
	json.Unmarshal(byteValue, &awsServices)

	return awsServices, nil
}

func parseIPRanges(awsServices Services, serviceWhitelist []string) ([]string, error) {
	var prefixesForWhitelisting []string

	// Go through list of AWS services; get their IP Prefixes if in Whitelist
	for i := 0; i < len(awsServices.Prefixes); i++ {
		// Check if service is to be whitelisted
		if searchStringInArray(awsServices.Prefixes[i].ServiceName, serviceWhitelist) {

			ipSlice := strings.Split(awsServices.Prefixes[i].IPPrefix, ".")
			ipFirstTwoOctets := strings.Join(ipSlice[0:2], ".")
			ipRange16 := ipFirstTwoOctets + ".0.0/16"

			// Check if IP subnet range (/16) is already in whitelisted ranges
			if !searchStringInArray(ipRange16, prefixesForWhitelisting) {
				prefixesForWhitelisting = append(prefixesForWhitelisting, ipRange16)
			}
		}
	}
	fmt.Printf("Amount of IP Ranges to be whitelisted [%v]", len(prefixesForWhitelisting))
	fmt.Printf("List of IP Ranges %v\n", prefixesForWhitelisting)

	return prefixesForWhitelisting, nil
}

func describeSecurityGroup(securityGroupIDs []string) error {
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
		fmt.Println("\nDescribe Security Group : ")
		fmt.Println("\n", group)
	}
	return nil
}

func updateSecurityGroup(securityGroupIDs []string, prefixesForWhitelisting []string) error {
	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String("eu-central-1")},
	)
	if err != nil {
		panic(err)
	}

	// Create an EC2 service client
	svc := ec2.New(sess)

	var counter int

	// Go over each Security Group and update its Egress according to Input
	for _, securityGroup := range securityGroupIDs {
		// Go over each IP range that is to be whitelisted
		for _, ipRange16 := range prefixesForWhitelisting {
			// Max 50 IP ranges per SG
			if counter <= 50 {
				// Define Egress rule Input
				fmt.Printf("\nAdding IP range [%v] to Security Group [%v]... \n", ipRange16, securityGroup)
				input := &ec2.AuthorizeSecurityGroupEgressInput{
					GroupId: aws.String(securityGroup),
					IpPermissions: []*ec2.IpPermission{
						{
							FromPort:   aws.Int64(443),
							ToPort:     aws.Int64(443),
							IpProtocol: aws.String("tcp"),
							IpRanges: []*ec2.IpRange{
								{
									CidrIp: aws.String(ipRange16),
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
						// Print the error, cast err to awserr.Error to get the Code and Message from an error.
						fmt.Println(err.Error())
					}
				} else {
					fmt.Println("IP Range added successfully")
				}

				prefixesForWhitelisting = removeFromList(prefixesForWhitelisting, 0)
				counter++
				fmt.Println(prefixesForWhitelisting)
			}
		}
	}
	return nil
}

func exitErrorf(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg+"\n", args...)
	os.Exit(1)
}

func searchStringInArray(searchString string, list []string) bool {
	for _, value := range list {
		if value == searchString {
			return true
		}
	}
	return false
}

func removeFromList(slice []string, index int) []string {
	return append(slice[:index], slice[index+1:]...)
}

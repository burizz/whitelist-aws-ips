package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
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
	// List of Security groups to be updated
	securityGroupIDs := []string{"sg-0f467b0f6743bfc22", "sg-0ec48f26429e25bfe"}

	// List of services to be whitelisted
	awsServiceWhitelist := []string{"API_GATEWAY", "AMAZON", "EC2"}

	// JSON URL and local path
	amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
	jsonFileLocalPath := "ip-ranges.json"

	// Download JSON file
	if err := downloadFile(jsonFileLocalPath, amazonIPRangesURL); err != nil {
		panic(err)
	}

	// Parse JSON file with IP Ranges
	services, prefixForWhitelisting, err := parseJSONFile(jsonFileLocalPath, awsServiceWhitelist)
	if err != nil {
		panic(err)
	}
	fmt.Println(prefixForWhitelisting)

	var previousDate string

	// Verify if file has changes since last update
	var createDate = services.CreationDate
	if previousDate != createDate {
		fmt.Println("File has changed since last run, updating creation date to " + createDate)
		previousDate = createDate
	}

	// TODO: Check list of services against already whitelisted ones
	fmt.Println(awsServiceWhitelist)

	// Update Security Groups
	if err := updateSecurityGroup(securityGroupIDs); err != nil {
		panic(err)
	}

	// Print Security Group details
	if err := describeSecurityGroup(securityGroupIDs); err != nil {
		panic(err)
	}

	// TODO Remaining tasks
	// 1. Get array of IP ranges we want to whitelist in parseJSONFile (or separate func)
	// 2. Check security group ip limit and how to work around it
	// 3. Send array of IP ranges to updateSecurityGroup func and loop through to update them
	// 4. Convert to lambda function handler
	// 5. Persistent way of storing previousDate var and checking it
	// 6. OPTIONAL: Update only entries that don't exist already, as it seems AWS handles the already exist part with errors
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

func parseJSONFile(jsonFilePath string, awsServiceWhitelist []string) (Services, []string, error) {
	// Initialize Services data structure
	var services Services
	var prefixForWhitelisting []string

	// Open JSON file
	jsonFile, err := os.Open(jsonFilePath)
	if err != nil {
		return services, prefixForWhitelisting, err
	}

	fmt.Println("Successfully opened: " + jsonFilePath)
	defer jsonFile.Close()

	// Read JSON file as byte array
	byteValue, _ := ioutil.ReadAll(jsonFile)

	// Unmarshal byte array into ipRanges data structure
	json.Unmarshal(byteValue, &services)

	// Go through list of AWS services; get their IP Prefixes if in Whitelist
	// https://golang.org/pkg/net/#IPNet.Contains ; https://stackoverflow.com/questions/19882961/go-golang-check-ip-address-in-range
	for i := 0; i < len(services.Prefixes); i++ {
		if searchStringInArray(services.Prefixes[i].ServiceName, awsServiceWhitelist) {
			// fmt.Println(services.Prefixes[i].ServiceName)
			// fmt.Println(services.Prefixes[i].IPPrefix)
			_, subnet, _ := net.ParseCIDR(services.Prefixes[i].IPPrefix)
			fmt.Println(subnet)
		}
		// fmt.Printf("Service: %v - IP Prefix: %v", services.Prefixes[i].ServiceName, services.Prefixes[i].IPPrefix)
		// fmt.Println()
	}

	return services, prefixForWhitelisting, nil
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

func searchStringInArray(searchString string, list []string) bool {
	for _, value := range list {
		if value == searchString {
			return true
		}
	}
	return false
}

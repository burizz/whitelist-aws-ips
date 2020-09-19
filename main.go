package main

import (
	"encoding/json"
	"errors"
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
	servicesToBeWhitelist := []string{"S3", "AMAZON"}

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

	// Check if AWS JSON file was modified since last run
	if err := checkIfFileModified(awsServices); err != nil {
		panic(err)
	}

	// Check how many SGs we need
	if err := checkSGCount(securityGroupIDs, prefixesForWhitelisting); err != nil {
		panic(err)
	}

	// Update Security Groups
	if err := updateSecurityGroups(securityGroupIDs, prefixesForWhitelisting); err != nil {
		panic(err)
	}

	// Describe Security Groups
	if err := describeSecurityGroups(securityGroupIDs); err != nil {
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

func checkIfFileModified(awsServices Services) error {
	// Check if the AWS JSON file was modified since last run

	var previousDate string
	// previousDate := "2020-09-18-21-51-15"

	// Verify if file has changes since last update
	var createDate = awsServices.CreationDate
	if previousDate != createDate {
		fmt.Println("Previous Date " + previousDate)
		fmt.Println("AWS JSON file has changed since last run, updating creation date to " + createDate)
		pointerToDate := &previousDate
		*pointerToDate = createDate
	} else {
		fmt.Println("Last modifed date : " + previousDate)
		errMsg := fmt.Sprintf("[ERROR]: File has not changed since last run, skipping ... ")
		return errors.New(errMsg)
	}
	return nil
}

func checkSGCount(securityGroupIDs []string, prefixesForWhitelisting []string) error {
	// Calculate how many SGs are needed to fit all IP Prefixes
	sgAmountNeeded := len(prefixesForWhitelisting)/50 + 1
	sgAmountProvided := len(securityGroupIDs)

	if sgAmountProvided < sgAmountNeeded {
		// fmt.Printf("You will need %d security groups, you porvided %d", sgAmountNeeded, sgAmountProvided)
		errMsg := fmt.Sprintf("[ERROR]: You will need [%d] Security Groups, you provided [%d]", sgAmountNeeded, sgAmountProvided)
		return errors.New(errMsg)
	}

	return nil
}

func updateSecurityGroups(securityGroupIDs []string, prefixesForWhitelisting []string) error {
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

	for _, securityGroup := range securityGroupIDs {
		// Go over each IP range and add it in SG
		for indexKey := range prefixesForWhitelisting {
			fmt.Printf("\nAdding IP range [%v] to Security Group [%v]... \n", prefixesForWhitelisting[indexKey], securityGroup)
			// Update Security Group with IP Prefix
			if err := awsUpdateSg(svc, prefixesForWhitelisting[indexKey], securityGroup); err != nil {
				return err
			}

			// Max 50 IP ranges per SG
			if counter == 50 {
				counter = 0
				break
			} else {
				counter++
			}
		}
	}
	return nil
}

func awsUpdateSg(ec2ClientSvc *ec2.EC2, prefixesForWhitelisting string, securityGroup string) error {
	// Define Egress rule Input
	input := &ec2.AuthorizeSecurityGroupEgressInput{
		GroupId: aws.String(securityGroup),
		IpPermissions: []*ec2.IpPermission{
			{
				FromPort:   aws.Int64(443),
				ToPort:     aws.Int64(443),
				IpProtocol: aws.String("tcp"),
				IpRanges: []*ec2.IpRange{
					{
						CidrIp: aws.String(prefixesForWhitelisting),
					},
				},
			},
		},
	}

	// Update Security Group Egress rule from Input
	_, err := ec2ClientSvc.AuthorizeSecurityGroupEgress(input)
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
	return nil
}

func describeSecurityGroups(securityGroupIDs []string) error {
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
		fmt.Println("\nDescribe Security Group Egress Rules : ")
		fmt.Println("\n", group)
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

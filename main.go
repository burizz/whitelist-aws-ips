package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
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

func main() {
	lambda.Start(LambdaHandler)
}

// LambdaHandler - Main AWS Lambda Entrypoint
func LambdaHandler() (msg string, err error) {
	// List of Security groups to be updated
	// securityGroupIDs := []string{"sg-041c5e7daf95e16a3", "sg-00ffabccebd5efda2"}
	// Comma-serpated env var - no spaces
	securityGroupIDs := strings.Split(os.Getenv("securityGroupIDs"), ",")

	// List of services to be whitelisted - e.g. AMAZON, COUDFRONT, S3, EC2, API_GATEWAY, DYNAMODB, ROUTE53_HEALTHCHECKS, CODEBUILD
	// servicesToBeWhitelist := []string{"S3", "API_GATEWAY"}
	// Comma-serpated env var - no spaces
	servicesToBeWhitelist := strings.Split(os.Getenv("servicesToBeWhitelist"), ",")

	// AWS JSON URL and local download path
	// amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
	amazonIPRangesURL := os.Getenv("amazonIPRangesURL")

	// AWS SSM Param Store that hold the last modified date of the JSON file - format "2020-09-18-21-51-15"
	// previousDateParamStore := "lastModifiedDateIPRanges"
	previousDateParamStore := os.Getenv("previousDateParamStore")

	// AWS DynamoDB table to be created that will maintain a list of all whitelisted IP Ranges
	// dynamoTableName := "whitelistedIPRanges"
	dynamoTableName := os.Getenv("dynamoTableName")

	// Set AWS Region
	// awsRegion := "eu-central-1"
	awsRegion := os.Getenv("awsRegion")

	errMsg := "Error"

	// Parse JSON file into Services data structure
	// var byteValue []byte
	var awsServices Services
	if jsonParseErr := parseJSONfromURL(amazonIPRangesURL, &awsServices); jsonParseErr != nil {
		return errMsg, jsonParseErr
	}

	// Get IP ranges (/16) of all AWS Services that need to be whitelisted
	prefixesForWhitelisting, ipParseErr := parseIPRanges(awsServices, servicesToBeWhitelist)
	if ipParseErr != nil {
		return errMsg, ipParseErr
	}

	// Check how many SGs we need
	sgCheckErr := checkSGCount(securityGroupIDs, prefixesForWhitelisting)
	if sgCheckErr != nil {
		return errMsg, sgCheckErr
	}

	// Check if AWS JSON file was modified since last run
	checkModifiedMsg, jsonModifiedErr := checkIfFileModified(awsServices, previousDateParamStore, awsRegion)
	if jsonModifiedErr != nil {
		return errMsg, jsonModifiedErr
	} else if checkModifiedMsg == "skipRun" {
		fmt.Println("Lambda exeuction completed successfully")
		return fmt.Sprintf("Success"), nil
	}

	// Check if Dynamo table exists
	fmt.Println("Checking if DynamoDB table exists ...")
	tablexists, dbDescribeErr := describeDynamoTable(dynamoTableName, awsRegion)
	if dbDescribeErr != nil {
		return errMsg, dbDescribeErr
	} else if !tablexists {
		fmt.Printf("Table [%v] does not exist\n", dynamoTableName)
		dbCreateErr := createDynamoTable(dynamoTableName, awsRegion)
		if dbCreateErr != nil {
			return errMsg, dbCreateErr
		}
	} else {
		fmt.Printf("[%v] table already exists\n", dynamoTableName)
	}

	var newIPCounter int
	var newIPRanges []string

	fmt.Println("Checking if any IP Ranges need to be whitelisted ...")
	for _, ip := range prefixesForWhitelisting {
		// Check if IP Ranges are already whitelsited
		ipPresent, dbGetErr := getDynamoItem(dynamoTableName, ip, awsRegion)
		if dbGetErr != nil {
			return errMsg, dbGetErr
		}
		if !ipPresent {
			// Update DynamoDB table with IP range that is to be whitelisted
			dbPutErr := putDynamoItem(dynamoTableName, ip, awsRegion)
			if dbPutErr != nil {
				return errMsg, dbPutErr
			}
			newIPRanges = append(newIPRanges, ip)
			newIPCounter++
		}
	}

	if newIPCounter > 0 {
		// Update Security Groups Egress rules
		updateSGErr := updateSecurityGroups(securityGroupIDs, newIPRanges, awsRegion)
		if updateSGErr != nil {
			return errMsg, updateSGErr
		}
		fmt.Printf("Successfully populated IP addresses in security groups %v\n", securityGroupIDs)
	} else if newIPCounter == 0 {
		fmt.Println("No new IP Ranges found, exiting")
		fmt.Println("Lambda exeuction completed successfully")
		return fmt.Sprintf("Success"), nil
	}

	// Describe Security Group Egress rules
	describeSGErr := describeSecurityGroups(securityGroupIDs, awsRegion)
	if describeSGErr != nil {
		return errMsg, describeSGErr
	}
	fmt.Println("Lambda exeuction completed successfully")
	return fmt.Sprintf("Success"), nil
}

// Parse JSON get IP Ranges

func parseJSONfromURL(url string, target interface{}) error {
	var myClient = &http.Client{Timeout: 10 * time.Second}
	r, err := myClient.Get(url)
	if err != nil {
		return fmt.Errorf("parseJSONfromURL: Cannot get JSON from URL : %w", err)
	}
	defer r.Body.Close()

	return json.NewDecoder(r.Body).Decode(target)
}

func parseIPRanges(awsServices Services, serviceWhitelist []string) ([]string, error) {
	var prefixesForWhitelisting []string

	// Go through list of AWS services; get their IP Prefixes if in Whitelist
	for i := 0; i < len(awsServices.Prefixes); i++ {
		// Check if service is to be whitelisted
		if searchStringInArray(awsServices.Prefixes[i].ServiceName, serviceWhitelist) {
			// Parse IP range and make it /16
			ipSlice := strings.Split(awsServices.Prefixes[i].IPPrefix, ".")
			ipFirstTwoOctets := strings.Join(ipSlice[0:2], ".")
			ipRange16 := ipFirstTwoOctets + ".0.0/16"

			// Check if IP subnet range (/16) is already in whitelisted ranges
			if !searchStringInArray(ipRange16, prefixesForWhitelisting) {
				prefixesForWhitelisting = append(prefixesForWhitelisting, ipRange16)
			}
		}
	}

	if len(prefixesForWhitelisting) <= 0 {
		return prefixesForWhitelisting, fmt.Errorf("parseIPRanges: No IP ranges added in list")
	}

	fmt.Printf("IP Ranges that need to be in whitelist: [%v]\n", len(prefixesForWhitelisting))
	fmt.Printf("List of IP Ranges : %v\n", prefixesForWhitelisting)
	return prefixesForWhitelisting, nil
}

// SSM Param Store Functions //

func checkIfFileModified(awsServices Services, previousDateParamStore string, awsRegion string) (responseMsg string, err error) {
	// Check if the AWS JSON file was modified since last run
	previousDate, err := getParamStoreValue(previousDateParamStore, awsRegion)
	if err != nil {
		return "Error", fmt.Errorf("checkIfFileModified: Cannot get SSM Parameter store: %w", err)
	}

	// Type of SSM parameter - String | StringList | SecureString
	ssmParamType := "String"

	// Verify if file has changes since last update
	var currentDate = awsServices.CreationDate
	if previousDate != currentDate {
		fmt.Println("AWS JSON file has changed since last run, previous date: " + previousDate)
		fmt.Println("Updating modified date to " + currentDate)
		// Update Date in SSM Param Store
		setParamStoreValue(previousDateParamStore, currentDate, ssmParamType, awsRegion)
		fmt.Println("Modified date changed")
		return fmt.Sprintf("Success"), nil
	}
	fmt.Println("Last modifed date : " + previousDate)
	fmt.Println("Amazon JSON file has not changed since last run, exiting ...")
	return fmt.Sprintf("skipRun"), nil
}

func getParamStoreValue(previousDateParamStore string, awsRegion string) (string, error) {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return previousDateParamStore, fmt.Errorf("getParamStoreValue: Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS SSM service client
	ssmService := ssm.New(sess, aws.NewConfig())
	// Get SSM Param Store value
	paramKey, err := ssmService.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(previousDateParamStore),
		WithDecryption: aws.Bool(false),
	})
	if err != nil {
		return previousDateParamStore, fmt.Errorf("getParamStoreValue: Get SSM Parameter: %w", err)
	}
	paramValue := *paramKey.Parameter.Value
	return paramValue, nil
}

func setParamStoreValue(previousDateParamStore string, currentDate string, paramType string, awsRegion string) error {
	// Create AWS session with default credentials(in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("setParamStoreValue: Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS SSM service client
	ssmService := ssm.New(sess, aws.NewConfig())
	_, err = ssmService.PutParameter(&ssm.PutParameterInput{
		Name:      aws.String(previousDateParamStore),
		Value:     aws.String(currentDate),
		Overwrite: aws.Bool(true),
		Type:      aws.String(paramType),
	})
	if err != nil {
		return fmt.Errorf("setParamStoreValue: Cannot put SSM Parameter value: %w", err)
	}

	fmt.Printf("Parameter store [%v] type [%v] updated successfully with value [%v]\n", previousDateParamStore, paramType, currentDate)
	return nil
}

// Security Group Functions //

func checkSGCount(securityGroupIDs []string, prefixesForWhitelisting []string) error {
	// Calculate how many SGs are needed to fit all IP Prefixes
	sgAmountNeeded := len(prefixesForWhitelisting)/50 + 1
	sgAmountProvided := len(securityGroupIDs)

	// Check if provided SGs are enough to fit all IP Ranges
	if sgAmountProvided < sgAmountNeeded || sgAmountProvided > sgAmountNeeded {
		errMsg := fmt.Sprintf("checkSGCount: You need [%d] Security Groups, you provided [%d]", sgAmountNeeded, sgAmountProvided)
		return errors.New(errMsg)
	}

	return nil
}

func updateSecurityGroups(securityGroupIDs []string, newIPRanges []string, awsRegion string) error {
	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("updateSecurityGroups: Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS EC2 service client
	svc := ec2.New(sess)

	var counter int

	for _, securityGroup := range securityGroupIDs {
		// Go over each IP range and add it in SG
		for indexKey := range newIPRanges {
			fmt.Printf("\nAdding IP range [%v] to Security Group [%v]... \n", newIPRanges[indexKey], securityGroup)
			// Update Security Group with IP Prefix
			if err := awsUpdateSg(svc, newIPRanges[indexKey], securityGroup); err != nil {
				return fmt.Errorf("updateSecurityGroups: Cannot update security group: %w", err)
			}

			// Max 50 IP ranges per SG
			if counter >= 50 {
				fmt.Printf("updateSecurityGroups: Security group %v full, moving to next one\n", securityGroup)
				counter = 0
				break
			} else {
				counter++
			}
		}
	}
	fmt.Println("Security group update finished")
	return nil
}

func awsUpdateSg(ec2ClientSvc *ec2.EC2, ipForWhitelist string, securityGroup string) error {
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
						CidrIp: aws.String(ipForWhitelist),
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
				return fmt.Errorf("awsUpdateSg: %v", aerr)
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

func describeSecurityGroups(securityGroupIDs []string, awsRegion string) error {
	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("describeSecurityGroups: Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS EC2 service client
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
				exitErrorf("describeSecurityGroups: %s.", aerr.Message())
			}
		}
		exitErrorf("describeSecurityGroups: Unable to get descriptions for security groups, %v", err)
	}

	// Display each security group
	for _, group := range describeResult.SecurityGroups {
		fmt.Println("\nDescribe Security Group Egress Rules : ")
		fmt.Println("\n", group)
	}
	return nil
}

// DynamoDB Functions //

func describeDynamoTable(dynamoTableName string, awsRegion string) (tableExists bool, err error) {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return false, fmt.Errorf("describeDynamoTable: Cannot create AWS config sessions: %w", err)
	}

	// Create a AWS DynamoDB service client
	svc := dynamodb.New(sess)

	input := &dynamodb.DescribeTableInput{
		TableName: aws.String(dynamoTableName),
	}

	_, err = svc.DescribeTable(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeResourceNotFoundException:
				return false, nil
			case dynamodb.ErrCodeInternalServerError:
				return true, fmt.Errorf("describeDynamoTable: %v", err)
			default:
				return true, fmt.Errorf("describeDynamoTable: %v", err)
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	}
	return true, nil
}

func createDynamoTable(dynamoTableName string, awsRegion string) (err error) {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("createDynamoTable: Cannot create AWS config sessions: %w", err)
	}

	// Create a AWS DynamoDB service client
	svc := dynamodb.New(sess)

	// Define Input for creating table
	input := &dynamodb.CreateTableInput{
		AttributeDefinitions: []*dynamodb.AttributeDefinition{
			{
				AttributeName: aws.String("awsIPRanges"),
				AttributeType: aws.String("S"),
			},
		},
		KeySchema: []*dynamodb.KeySchemaElement{
			{
				AttributeName: aws.String("awsIPRanges"),
				KeyType:       aws.String("HASH"),
			},
		},
		BillingMode: aws.String(dynamodb.BillingModePayPerRequest),
		TableName:   aws.String(dynamoTableName),
	}

	// Create the DynamoDB table using input
	_, err = svc.CreateTable(input)
	if err != nil {
		return fmt.Errorf("createDynamoTable: Cannot create DynamoDB table: %v", err.Error())
	}
	fmt.Printf("createDynamoTable: Successfully create table [%v]\n", dynamoTableName)
	return nil
}

func getDynamoItem(dynamoTableName string, ipRange string, awsRegion string) (ipPresent bool, err error) {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return false, fmt.Errorf("getDynamoItem: Cannot create AWS config sessions: %w", err)
	}

	// Create a AWS DynamoDB service client
	svc := dynamodb.New(sess)

	// Input for Dynamo Get Item operation
	// https://docs.aws.amazon.com/sdk-for-go/api/service/dynamodb/#DynamoDB.GetItem
	input := &dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"awsIPRanges": {
				S: aws.String(ipRange),
			},
		},
		TableName: aws.String(dynamoTableName),
	}

	fmt.Println("Checking item: " + ipRange)

	result, err := svc.GetItem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				return false, fmt.Errorf("getDynamoItem: %v", err)
			case dynamodb.ErrCodeResourceNotFoundException:
				// nil because we dont want it stop executing when item is missing in DB, as we add it later
				return false, nil
			case dynamodb.ErrCodeRequestLimitExceeded:
				return false, fmt.Errorf("getDynamoItem: %v", err)
			case dynamodb.ErrCodeInternalServerError:
				return false, fmt.Errorf("getDynamoItem: %v", err)
			default:
				return false, fmt.Errorf("getDynamoItem: %v", err)
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	}
	stringResult := result.GoString()

	// Regex to match IP address
	re := regexp.MustCompile(`(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)(\.(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)){3}`)
	// Return true if IP Range already in Dynamo table
	return re.MatchString(stringResult), nil
}

func putDynamoItem(dynamoTableName string, ipRange string, awsRegion string) error {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("putDynamoItem: Cannot create AWS config sessions: %w", err)
	}

	// Create a AWS DynamoDB service client
	svc := dynamodb.New(sess)

	// Input for Dynamo Put Item operation
	// https://docs.aws.amazon.com/sdk-for-go/api/service/dynamodb/#DynamoDB.PutItem
	input := &dynamodb.PutItemInput{
		Item: map[string]*dynamodb.AttributeValue{
			"awsIPRanges": {
				S: aws.String(ipRange),
			},
		},
		ReturnConsumedCapacity: aws.String("TOTAL"),
		TableName:              aws.String(dynamoTableName),
	}
	if _, err := svc.PutItem(input); err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeConditionalCheckFailedException:
				fmt.Println(dynamodb.ErrCodeConditionalCheckFailedException, aerr.Error())
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				fmt.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				fmt.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
			case dynamodb.ErrCodeItemCollectionSizeLimitExceededException:
				fmt.Println(dynamodb.ErrCodeItemCollectionSizeLimitExceededException, aerr.Error())
			case dynamodb.ErrCodeTransactionConflictException:
				fmt.Println(dynamodb.ErrCodeTransactionConflictException, aerr.Error())
			case dynamodb.ErrCodeRequestLimitExceeded:
				fmt.Println(dynamodb.ErrCodeRequestLimitExceeded, aerr.Error())
			case dynamodb.ErrCodeInternalServerError:
				fmt.Println(dynamodb.ErrCodeInternalServerError, aerr.Error())
			default:
				fmt.Println(aerr.Error())
			}
		} else {
			// Print the error, cast err to awserr.Error to get the Code and
			// Message from an error.
			fmt.Println(err.Error())
		}
	}
	fmt.Printf("Adding IP in Dynamo table : %v\n", ipRange)
	fmt.Printf("Successfully updated dynamo table %v\n", dynamoTableName)
	return nil
}

// Helpers //

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

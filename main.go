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
	"github.com/aws/aws-sdk-go/service/dynamodb"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ssm"
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

// https://docs.aws.amazon.com/lambda/latest/dg/golang-handler.html
// func main() {
//     lambda.Start(LambdaHandler)
// }

//func LambdaHandler() {}
func main() {
	// List of Security groups to be updated
	securityGroupIDs := []string{"sg-041c5e7daf95e16a3", "sg-00ffabccebd5efda2"}

	// List of services to be whitelisted - e.g. AMAZON, COUDFRONT, S3, EC2, API_GATEWAY, DYNAMODB, ROUTE53_HEALTHCHECKS, CODEBUILD
	servicesToBeWhitelist := []string{"S3"}

	// AWS JSON URL and local download path
	amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
	jsonFileLocalPath := "ip-ranges.json"

	// AWS SSM Param Store that hold the last modified date of the JSON file - format "2020-09-18-21-51-15"
	previousDateParamStore := "lastModifiedDateIPRanges"

	// AWS DynamoDB table to be created that will maintain a list of all whitelisted IP Ranges
	dynamoTableName := "whitelistedIPRanges"

	// Set AWS Region
	awsRegion := "eu-central-1"

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

	// Check how many SGs we need
	if err := checkSGCount(securityGroupIDs, prefixesForWhitelisting); err != nil {
		panic(err)
	}

	// Check if AWS JSON file was modified since last run
	if err := checkIfFileModified(awsServices, previousDateParamStore, awsRegion); err != nil {
		panic(err)
	}

	// Create DynamoDB table if it doesn't exist
	if err := createDynamoTable(dynamoTableName, awsRegion); err != nil {
		panic(err)
	}

	if err := getDynamoItem(dynamoTableName, awsRegion); err != nil {
		panic(err)
	}

	// Update DynamoDB table with IP ranges
	if err := putDynamoItem(dynamoTableName, prefixesForWhitelisting, awsRegion); err != nil {
		panic(err)
	}

	// Update Security Groups
	if err := updateSecurityGroups(securityGroupIDs, prefixesForWhitelisting, awsRegion); err != nil {
		panic(err)
	}

	// Describe Security Groups
	// if err := describeSecurityGroups(securityGroupIDs); err != nil {
	// 	panic(err)
	// }
}

func downloadFile(downloadPath, amazonIPRangesURL string) error {
	// Get data
	resp, err := http.Get(amazonIPRangesURL)
	if err != nil {
		return fmt.Errorf("Cannot open remote URL: %w", err)
	}
	defer resp.Body.Close()

	// Create file
	out, err := os.Create(downloadPath)
	if err != nil {
		return fmt.Errorf("Cannot create local file: %w", err)
	}
	defer out.Close()

	// Write body to file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return fmt.Errorf("Cannot write Response Body to file: %w", err)
	}

	fmt.Println("Successfully downloaded: " + amazonIPRangesURL)
	return nil
}

func parseJSONFile(jsonFilePath string) (Services, error) {
	// Initialize JSON data structure
	var awsServices Services

	// Open JSON file
	jsonFile, err := os.Open(jsonFilePath)
	if err != nil {
		return awsServices, fmt.Errorf("Cannot open JSON file: %w", err)
	}

	fmt.Println("Successfully opened: " + jsonFilePath)
	defer jsonFile.Close()

	// Read JSON file as byte array
	byteValue, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		return awsServices, fmt.Errorf("Cannot read JSON file as byte array: %w", err)
	}
	// Unmarshal byte array into ipRanges data structure
	if err := json.Unmarshal(byteValue, &awsServices); err != nil {
		return awsServices, fmt.Errorf("Cannot Unmarshal JSON into awsService data structure: %w", err)
	}

	fmt.Println("JSON file parsed")
	return awsServices, nil
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
		return prefixesForWhitelisting, fmt.Errorf("No IP ranges added in list")
	}

	fmt.Printf("Amount of IP Ranges to be whitelisted [%v]", len(prefixesForWhitelisting))
	fmt.Printf("List of IP Ranges %v\n", prefixesForWhitelisting)
	return prefixesForWhitelisting, nil
}

func checkIfFileModified(awsServices Services, previousDateParamStore string, awsRegion string) error {
	// Check if the AWS JSON file was modified since last run
	previousDate, err := getParamStoreValue(previousDateParamStore, awsRegion)
	if err != nil {
		return fmt.Errorf("Cannot get SSM Parameter store: %w", err)
	}

	// Type of SSM parameter - String | StringList | SecureString
	ssmParamType := "String"

	// Verify if file has changes since last update
	var currentDate = awsServices.CreationDate
	if previousDate != currentDate {
		fmt.Println("Previous Date " + previousDate)
		fmt.Println("AWS JSON file has changed since last run, updating creation date to " + currentDate)
		// Update Date in SSM Param Store
		setParamStoreValue(previousDateParamStore, currentDate, ssmParamType, awsRegion)
	} else {
		fmt.Println("Last modifed date : " + previousDate)
		return fmt.Errorf("Amazon JSON file has not changed since last run, exiting ... ")
	}
	return nil
}

func getParamStoreValue(previousDateParamStore string, awsRegion string) (string, error) {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return previousDateParamStore, fmt.Errorf("Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS SSM service client
	ssmService := ssm.New(sess, aws.NewConfig())
	// Get SSM Param Store value
	paramKey, err := ssmService.GetParameter(&ssm.GetParameterInput{
		Name:           aws.String(previousDateParamStore),
		WithDecryption: aws.Bool(false),
	})
	if err != nil {
		return previousDateParamStore, fmt.Errorf("Get SSM Parameter: %w", err)
	}
	paramValue := *paramKey.Parameter.Value
	fmt.Println(paramValue)
	return paramValue, nil
}

func setParamStoreValue(previousDateParamStore string, currentDate string, paramType string, awsRegion string) error {
	// Create AWS session with default credentials(in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS SSM service client
	ssmService := ssm.New(sess, aws.NewConfig())
	paramKey, err := ssmService.PutParameter(&ssm.PutParameterInput{
		Name:      aws.String(previousDateParamStore),
		Value:     aws.String(currentDate),
		Overwrite: aws.Bool(true),
		Type:      aws.String(paramType),
	})
	if err != nil {
		return fmt.Errorf("Cannot put SSM Parameter value: %w", err)
	}

	fmt.Printf("Parameter store [%v] type [%v] updated successfully with value [%v]\n", previousDateParamStore, paramType, currentDate)
	fmt.Println(paramKey)
	return nil
}

func checkSGCount(securityGroupIDs []string, prefixesForWhitelisting []string) error {
	// Calculate how many SGs are needed to fit all IP Prefixes
	sgAmountNeeded := len(prefixesForWhitelisting)/50 + 1
	sgAmountProvided := len(securityGroupIDs)

	if sgAmountProvided < sgAmountNeeded {
		// fmt.Printf("You will need %d security groups, you porvided %d", sgAmountNeeded, sgAmountProvided)
		errMsg := fmt.Sprintf("You will need [%d] Security Groups, you provided [%d]", sgAmountNeeded, sgAmountProvided)
		return errors.New(errMsg)
	}

	return nil
}

func updateSecurityGroups(securityGroupIDs []string, prefixesForWhitelisting []string, awsRegion string) error {
	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("Cannot create AWS config sessions: %w", err)
	}

	// Create an AWS EC2 service client
	svc := ec2.New(sess)

	var counter int

	for _, securityGroup := range securityGroupIDs {
		// Go over each IP range and add it in SG
		for indexKey := range prefixesForWhitelisting {
			fmt.Printf("\nAdding IP range [%v] to Security Group [%v]... \n", prefixesForWhitelisting[indexKey], securityGroup)
			// Update Security Group with IP Prefix
			if err := awsUpdateSg(svc, prefixesForWhitelisting[indexKey], securityGroup); err != nil {
				return fmt.Errorf("Cannot update security group: %w", err)
			}

			// Max 50 IP ranges per SG
			if counter >= 50 {
				fmt.Printf("Security group %v full, moving to next one", securityGroup)
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

func describeSecurityGroups(securityGroupIDs []string, awsRegion string) error {
	// Create AWS session with default credentials and region (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("Cannot create AWS config sessions: %w", err)
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

func createDynamoTable(dynamoTableName string, awsRegion string) (err error) {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("Cannot create AWS config sessions: %w", err)
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
		return fmt.Errorf("Cannot create DynamoDB table: %v", err.Error())
	}
	fmt.Printf("Successfully create table %v", dynamoTableName)
	return nil
}

func getDynamoItem(dynamoTableName string, awsRegion string) error {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("Cannot create AWS config sessions: %w", err)
	}

	// Create a AWS DynamoDB service client
	svc := dynamodb.New(sess)

	// Input for Dynamo Get Item operation
	// https://docs.aws.amazon.com/sdk-for-go/api/service/dynamodb/#DynamoDB.GetItem
	input := &dynamodb.GetItemInput{
		Key: map[string]*dynamodb.AttributeValue{
			"awsIPRanges": {
				S: aws.String("test"),
			},
		},
		TableName: aws.String(dynamoTableName),
	}

	result, err := svc.GetItem(input)
	if err != nil {
		if aerr, ok := err.(awserr.Error); ok {
			switch aerr.Code() {
			case dynamodb.ErrCodeProvisionedThroughputExceededException:
				fmt.Println(dynamodb.ErrCodeProvisionedThroughputExceededException, aerr.Error())
			case dynamodb.ErrCodeResourceNotFoundException:
				fmt.Println(dynamodb.ErrCodeResourceNotFoundException, aerr.Error())
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
		return err
	}
	fmt.Println(result)
	return nil
}

func putDynamoItem(dynamoTableName string, inputIPRanges []string, awsRegion string) error {
	// Create AWS session with default credentials (in ENV vars)
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion)},
	)
	if err != nil {
		return fmt.Errorf("Cannot create AWS config sessions: %w", err)
	}

	// Create a AWS DynamoDB service client
	svc := dynamodb.New(sess)

	// Input for Dynamo Put Item operation
	// Item map[string]*AttributeValue `type:"map" required:"true"`
	// https://docs.aws.amazon.com/sdk-for-go/api/service/dynamodb/#DynamoDB.PutItem
	for _, item := range inputIPRanges {
		input := &dynamodb.PutItemInput{
			Item: map[string]*dynamodb.AttributeValue{
				"awsIPRanges": {
					S: aws.String(item),
				},
			},
			ReturnConsumedCapacity: aws.String("TOTAL"),
			TableName:              aws.String(dynamoTableName),
		}

		_, err := svc.PutItem(input)
		if err != nil {
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
			return err
		}
	}
	fmt.Printf("Successfully updated table %v", dynamoTableName)
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

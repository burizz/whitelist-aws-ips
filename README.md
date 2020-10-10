# Whitelist AWS IP Ranges - Lambda

Lambda for whitelisting Amazon IP ranges in Security Group outbound rules.

Works by updating Security Group Egress rules with a list of AWS IP Ranges based on AWS Service Nam.
Pulls latest JSON from - https://docs.aws.amazon.com/general/latest/gr/aws-ip-ranges.html

This is written in Go as practice for using the AWS SDK and Golang in general.

**Initial release - v1.0** :
- [x] Download Amazon IP range file and parse JSON data structure
- [x] Update list of IP ranges in Security Groups / Describe Security Groups
- [x] Work around SG limit of 60 inbound/outbound rules
- [x] Persistent way of storing JSON modified date - SSM Param Store
- [x] Better error handling
- [x] Make AWS region configurable
- [x] Update only entries that don't exist already - DynamoDB persistence
  - [x] Check if DynamoDB table exists; 
  - [x] Create DynamoDB table if doesn't exist
  - [x] Add list of IP ranges in DynamoDB table
  - [x] Only update if an entry is missing
  - [x] Create list of IPs to be added in SG from DynamoDB Table
- [ ] Implement Lambda function handler instead of main

**Improvements - v1.1** : 
- [x] Fix bug with security group updates when IPs are less than 50 (they get duplicated in all SGs)
- [x] Combine download and json parse funcs into one using decoder (no need to download the file locally)
- [ ] Create SSM param store if it doesnt exist
- [ ] Move all AWS svc client duplications to an init() function - https://tutorialedge.net/golang/the-go-init-function/; we can have more than 1 init() to initialize the different svc clients
- [ ] Figure out a good way to link all SGs at the end into a single one - some sort of inheritance ?

## Build Lambda zip

Linux : 
```
# Get dependency
go get -u github.com/aws/aws-lambda-go/cmd/build-lambda-zip
```

```
# Compile and zip
GOOS=linux go build main.go
zip go_lambda.zip main
```

Windows : 
```
# Get dependency
go get -u github.com/aws/aws-lambda-go/cmd/build-lambda-zip
```

```
# Compile and zip
$env:GOOS = "linux"
$env:CGO_ENABLED = "0"
$env:GOARCH = "amd64"
go build -o main main.go
~\Go\Bin\build-lambda-zip.exe -output go_lambda.zip main
```

### Create Lambda
```
aws lambda create-function --function-name my-function --runtime go1.x \
  --zip-file fileb://go_lambda.zip --handler main \
  --role arn:aws:iam::123456789012:role/execution_role
```

### Variables
```
// List of Security groups to be updated
securityGroupIDs := []string{"sg-041c5e7daf95e16a3"}

// List of services to be whitelisted - e.g. AMAZON, COUDFRONT, S3, EC2, API_GATEWAY, DYNAMODB, ROUTE53_HEALTHCHECKS, CODEBUILD
servicesToBeWhitelist := []string{"S3"}

// AWS JSON URL and local download path
amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"

// AWS SSM Param Store that hold the last modified date of the JSON file - format "2020-09-18-21-51-15"
previousDateParamStore := "lastModifiedDateIPRanges"

// AWS DynamoDB table to be created that will maintain a list of all whitelisted IP Ranges
dynamoTableName := "whitelistedIPRanges"

// Set AWS Region
awsRegion := "eu-central-1"
```


## Test locally :

```
# Go Dependencies
go get -u github.com/aws/aws-sdk-go/...
go get -u github.com/aws/aws-lambda-go/lambda
```

### Setup AWS Credentials
```
# Linux
export AWS_ACCESS_KEY_ID=YOUR_AKID
export AWS_SECRET_ACCESS_KEY=YOUR_SECRET_KEY
```

```
# Windows
$env:AWS_ACCESS_KEY_ID='YOUR_AKID'
$env:AWS_SECRET_ACCESS_KEY='YOUR_SECRET_KEY'
```


### Common errors

##### SSM Param Store
```
# You need a IAM policy with permissions to SSM : 
panic: AccessDeniedException: User: arn:aws:iam::111111111111:user/test is not authorized to perform: ssm:GetParameter on resource: arn:aws:ssm:eu-central-1:111111111111:parameter/lastModifiedDateIPRanges
        status code: 400, request id: 4a0ab454-176d-4bc6-9418-78529da0f944

# Make sure that no permissions boundary is also not limiting you in case you already have an IAM policy with sufficient access.
```

##### Security Groups
```
# You did not provide endough Security Groups to fit all IP ranges :
panic: [ERROR]: You will need [5] Security Groups, you provided [2]
```

### Example Output
```go run .\main.go
Downloaded: https://ip-ranges.amazonaws.com/ip-ranges.json
Successfully opened: ip-ranges.json
Amount of IP Ranges to be whitelisted [10]
[3.5.0.0/16 52.95.0.0/16 52.219.0.0/16 108.175.0.0/16 52.92.0.0/16 54.231.0.0/16 52.218.0.0/16 52.216.0.0/16 54.222.0.0/16 52.82.0.0/16]
File has changed since last run, updating creation date to 2020-09-11-01-51-14

Adding IP range [3.5.0.0/16] to Security Group [sg-041c5e7daf95e16a3]...
IP Range added successfully

Adding IP range [3.5.0.0/16] to Security Group [sg-00ffabccebd5efda2]...
IP Range added successfully

Adding IP range [52.95.0.0/16] to Security Group [sg-041c5e7daf95e16a3]...
IP Range added successfully

Adding IP range [52.95.0.0/16] to Security Group [sg-00ffabccebd5efda2]...
IP Range added successfully

Adding IP range [52.219.0.0/16] to Security Group [sg-041c5e7daf95e16a3]...
IP Range added successfully

Describe Security Group :

 {
  Description: "not used",
  GroupId: "sg-041c5e7daf95e16a3",
  GroupName: "test1234",
  IpPermissionsEgress: [{
      FromPort: 443,
      IpProtocol: "tcp",
      IpRanges: [
        {
          CidrIp: "3.5.0.0/16"
        },
        {
          CidrIp: "52.95.0.0/16"
        },
        {
          CidrIp: "52.219.0.0/16"
        }
      ],
      ToPort: 443
    }],
  OwnerId: "722377226063",
  VpcId: "vpc-342c735c"
}
```

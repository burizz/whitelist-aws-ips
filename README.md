# Whitelist AWS IP Ranges - Lambda

Lambda for whitelisting Amazon IP ranges in Security Group outbound rules.

Works by updating Security Group Egress rules with a list of AWS IP Ranges based on AWS Service Nam.
Pulls latest JSON from - https://docs.aws.amazon.com/general/latest/gr/aws-ip-ranges.html

This is written in Go as practice for using the AWS SDK and Golang in general.

## Build Lambda zip

Linux : 
```
# Get dependency
go get -u github.com/aws/aws-lambda-go/cmd/build-lambda-zip
```

```
# Compile and zip
GOOS=linux go build main.go && zip go_lambda.zip main
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
go build -o main main.go; ~\Go\Bin\build-lambda-zip.exe -output go_lambda.zip main
```

### Create Lambda
```
aws lambda create-function --function-name my-function --runtime go1.x \
  --zip-file fileb://go_lambda.zip --handler main \
  --role arn:aws:iam::123456789012:role/execution_role

```
Setup Lambda env variables :

|  Key | Value  | Description |
|---   |---     |---          |
| amazonIPRangesURL |	https://ip-ranges.amazonaws.com/ip-ranges.json | URL with AWS IP ranges |
| awsRegion  | eu-central-1 | AWS Region |
| dynamoTableName  | whitelistedIPRanges | Name of DynamoDB table to store whitelisted IP ranges |
| previousDateParamStore  | lastModifiedDateIPRanges | SSM Param store name to keep modified date of AWS JSON file |
| securityGroupIDs  | sg-041c5e7daf95e16a3 | Comma separated list of Security groups (no spaces) |
| servicesToBeWhitelist  | S3 | Comma separated list of AWS Services from JSON list (no spaces) |


Setup a Lambda Trigger, e.g. time based
EventBridge trigger - scheduled expressions
```
# Run every hour
cron(0 * * * ? *)

# Run every 30 min
cron(0/30 * * * ? *)
```

### IAM Policy Permissions needed
TODO: add IAM policy example
```
SSM Param Store - Get/Put Parameter
Cloudwatch - log groups
DynamoDB - create/describe table, get/put item
Security Groups - update
```


### Test locally :

```
# Go Dependencies
go get -u github.com/aws/aws-sdk-go/...
go get -u github.com/aws/aws-lambda-go/lambda
```


Setup AWS Credentials
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

Hardcoded Variables example
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

### Common errors

##### SSM Param Store
```
# You need a IAM policy with permissions to SSM : 
panic: AccessDeniedException: User: arn:aws:iam::111111111111:user/test is not authorized to perform: ssm:GetParameter on resource: arn:aws:ssm:eu-central-1:111111111111:parameter/lastModifiedDateIPRanges
        status code: 400, request id: 4a0ab454-176d-4bc6-9418-78529da0f944

# Make sure that no permissions boundary is also not limiting you in case you already have an IAM policy with sufficient access.
```

```
"errorMessage": "updateSecurityGroups: Cannot update security group: awsUpdateSg: UnauthorizedOperation: You are not authorized to perform this operation. Encoded authorization failure message: ... n\tstatus code: 403, request id: a2cfb4ab-b102-4eaa-bda6-cc9d326c4fa7",
"errorType": "wrapError"

```


##### Security Groups
```
# You did not provide endough Security Groups to fit all IP ranges :
panic: [ERROR]: You will need [5] Security Groups, you provided [2]
```

### backlog:

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
- [x] Implement Lambda handler

**Improvements - v1.1** : 
- [x] Combine download and json parse funcs into one using decoder (no need to download the file locally)
- [x] Add Lambda trigger example in Readme
- [x] Move all vars to be taken from Lambda ENV vars instead of hardcoded
- [x] Handle dependencies as Go modules
- [ ] Create SSM param store if it doesnt exist
- [ ] Move all AWS svc client duplications to an init() function - https://tutorialedge.net/golang/the-go-init-function/; we can have more than 1 init() to initialize the different svc clients
- [ ] Figure out a good way to link all SGs at the end into a single one - some sort of inheritance ?
- [ ] Add IAM policy example with minimal access needed in Readme

**Fix Bugs** :
- [x] Security group updates when IPs are less than 50 (they get duplicated in all SGs)
- [x] Dynamo update items when the table needs to be created - it adds IPs in table but but goes to case where No new IP Ranges were found and doesnt update the SGs, although the Dynamo table is completely empty. putDynamoItem() also doesn't print its success messages, although items are successfully created there. But on next Lambda run eveyrthing is fine because Dynamo table with items already exist. Seems to be related to the regex check, but not sure why it happens only when table doesn't initially excist. We are hitting this condition : case dynamodb.ErrCodeResourceNotFoundException: return true, nil



##### Successful run example outputs
1. When the AWS JSON file has not changed
```
START RequestId: fb9dddc4-3a0a-4c1a-bc75-a82f0f28373d Version: $LATEST
IP Ranges that need to be in whitelist: [10]
List of IP Ranges : [3.5.0.0/16 52.219.0.0/16 52.95.0.0/16 108.175.0.0/16 52.92.0.0/16 54.231.0.0/16 52.218.0.0/16 52.216.0.0/16 54.222.0.0/16 52.82.0.0/16]
Last modifed date : 2020-10-10-04-51-17
Amazon JSON file has not changed since last run, exiting ...
Lambda exeuction completed successfully
END RequestId: fb9dddc4-3a0a-4c1a-bc75-a82f0f28373d
REPORT RequestId: fb9dddc4-3a0a-4c1a-bc75-a82f0f28373d	Duration: 358.33 ms	Billed Duration: 400 ms	Memory Size: 512 MB	Max Memory Used: 55 MB	Init Duration: 124.49 ms	
```

2. When AWS JSON file has changed but there are no new IP Ranges to be whitelisted : 
```
START RequestId: 81d73f22-bb79-47ac-b151-9b7787a20b36 Version: $LATEST
IP Ranges that need to be in whitelist: [10]
List of IP Ranges : [3.5.0.0/16 52.219.0.0/16 52.95.0.0/16 108.175.0.0/16 52.92.0.0/16 54.231.0.0/16 52.218.0.0/16 52.216.0.0/16 54.222.0.0/16 52.82.0.0/16]
AWS JSON file has changed since last run, previous date: 2020-10-10-04-51-17213
Updating creation date to 2020-10-10-04-51-17
Parameter store [lastModifiedDateIPRanges] type [String] updated successfully with value [2020-10-10-04-51-17]
Modified date changed
Checking if DynamoDB table exists ...
[whitelistedIPRanges] table already exists
Checking if any IP Ranges need to be whitelisted ...
Checking item: 3.5.0.0/16
Checking item: 52.219.0.0/16
Checking item: 52.95.0.0/16
Checking item: 108.175.0.0/16
Checking item: 52.92.0.0/16
Checking item: 54.231.0.0/16
Checking item: 52.218.0.0/16
Checking item: 52.216.0.0/16
Checking item: 54.222.0.0/16
Checking item: 52.82.0.0/16
No new IP Ranges found, exiting
Lambda exeuction completed successfully
END RequestId: 81d73f22-bb79-47ac-b151-9b7787a20b36
REPORT RequestId: 81d73f22-bb79-47ac-b151-9b7787a20b36	Duration: 185.74 ms	Billed Duration: 200 ms	Memory Size: 512 MB	Max Memory Used: 56 MB	
```

3. When there are new a few new IP Addresses to be added : 
```
START RequestId: f306eedc-8341-429c-be10-6e5c6a8f5dfe Version: $LATEST
IP Ranges that need to be in whitelist: [10]
List of IP Ranges : [3.5.0.0/16 52.219.0.0/16 52.95.0.0/16 108.175.0.0/16 52.92.0.0/16 54.231.0.0/16 52.218.0.0/16 52.216.0.0/16 54.222.0.0/16 52.82.0.0/16]
AWS JSON file has changed since last run, previous date: 2020-10-10-04-51-17312
Updating creation date to 2020-10-10-04-51-17
Parameter store [lastModifiedDateIPRanges] type [String] updated successfully with value [2020-10-10-04-51-17]
Modified date changed
Checking if DynamoDB table exists ...
[whitelistedIPRanges] table already exists
Checking if any IP Ranges need to be whitelisted ...
Checking item: 3.5.0.0/16
Checking item: 52.219.0.0/16
Adding IP in Dynamo table : 52.219.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.95.0.0/16
Checking item: 108.175.0.0/16
Checking item: 52.92.0.0/16
Checking item: 54.231.0.0/16
Checking item: 52.218.0.0/16
Adding IP in Dynamo table : 52.218.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.216.0.0/16
Checking item: 54.222.0.0/16
Checking item: 52.82.0.0/16

Adding IP range [52.219.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.218.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully
Security group update finished
Successfully populated IP addresses in security groups [sg-041c5e7daf95e16a3]

Describe Security Group Egress Rules : 

 {
  Description: "not used",
  GroupId: "sg-041c5e7daf95e16a3",
  GroupName: "test1234",
  IpPermissionsEgress: [{
      FromPort: 443,
      IpProtocol: "tcp",
      IpRanges: [
        {
          CidrIp: "52.95.0.0/16"
        },
        {
          CidrIp: "108.175.0.0/16"
        },
        {
          CidrIp: "52.92.0.0/16"
        },
        {
          CidrIp: "54.231.0.0/16"
        },
        {
          CidrIp: "52.216.0.0/16"
        },
        {
          CidrIp: "54.222.0.0/16"
        },
        {
          CidrIp: "52.82.0.0/16"
        },
        {
          CidrIp: "3.5.0.0/16"
        },
        {
          CidrIp: "52.219.0.0/16"
        },
        {
          CidrIp: "52.218.0.0/16"
        }
      ],
      ToPort: 443
    }],
  OwnerId: "722377226063",
  VpcId: "vpc-342c735c"
}
Lambda exeuction completed successfully
END RequestId: f306eedc-8341-429c-be10-6e5c6a8f5dfe
REPORT RequestId: f306eedc-8341-429c-be10-6e5c6a8f5dfe	Duration: 542.66 ms	Billed Duration: 600 ms	Memory Size: 512 MB	Max Memory Used: 59 MB	
```

4. On First Lambda run if all IAM policy permissions are set okay. Ignore the resource not found exceptions on the first run as they should not appear aferwards.
```
START RequestId: 7e3275c6-1169-4080-9178-aa0c9fb09485 Version: $LATEST
IP Ranges that need to be in whitelist: [10]
List of IP Ranges : [3.5.0.0/16 52.219.0.0/16 52.95.0.0/16 108.175.0.0/16 52.92.0.0/16 54.231.0.0/16 52.218.0.0/16 52.216.0.0/16 54.222.0.0/16 52.82.0.0/16]
AWS JSON file has changed since last run, previous date: 2020-10-10-04-51-17132213
Updating creation date to 2020-10-10-04-51-17
Parameter store [lastModifiedDateIPRanges] type [String] updated successfully with value [2020-10-10-04-51-17]
Modified date changed
Checking if DynamoDB table exists ...
Table [whitelistedIPRanges] does not exist
createDynamoTable: Successfully create table [whitelistedIPRanges]
Checking if any IP Ranges need to be whitelisted ...
Checking item: 3.5.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 3.5.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.219.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 52.219.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.95.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 52.95.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 108.175.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 108.175.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.92.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 52.92.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 54.231.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 54.231.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.218.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 52.218.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.216.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 52.216.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 54.222.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 54.222.0.0/16
Successfully updated dynamo table whitelistedIPRanges
Checking item: 52.82.0.0/16
ResourceNotFoundException ResourceNotFoundException: Requested resource not found
Adding IP in Dynamo table : 52.82.0.0/16
Successfully updated dynamo table whitelistedIPRanges

Adding IP range [3.5.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.219.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.95.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [108.175.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.92.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [54.231.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.218.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.216.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [54.222.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully

Adding IP range [52.82.0.0/16] to Security Group [sg-041c5e7daf95e16a3]... 
IP Range added successfully
Security group update finished
Successfully populated IP addresses in security groups [sg-041c5e7daf95e16a3]

Describe Security Group Egress Rules : 

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
          CidrIp: "52.219.0.0/16"
        },
        {
          CidrIp: "52.95.0.0/16"
        },
        {
          CidrIp: "108.175.0.0/16"
        },
        {
          CidrIp: "52.92.0.0/16"
        },
        {
          CidrIp: "54.231.0.0/16"
        },
        {
          CidrIp: "52.218.0.0/16"
        },
        {
          CidrIp: "52.216.0.0/16"
        },
        {
          CidrIp: "54.222.0.0/16"
        },
        {
          CidrIp: "52.82.0.0/16"
        }
      ],
      ToPort: 443
    }],
  OwnerId: "722377226063",
  VpcId: "vpc-342c735c"
}
Lambda exeuction completed successfully
END RequestId: 7e3275c6-1169-4080-9178-aa0c9fb09485
REPORT RequestId: 7e3275c6-1169-4080-9178-aa0c9fb09485	Duration: 1780.98 ms	Billed Duration: 1800 ms	Memory Size: 512 MB	Max Memory Used: 59 MB	
```
# AWS IP Ranges - Lambda

Lambda for whitelisting Amazon IP ranges in Security Group outbound rules

Works by updating Security Group Egress rules with a list of AWS IP Ranges based on AWS Service Nam.
Pulls latest JSON from - https://docs.aws.amazon.com/general/latest/gr/aws-ip-ranges.html

This is still work in progress : 
TODO :
- [x] Download Amazon IP range file and parse JSON data structure
- [x] Update list of IP ranges in Security Groups / Describe Security Groups
- [x] Work around SG limit of 60 inbound/outbound rules
- [x] Persistent way of storing JSON modified date - SSM Param Store
- [x] Better error handling
- [x] Make AWS region configurable
- [ ] Update only entries that don't exist already - DynamoDB persistence
- [ ] Implement lambda function handler instead of main
- [ ] Figure out a good way to link all SGs at the end into a single one - inheritance ?

### Go Dependencies
```
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

### Variables
```
// List of Security groups to be updated
securityGroupIDs := []string{"sg-041c5e7daf95e16a3", "sg-00ffabccebd5efda2"}

// List of services to be whitelisted - e.g. AMAZON, COUDFRONT, S3, EC2, API_GATEWAY, DYNAMODB, ROUTE53_HEALTHCHECKS, CODEBUILD
servicesToBeWhitelist := []string{"S3"}

// AWS JSON URL and local download path
amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
jsonFileLocalPath := "ip-ranges.json"

// AWS SSM Param Store that hold the last modified date of the JSON file - format "2020-09-18-21-51-15"
previousDateParamStore := "lastModifiedDateIPRanges"

// Set AWS Region
awsRegion := "eu-central-1"
```

### Common errors

##### SSM Param Store
```
# You need a IAM policy with permissions to SSM : 
panic: AccessDeniedException: User: arn:aws:iam::111111111111:user/test is not authorized to perform: ssm:GetParameter on resource: arn:aws:ssm:eu-central-1:111111111111:parameter/lastModifiedDateIPRanges
        status code: 400, request id: 4a0ab454-176d-4bc6-9418-78529da0f944
```

```
# SSM Parameter Store is misspelled or does not exist :
panic: [ERROR]: ParameterNotFound: 
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

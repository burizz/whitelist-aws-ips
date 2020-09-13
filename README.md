# Lambda for whitelisting amazon IP ranges
Lambda for updating Security Group Egress rules with AWS IP Ranges
Pulls latest JSON from - https://docs.aws.amazon.com/general/latest/gr/aws-ip-ranges.html

*This was done just for coding practice in Go, so it's probably not a good idea to use it for anything serious.*

Remaining todo : 
- [ ] Accomodate for SG limit of 60 inbound/outbound rules (maybe counter and them to multiple SGs ?)
- [x] Send array of IP ranges to updateSecurityGroup and update them in SG
- [ ] Convert to lambda function handler
- [ ] Persistent way of storing previousDate var and checking it
- [ ] Update only entries that don't exist already, as it seems AWS handles the already exist part with errors

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
export AWS_REGION=eu-central-1
```

```
# Windows
$env:AWS_ACCESS_KEY_ID='YOUR_AKID'
$env:AWS_SECRET_ACCESS_KEY='YOUR_SECRET_KEY'
$env:AWS_REGION='eu-central-1'
```

### Variables
```
// List of Security groups to be updated
securityGroupIDs := []string{"sg-0f467b0f6743bfc22", "sg-0ec48f26429e25bfe"}

// List of services to be whitelisted - e.g. AMAZON, COUDFRONT, S3, EC2, API_GATEWAY, DYNAMODB, OUTE53_HEALTHCHECKS, CODEBUILD
servicesToBeWhitelist := []string{"S3", "CODEBUILD", "AMAZON"}

// AWS JSON URL and local path to download it
amazonIPRangesURL := "https://ip-ranges.amazonaws.com/ip-ranges.json"
jsonFileLocalPath := "ip-ranges.json"
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
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
- [x] Create initial release v1

**Improvements - v1.1** : 
- [x] Combine download and json parse funcs into one using decoder (no need to download the file locally)
- [x] Add Lambda trigger example in Readme
- [x] Move all vars to be taken from Lambda ENV vars instead of hardcoded
- [x] Handle dependencies as Go modules
- [x] Add CI with Github actions
- [ ] Split functions into separate packages
  - [ ] SSM Param store funcs
  - [ ] Security Group functions
  - [ ] DynamoDB functions
- [ ] Add Unit Tests
- [ ] Create SSM param store if it doesnt exist
- [ ] Move all AWS svc client duplications to an init() function - https://tutorialedge.net/golang/the-go-init-function/; we can have more than 1 init() to initialize the different svc clients
- [ ] Figure out a good way to link all SGs at the end into a single one - some sort of inheritance ?
- [ ] Add IAM policy example with minimal access needed in Readme

**Fix Bugs** :
- [x] Security group updates when IPs are less than 50 (they get duplicated in all SGs)
- [x] Dynamo update items when the table needs to be created - it adds IPs in table but but goes to case where No new IP Ranges were found and doesnt update the SGs, although the Dynamo table is completely empty. putDynamoItem() also doesn't print its success messages, although items are successfully created there. But on next Lambda run eveyrthing is fine because Dynamo table with items already exist. Seems to be related to the regex check, but not sure why it happens only when table doesn't initially excist. We are hitting this condition : case dynamodb.ErrCodeResourceNotFoundException: return true, nil

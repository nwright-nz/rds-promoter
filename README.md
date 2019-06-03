# RDS DB Promoter

This is a proof of concept of programmatically creating AWS Aurora MySQL database clusters / instances, and promoting them through a dev, test and production lifecycle.    
It will also clone a DB in prod back to dev or test for further work. 


## Usage
This uses the AWS Go SDK - your AWS credentials should be in the environment variables `AWS_SECRET_ACCESS_KEY` and `AWS_ACCESS_KEY_ID`.



Edit the config.site file with the name of your DB cluster to create, the db admin user name, and the AWS region you wish to create it in. For example:
```
dbuser: dbadmin
dbname: mydb
awsRegion: ap-southeast-2
```
<b> Note: Obviously you can build a binary from this, and use the binary name instead of the below `go run main.go` examples.</b>

To deploy a DB cluster into Dev, run the following command :   
`go run main.go --env=dev --config=./config.site`

This will progress through database cluster and instance creation, with status. The endpoint of the cluster will de displayed.
```
Creating Aurora Cluster...
Waiting for Aurora cluster creation:   ◳ creating
Waiting for Aurora cluster creation:   ◱ creating
Waiting for Aurora cluster creation:   ◲ creating
Waiting for Aurora cluster creation:   ◰ creating
Waiting for Aurora cluster creation:   ◱ creating
Waiting for Aurora cluster creation:   ◲ available
Created Cluster successfully!
Endpoint is: mydb-dev.cluster-cbrntrlimj6n.ap-southeast-2.rds.amazonaws.com
Creating Database Instance...
Waiting for DB instance creation:   ◳ creating
Waiting for DB instance creation:   ◰ creating
Waiting for DB instance creation:   ◲ creating
Waiting for DB instance creation:   ◳ creating
Waiting for DB instance creation:   ◰ creating
Created Instance successfully!
```

To 'promote' this database to Test, run the following command:   
`go run main.go --env=test --config=./config.site`

This will rename the database with a '-test' suffix.
The output will be similar to the below:   
```
Reading config...
Renaming Aurora Cluster...
Waiting for Aurora cluster rename:   ◱ renaming
Waiting for Aurora cluster rename:   ◳ available
Endpoint is: mydb-test.cluster-cbrntrlimj6n.ap-southeast-2.rds.amazonaws.com
Renamed Cluster successfully!
Renaming Aurora Instance...
Waiting for Aurora instance rename:   ◱ rebooting
Waiting for Aurora instance rename:   ◲ rebooting
Waiting for Aurora instance rename:   ◲ rebooting
Waiting for Aurora instance rename:   ◳ rebooting
Waiting for Aurora instance rename:   ◳ rebooting
Waiting for Aurora instance rename:   ◰ available
Renamed Instance successfully!
```

As per the above example, 'promoting' to Prod involves the following command:   
`go run main.go --env=prod --config=./config.site`

The output of this command will be identical to the above.

If there is an existing Prod database, and the environment specified is dev or test, this will clone the prod DB to a new cluster. 

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"flag"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/rds"
	"github.com/briandowns/spinner"
	"github.com/sethvargo/go-password/password"
)

var (
	config         siteConfig
	sg             = config.sgID
	sgID           = []*string{&sg}
	root           = ""
	sess           session.Session
	environmentPtr = flag.String("env", "dev", "Environment to deploy to (dev, test, prod)")
	configPath     = flag.String("config", "./config.site", "Full path to the config file. E.g. /config/config.site")
)

const (
	letterBytes = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890"
)

type siteConfig struct {
	username     string
	password     string
	dbname       string
	endpoint     string
	snapshotName string
	clustername  string
	domainSuffix string
	sgID         string
	awsRegion    string
}

func main() {
	flag.Parse()

	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		fmt.Println("Config file not found, exiting...")
		os.Exit(1)
	}

	readConfig()

	session := initAWSSession(strings.ToLower(*environmentPtr))

	switch strings.ToLower(*environmentPtr) {
	case "dev":
		genPassword(12)

		if checkForExistingDB(*session, "prod") {
			cloneProdClusterToDev(*session)
			createInstance(*session, config.clustername)
			resetDBPassword(*session, config.clustername+"-dev")
		} else if checkForExistingDB(*session, "test") {
			renameRDS(*session, "dev")
			renameInstance(*session, "dev")
			resetDBPassword(*session, config.clustername+"-dev")
		} else {
			createCluster(*session)
			createInstance(*session, config.clustername)
		}
	case "test":
		if !checkForExistingDB(*session, "test") {
			renameRDS(*session, "test")
			renameInstance(*session, "test")
			modifyInstance(*session)
		} else {
			fmt.Println("Test cluster already exists, no further action required.")
		}
	case "prod":
		if !checkForExistingDB(*session, "prod") {
			renameRDS(*session, "prod")
			renameInstance(*session, "prod")
		}

	}

}

func resetDBPassword(rdsSession rds.RDS, dbClusterName string) {

	dbClusterParams := rds.ModifyDBClusterInput{}
	dbClusterParams.SetMasterUserPassword(config.password)
	dbClusterParams.SetApplyImmediately(true)
	dbClusterParams.SetDBClusterIdentifier(dbClusterName)
	output, err := rdsSession.ModifyDBCluster(&dbClusterParams)
	if err != nil {
		fmt.Println(err)
	}
	status := *output.DBCluster.Status

	describeClusterInput := rds.DescribeDBClustersInput{}
	describeClusterInput.SetDBClusterIdentifier(config.clustername + "-dev")

	s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	fmt.Println("Resetting DB Password...")
	s.Prefix = "Waiting for DB credential rotation:   "

	for {
		s.Start()
		clusterOutput, err := rdsSession.DescribeDBClusters(&describeClusterInput)
		if err != nil {
			fmt.Println(err)
		}

		status = *clusterOutput.DBClusters[0].Status
		fmt.Println(status)

		if status == "available" {
			s.Suffix = "Password reset complete!"
			s.Stop()
			break
		}
		time.Sleep(10 * time.Second)
	}
	fmt.Println("Reset password successfully!")

}
func createCluster(rdsSession rds.RDS) {

	rdsClusterParams := rds.CreateDBClusterInput{}
	rdsClusterParams.SetDatabaseName(config.dbname)
	rdsClusterParams.SetEngine("aurora")
	rdsClusterParams.SetDBClusterIdentifier(config.clustername + "-dev")
	// rdsClusterParams.SetDBSubnetGroupName("wordpress sites")
	rdsClusterParams.SetMasterUsername(config.username)
	rdsClusterParams.SetMasterUserPassword(config.password)
	// rdsClusterParams.SetVpcSecurityGroupIds(sgID)
	cluster, err := rdsSession.CreateDBCluster(&rdsClusterParams)
	if awserr, ok := err.(awserr.Error); ok {
		awsCode := awserr.Code()
		if awsCode == rds.ErrCodeDBClusterAlreadyExistsFault {
			fmt.Println("Cluster already exists, no action required")
			return

		}
	}

	if err != nil {
		fmt.Println(err)
	}
	status := *cluster.DBCluster.Status

	describeClusterInput := rds.DescribeDBClustersInput{}
	describeClusterInput.SetDBClusterIdentifier(config.clustername + "-dev")
	s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	fmt.Println("Creating Aurora Cluster...")
	s.Prefix = "Waiting for Aurora cluster creation:   "

	for {
		s.Start()
		clusterOutput, err := rdsSession.DescribeDBClusters(&describeClusterInput)
		if err != nil {
			fmt.Println(err)
		}
		status = *clusterOutput.DBClusters[0].Status
		fmt.Println(status)

		if status == "available" {
			s.Suffix = "Created Aurora instance successfully!"
			s.Stop()
			break
		}
		time.Sleep(10 * time.Second)
	}
	fmt.Println("Created Cluster successfully!")
	fmt.Println("Endpoint is: " + *cluster.DBCluster.Endpoint)
	config.endpoint = *cluster.DBCluster.Endpoint
	// }

}

func createInstance(rdsSession rds.RDS, clusterName string) {

	rdsInstanceParams := rds.CreateDBInstanceInput{}

	rdsInstanceParams.SetDBClusterIdentifier(clusterName + "-dev")
	rdsInstanceParams.SetDBInstanceIdentifier(clusterName + "-dev")

	rdsInstanceParams.SetDBInstanceClass("db.t2.small")
	rdsInstanceParams.SetMultiAZ(false)
	rdsInstanceParams.SetAutoMinorVersionUpgrade(true)
	if strings.ToLower(*environmentPtr) == "dev" {
		rdsInstanceParams.SetPubliclyAccessible(true)
	} else {
		rdsInstanceParams.SetPubliclyAccessible(false)
	}

	rdsInstanceParams.SetEngine("aurora")
	instance, err := rdsSession.CreateDBInstance(&rdsInstanceParams)
	if awserr, ok := err.(awserr.Error); ok {
		awsCode := awserr.Code()
		if awsCode == rds.ErrCodeDBInstanceAlreadyExistsFault {
			fmt.Println("Instance already exists, no action required")
			return

		}
	}
	if err != nil {
		fmt.Println(err)
	}
	status := *instance.DBInstance.DBInstanceStatus
	fmt.Println(status)
	describeInstanceInput := rds.DescribeDBInstancesInput{}
	describeInstanceInput.SetDBInstanceIdentifier(clusterName + "-dev")

	s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	fmt.Println("Creating Database Instance...")
	s.Prefix = "Waiting for DB instance creation:   "

	for {
		s.Start()
		instanceOutput, err := rdsSession.DescribeDBInstances(&describeInstanceInput)
		if err != nil {
			fmt.Println(err)
		}

		status = *instanceOutput.DBInstances[0].DBInstanceStatus
		fmt.Println(status)

		if status == "available" {
			s.Suffix = "Created Aurora instance successfully!"
			s.Stop()
			break
		}
		time.Sleep(10 * time.Second)
	}
	fmt.Println("Created Instance successfully!")

}

func genPassword(n int) {

	res, err := password.Generate(21, 5, 0, false, false)
	if err != nil {
		log.Fatal(err)
	}

	config.password = res

}

func readConfig() {
	fmt.Println("Reading config...")

	inFile, err := os.Open(*configPath)
	if err != nil {
		fmt.Println(err)
	}
	scanner := bufio.NewScanner(inFile)
	scanner.Split(bufio.ScanLines)

	for scanner.Scan() {
		stringSlice := strings.Split(scanner.Text(), ":")
		switch stringSlice[0] {
		case "dbuser":
			config.username = strings.TrimSpace(stringSlice[1])
		case "dbname":
			config.dbname = strings.TrimSpace(stringSlice[1])
			config.clustername = strings.TrimSpace(stringSlice[1])
		case "awsRegion":
			config.awsRegion = strings.TrimSpace(stringSlice[1])
		}

	}
}

func modifyInstance(rdsSession rds.RDS) {

	dbClusterParams := rds.ModifyDBClusterInput{}
	// dbClusterParams.SetVpcSecurityGroupIds(sgID)
	clusterToModify := config.clustername + "-test"
	instanceToModify := clusterToModify

	dbClusterParams.SetDBClusterIdentifier(clusterToModify)
	modifiedCluster, err := rdsSession.ModifyDBCluster(&dbClusterParams)
	if err != nil {
		fmt.Print(err)
	}
	fmt.Println(modifiedCluster.DBCluster.Status)

	dbInstanceParams := rds.ModifyDBInstanceInput{}
	public := false
	dbInstanceParams.PubliclyAccessible = &public
	dbInstanceParams.SetDBInstanceIdentifier(instanceToModify)
	dbInstanceParams.SetApplyImmediately(true)
	modifiedInstance, err := rdsSession.ModifyDBInstance(&dbInstanceParams)
	if err != nil {
		fmt.Println(err)
	}
	status := *modifiedInstance.DBInstance.DBInstanceStatus
	describeInstanceInput := rds.DescribeDBInstancesInput{}
	describeInstanceInput.SetDBInstanceIdentifier(instanceToModify)

	s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	fmt.Println("Making DB Instance Private...")
	s.Prefix = "Waiting for DB modification:   "

	for {
		s.Start()
		instanceOutput, err := rdsSession.DescribeDBInstances(&describeInstanceInput)
		if err != nil {
			fmt.Println(err)
		}

		status = *instanceOutput.DBInstances[0].DBInstanceStatus
		fmt.Println(status)

		if status == "available" {
			s.Suffix = "Modified Aurora instance successfully!"
			s.Stop()
			break
		}
		time.Sleep(10 * time.Second)
	}
	fmt.Println("Modified Instance successfully!")

}

func checkForExistingDB(rdsSession rds.RDS, environmentName string) bool {
	describeClusterInput := rds.DescribeDBClustersInput{}
	if environmentName == "dev" {
		describeClusterInput.SetDBClusterIdentifier(config.clustername + "-dev")
	}
	if environmentName == "test" {
		describeClusterInput.SetDBClusterIdentifier(config.clustername + "-test")
	}
	if environmentName == "prod" {
		describeClusterInput.SetDBClusterIdentifier(config.clustername)
	}

	clusterOutput, err := rdsSession.DescribeDBClusters(&describeClusterInput)
	if err != nil {
		if awserr, ok := err.(awserr.Error); ok {
			awsCode := awserr.Code()
			if awsCode == rds.ErrCodeDBClusterNotFoundFault {
				return false

			}

		}
	}
	config.endpoint = *clusterOutput.DBClusters[0].Endpoint
	return true

}

func renameInstance(rdsSession rds.RDS, environment string) {
	// fmt.Println("In rename function, environment is : " + environment)
	describeInstanceInput := rds.DescribeDBInstancesInput{}
	dbInstanceParams := rds.ModifyDBInstanceInput{}
	switch env := environment; env {
	case "dev":
		dbInstanceParams.SetDBInstanceIdentifier(config.clustername + "-test")
		dbInstanceParams.SetNewDBInstanceIdentifier(config.clustername + "-dev")
		dbInstanceParams.SetPubliclyAccessible(true)

		describeInstanceInput.SetDBInstanceIdentifier(config.clustername + "-dev")

	case "test":
		dbInstanceParams.SetDBInstanceIdentifier(config.clustername + "-dev")
		dbInstanceParams.SetNewDBInstanceIdentifier(config.clustername + "-test")
		describeInstanceInput.SetDBInstanceIdentifier(config.clustername + "-test")

	case "prod":
		dbInstanceParams.SetDBInstanceIdentifier(config.clustername + "-test")
		describeInstanceInput.SetDBInstanceIdentifier(config.clustername)
		dbInstanceParams.SetNewDBInstanceIdentifier(config.clustername)

	}

	dbInstanceParams.SetApplyImmediately(true)

	modifiedInstance, err := rdsSession.ModifyDBInstance(&dbInstanceParams)
	if err != nil {
		fmt.Println(err)
	}
	status := *modifiedInstance.DBInstance.DBInstanceStatus

	s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	fmt.Println("Renaming Aurora Instance...")
	s.Prefix = "Waiting for Aurora instance rename:   "
	s.Start()
	time.Sleep(40 * time.Second)

	for {

		instanceOutput, err := rdsSession.DescribeDBInstances(&describeInstanceInput)

		if err != nil {
			if awsErr, ok := err.(awserr.Error); ok {
				fmt.Println(awsErr.Code())
				fmt.Println(awsErr.Message())
				fmt.Println("Still waiting for the rename to happen...")
				time.Sleep(2 * time.Second)
				continue
				// process SDK error
			}
		}

		status = *instanceOutput.DBInstances[0].DBInstanceStatus
		fmt.Println(status)

		if status == "available" {
			s.Suffix = "Renamed Aurora instance successfully!"
			s.Stop()
			break
		}
		time.Sleep(10 * time.Second)
	}
	fmt.Println("Renamed Instance successfully!")

}
func renameRDS(rdsSession rds.RDS, environment string) {
	describeClusterInput := rds.DescribeDBClustersInput{}

	if !checkForExistingDB(rdsSession, environment) {

		dbClusterParams := rds.ModifyDBClusterInput{}
		switch env := environment; env {
		case "test":
			dbClusterParams.SetDBClusterIdentifier(config.clustername + "-dev")
			dbClusterParams.SetNewDBClusterIdentifier(config.clustername + "-test")
			describeClusterInput.SetDBClusterIdentifier(config.clustername + "-test")

		case "prod":
			dbClusterParams.SetDBClusterIdentifier(config.clustername + "-test")
			describeClusterInput.SetDBClusterIdentifier(config.clustername)
			dbClusterParams.SetNewDBClusterIdentifier(config.clustername)

		case "dev":
			dbClusterParams.SetDBClusterIdentifier(config.clustername + "-test")
			dbClusterParams.SetNewDBClusterIdentifier(config.clustername + "-dev")
			describeClusterInput.SetDBClusterIdentifier(config.clustername + "-dev")

		}

		dbClusterParams.SetApplyImmediately(true)
		modifiedCluster, err := rdsSession.ModifyDBCluster(&dbClusterParams)
		if err != nil {
			fmt.Println(err)
		}
		status := *modifiedCluster.DBCluster.Status
		s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
		fmt.Println("Renaming Aurora Cluster...")
		s.Prefix = "Waiting for Aurora cluster rename:   "
		s.Start()
		time.Sleep(40 * time.Second)

		for {
			clusterOutput, err := rdsSession.DescribeDBClusters(&describeClusterInput)
			if err != nil {
				if awsErr, ok := err.(awserr.Error); ok {
					fmt.Println(awsErr.Code())
					fmt.Println(awsErr.Message())
					fmt.Println("Still waiting for the rename to happen...")
					time.Sleep(2 * time.Second)
					continue
				}
			}
			status = *clusterOutput.DBClusters[0].Status
			fmt.Println(status)

			if status == "available" {
				s.Suffix = "Renamed Aurora cluster successfully!"
				fmt.Println("Endpoint is: " + *clusterOutput.DBClusters[0].Endpoint)
				config.endpoint = *clusterOutput.DBClusters[0].Endpoint
				s.Stop()
				break
			}
			time.Sleep(10 * time.Second)
		}
	} else {
		fmt.Println("DB already exists, no action required")
	}
	fmt.Println("Renames Cluster successfully!")
}

func initAWSSession(environment string) *rds.RDS {
	sess, err := session.NewSession(&aws.Config{Region: aws.String(config.awsRegion)})
	if err != nil {
		fmt.Println(err)
	}
	return rds.New(sess)

}

func cloneProdClusterToDev(rdsSession rds.RDS) {
	describeCloneInput := rds.RestoreDBClusterToPointInTimeInput{}
	describeCloneInput.SetDBClusterIdentifier(config.clustername + "-dev")

	describeCloneInput.SetSourceDBClusterIdentifier(config.clustername)
	describeCloneInput.SetUseLatestRestorableTime(true)
	describeCloneInput.SetRestoreType("copy-on-write")

	s := spinner.New(spinner.CharSets[5], 100*time.Millisecond)
	cloneOutput, err := rdsSession.RestoreDBClusterToPointInTime(&describeCloneInput)
	if err != nil {
		fmt.Println(err)
	}
	status := *cloneOutput.DBCluster.Status
	describeDBCluster := rds.DescribeDBClustersInput{}
	describeDBCluster.SetDBClusterIdentifier(config.clustername + "-dev")

	s.Prefix = "Waiting for Aurora clone:   "

	for {
		s.Start()
		clusterOutput, err := rdsSession.DescribeDBClusters(&describeDBCluster)
		if err != nil {
			fmt.Println(err)
		}
		status = *clusterOutput.DBClusters[0].Status
		fmt.Println(status)

		if status == "available" {
			s.Suffix = "Cloned production database successfully!"
			s.Stop()
			break
		}
		time.Sleep(10 * time.Second)
	}

	fmt.Print("cloned production to dev successfully!")

	config.endpoint = *cloneOutput.DBCluster.Endpoint

}

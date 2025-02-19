# Timestream Prometheus Connector with AWS PrivateLink

## Overview

This guide explains how to set up the Prometheus Connector to ingest data to Amazon Timestream from within an isolated VPC environment using [AWS PrivateLink](https://aws.amazon.com/privatelink/).

This [serverless application](https://aws.amazon.com/serverless/) consists of the following:
- [Amazon EC2](https://aws.amazon.com/ec2/getting-started/) instance that will host the Prometheus Connector.
- [VPC Endpoints](https://docs.aws.amazon.com/whitepapers/latest/aws-privatelink/what-are-vpc-endpoints.html) for securely communicating with AWS services using PrivateLink.

This application assumes that the VPC in which the template will be deployed has no internet access and ensures that all communication stays within Amazon's internal network. 

## Prerequisites

1. A VPC with at least two private subnets and route tables.
2. A Timestream database and table.
3. [Read and write cells](https://docs.aws.amazon.com/timestream/latest/developerguide/architecture.html#cells) for your Timestream account. Amazon routes requests to the write and query endpoints of the cell that your account has been mapped to for a given region.

To get your assigned cells using `awscli`:

For read endpoint:
```
aws timestream-query describe-endpoints --region <AWS_REGION>
```

For write endpoint:
```
aws timestream-write describe-endpoints --region <AWS_REGION>
```

Example output for the write endpoint:
```
{
    "Endpoints": [
        {
            "Address": "ingest-cell1.timestream.us-west-2.amazonaws.com",
            "CachePeriodInMinutes": 1440
        }
    ]
}
```
Take note of your assigned cells (`ingest-cell1` for the above example) for both read and write endpoints.


## Deployment

From your existing VPC, you will need the following values:
- VPC ID: This is the ID of your existing VPC
- VPC CIDR : This is the CIDR range for your VPC
- Private Subnet IDs: This is where the EC2 instance and VPC endpoints will be deployed
- Private Route Table ID(s): This is how the [S3 Gateway endpoint](https://docs.aws.amazon.com/vpc/latest/privatelink/vpc-endpoints-s3.html) will resolve requests
- Query and Write cells: These are your assigned endpoint cells for Timestream


1. From the `privatelink` directory, run the following command to deploy the SAM template:

```
sam deploy --parameter-overrides "VpcId=<VPC_ID> VpcCidrIp=<VPC_CIDR_IP> PrivateSubnetIds=<PRIVATE_SUBNET_ID_1>,<PRIVATE_SUBNET_ID_2> PrivateRouteTableIds=<PRIVATE_ROUTE_TABLE_ID> TimestreamQueryCell=<QUERY_CELL> TimestreamWriteCell=<WRITE_CELL> --region <AWS_REGION>"
```

To view the full set of `sam deploy` options see the [sam deploy documentation](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-deploy.html).

2. The deployment will have the following outputs upon completion:

- `InstanceId`: ID of the EC2 instance

   An example of the output:

```
------------------------------------------------------------------------------
Outputs                                                                                                                                           
------------------------------------------------------------------------------
Key                 InstanceId                                                                                                                    
Description         ID of the EC2 instance                                                                                                        
Value               i-08a5d7e1700c9be5a
------------------------------------------------------------------------------
```

3. Start an AWS SSM session, replacing `INSTANCE_ID` with your EC2 instance ID from deployment. You can install the [plugin here.](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html)

```shell
aws ssm start-session --target i-<INSTANCE_ID>
``` 

4. Install the Prometheus Connector.

    1. Create a directory for the connector.
    ```
    mkdir ~/connector && cd ~/connector
    ```
    
    2. Download the precompiled binary from S3 for your region. [See here](https://github.com/awslabs/amazon-timestream-connector-prometheus/tags) for released versions.

    ```shell
    curl -O https://timestreamassets-<AWS_REGION>.s3.<AWS_REGION>.amazonaws.com/timestream-prometheus-connector/timestream-prometheus-connector-linux-arm64-<VERSION>.zip
    ``` 
    
    3. Unzip the binary.

    ```shell
    unzip timestream-prometheus-connector-linux-arm64-<VERSION>.zip
    ```
    
    4. Disable endpoint discovery by setting the `AWS_ENABLE_ENDPOINT_DISCOVERY` environment variable to `false`. This ensures requests from the connector are routed through VPC endpoints.
    ```
    export AWS_ENABLE_ENDPOINT_DISCOVERY=false
    ```

5. Launch Prometheus Connector

Replace the following variables to configure your Timestream database, region, and assigned cells.


- `DEFAULT_DATABASE`: Specifies the default Timestream database for the Prometheus Connector.
- `DEFAULT_TABLE`: Specifies the default table for storing Prometheus metrics.
- `AWS_REGION`: Defines the AWS region.
- `QUERY_CELL`: Defines the query endpoint cell for Timestream.
- `INGEST_CELL`: Defines the ingestion endpoint cell for Timestream.

Run the Prometheus Connector:

```
./bootstrap \
   --default-database=<DEFAULT_DATABASE> \
   --default-table=<DEFAULT_TABLE> \
   --region=<AWS_REGION> \
   --query-base-endpoint=https://<QUERY_CELL>.timestream.<AWS_REGION>.amazonaws.com \
   --write-base-endpoint=https://<INGEST_CELL>.timestream.<AWS_REGION>.amazonaws.com
```

The connector is now ready to ingest data to Timestream!

To see an example of how Prometheus can be configured, [see here](https://github.com/awslabs/amazon-timestream-connector-prometheus?tab=readme-ov-file#prometheus-configuration).

### Cleanup

Delete the CloudFormation stack. From the `privatelink` directory, run the following command:

```shell
sam delete --region <AWS_REGION>
```

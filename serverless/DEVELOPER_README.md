# Timestream Prometheus Connector

## Overview

This [serverless application](https://aws.amazon.com/serverless/) consists of the following:
- [Amazon API Gateway](https://docs.aws.amazon.com/apigateway/latest/developerguide/welcome.html) that listens for [Prometheus remote read and write](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) requests.
- [AWS Lambda function](https://docs.aws.amazon.com/lambda/latest/dg/welcome.html) that stores the received Prometheus metrics in [Amazon Timestream](https://docs.aws.amazon.com/timestream/latest/developerguide/what-is-timestream.html).

This application is meant to be used as a getting started guide and does not configure TLS encryption between Prometheus and the API Gateway. This is not recommended to be used directly for production.
To enable TLS encryption for production, see [Configuring mutual TLS authentication for an HTTP API](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-mutual-tls.html).

### Prerequisites

1. [Create a Timestream database](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.db.using-console).

   ```shell
   aws timestream-write create-database --database-name <PrometheusDatabase>
   ```

2. [Create a Timestream table](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.table.using-console).

   ```shell
   aws timestream-write create-table --database-name <PrometheusDatabase> --table-name <PrometheusMetricsTable>
   ```

3. Download [Prometheus](https://prometheus.io/download) (or reuse your existing Prometheus instance).

   > **NOTE**: The user deploying this application must have administrative privileges due to the number of permissions required for deployment. For a detailed list of permissions see [Required Permissions](#required-permissions).

## Getting Started

To start using the Prometheus remote storage connector for Timestream, there are multiple steps involved:

1. **Deployment** &mdash; deploy the endpoints through Amazon API Gateway and deploy the Prometheus Connector on AWS Lambda by using either [one-click deployment](#one-click-deployment) or [AWS CLI](#aws-cli-deployment).
2. **Configure Prometheus** &mdash; configure the remote storage endpoints for Prometheus.
3. **Invoke AWS Lambda Function** &mdash; update the permissions for the users.

## Deployment

### One-click Deployment

Use an [AWS CloudFormation](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/Welcome.html) [template](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-whatis-concepts.html#cfn-concepts-templates) to create the [stack](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-whatis-concepts.html#cfn-concepts-stacks):

To install the Timestream Prometheus Connector service launch the AWS CloudFormation stack on the AWS CloudFormation console by choosing one of the "Launch Stack" buttons in the following table:

| Region                    | View                                                                                                                                      | View in Designer                                                                                                                                                                                                                                         | Launch                                                                                                                                                                                                                                                                            |
|---------------------------|-------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| US East (N. Virginia) us-east-1 | [View](https://timestreamassets-us-east-1.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-east-1&templateURL=https://timestreamassets-us-east-1.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-east-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-us-east-1.s3.amazonaws.com/template.yml)         |
| US East (Ohio) us-east-2 | [View](https://timestreamassets-us-east-2.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-east-2&templateURL=https://timestreamassets-us-east-2.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-east-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-us-east-2.s3.amazonaws.com/template.yml)         |
| US West (Oregon) us-west-2 | [View](https://timestreamassets-us-west-2.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-west-2&templateURL=https://timestreamassets-us-west-2.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-west-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-us-west-2.s3.amazonaws.com/template.yml)         |
| Asia Pacific (Sydney) ap-southeast-2 | [View](https://timestreamassets-ap-southeast-2.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-southeast-2&templateURL=https://timestreamassets-ap-southeast-2.s3.amazonaws.com/template.yml)      | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-southeast-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-ap-southeast-2.s3.amazonaws.com/template.yml)    |
| Asia Pacific (Tokyo) ap-northeast-1 | [View](https://timestreamassets-ap-northeast-1.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-northeast-1&templateURL=https://timestreamassets-ap-northeast-1.s3.amazonaws.com/template.yml)      | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-northeast-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-ap-northeast-1.s3.amazonaws.com/template.yml)    |
| Europe (Frankfurt) eu-central-1 | [View](https://timestreamassets-eu-central-1.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-central-1&templateURL=https://timestreamassets-eu-central-1.s3.amazonaws.com/template.yml)        | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-central-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-eu-central-1.s3.amazonaws.com/template.yml)      |
| Europe (Ireland) eu-west-1 | [View](https://timestreamassets-eu-west-1.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-west-1&templateURL=https://timestreamassets-eu-west-1.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-west-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://timestreamassets-eu-west-1.s3.amazonaws.com/template.yml)         |

> **Note**: Attempting to use one of the above "Launch" links to create an already existing stack will fail. To update an existing stack, such as the default `PrometheusTimestreamConnector` stack, via the AWS Console, go to the stacks page at `https://<region>.console.aws.amazon.com/cloudformation/home`, select the stack you want to update from the list, then click "Update" to proceed through the update process.

### AWS CLI Deployment

The steps to deploy a template are as follows:

1. Download the latest linux amd64 release `.zip` archive from the [Releases page](https://github.com/Bit-Quill/timestream-prometheus-connector/releases) and place in the `serverless` directory.

2. From the the `serverless` directory, run the following command to deploy the template:

   ```shell
   sam deploy
   ```
   To run this command from a different directory add the `-t <path to serverless/template.yml>` argument to specify the template.

   In addition to creating a new stack with the default name `PrometheusTimestreamConnector` this command creates a default stack called `aws-sam-cli-managed-default`. The default stack manages an [S3](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html) [bucket](https://docs.aws.amazon.com/AmazonS3/latest/userguide/Welcome.html#BasicsBucket) hosting all the deployment artifacts:


   - the [SAM](https://aws.amazon.com/serverless/sam/) template;
   - the precompiled connector binary for Linux.

   To use a specific S3 bucket run:

    ```shell
    sam deploy --s3-bucket <custom-bucket>
    ```
    To override default parameter values use the `--parameter-overrides` argument and provide a string with format ParameterKey=ParameterValue. For example:

    ```shell
    sam deploy --parameter-overrides "TimeoutInMillis=60000 DefaultDatabase=<CustomDatabase>"
    ```
    You can view the full set of parameters defined for `serverless/template.yml` below, in [AWS Lambda Configuration Options](#aws-lambda-configuration-options).

    To override default values for parameters and interactively proceed through stack deployment run:

    ```shell
    sam deploy --guided
    ```

  To deploy to a specific region:

  TBD - Lambda deployment needs region set on creation.

To view the full set of `sam deploy` options see the [sam deploy documentation](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-deploy.html).

3. The deployment will have the following outputs upon completion:

   1. InvokeReadURL: The remote read URL for Prometheus.
   2. InvokeWriteURL: The remote write URL for Prometheus.
   3. DefaultDatabase: The database destination for queries and ingestion.
   4. DefaultTable: The database table destination for queries and ingestion.

   An example of the output:

    ```
    CloudFormation outputs from deployed stack
    ------------------------------------------------------------------------------------------------------------
    Outputs                                                                                                                                                                                                                             
    ------------------------------------------------------------------------------------------------------------
    Key                 InvokeReadURL                                                                                                                                                                                                   
    Description         Remote read URL for Prometheus                                                                                                                                                                                  
    Value               https://api-id.execute-api.region.amazonaws.com/prod/read                                                                                                                                                                                                                                                                                      
    
    Key                 InvokeWriteURL                                                                                                                                                                                                  
    Description         Remote write URL for Prometheus                                                                                                                                                                                 
    Value               https://api-id.execute-api.region.amazonaws.com/prod/write                                                                                                                                                                                                                                                                                               
    
    Key                 DefaultDatabase                                                                                                                                                                                                  
    Description         The Prometheus label containing the database name                                                                                                                                                                                 
    Value               PrometheusDatabase                                                                                                                                                                                                                                                                                               
    
    Key                 DefaultTable                                                                                                                                                                                                  
    Description         The Prometheus label containing the table name                                                                                                                                                                            
    Value               PrometheusMetricsTable                                                                                                                                               
    ------------------------------------------------------------------------------------------------------------
    ```

   To view all the stack information open the for more details see [Viewing AWS CloudFormation stack data and resources on the AWS Management Console](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-console-view-stack-data-resources.html).

## Configuration

### Configure Prometheus

To let the Lambda function know which database/table destination is for a Prometheus time series,
two additional labels need to be added in every Prometheus time series through [Prometheus relabel config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config).

1. Open the configuration file for Prometheus, the file is usually named `prometheus.yml`.

2. Replace the `InvokeWriteURL` and `InvokeReadURL` with the API Gateway URLs from deployment, and provide the appropriate IAM credentials in `basic_auth` before adding the following sections to the configuration file:

   > **NOTE**: All configuration options are *case-sensitive*, and *session_token* authentication parameter is not supported for MFA authenticated AWS users.

   ```yaml
   scrape_configs:
     - job_name: 'prometheus'
       scrape_interval:    15s

       static_configs:
         - targets: ['localhost:9090']

   remote_write:
   # Update the value to the InvokeWriteURL returned when deploying the stack.
   - url: "InvokeWriteURL"
     queue_config:
       max_samples_per_send: 100

     # Update the username and password to a valid IAM access key and secret access key.
     basic_auth:
         username: accessKey
         password_file: passwordFile

   remote_read:
   # Update the value to the InvokeReadURL returned when deploying the stack.
   - url: "InvokeReadURL"

     # Update the username and password to a valid IAM access key and secret access key.
     basic_auth:
         username: accessKey
         password_file: passwordFile
   ```

   The *password_file* path must be the absolute path for the file, and the password file must contain only the value for the *aws_secret_access_key*.

   The *url* values for *remote_read* and *remote_write* will be outputs from the cloudformation deployment. See the following example for a remote write url:

   ```yaml
   url: "https://foo9l30.execute-api.us-east-1.amazonaws.com/dev/write"
   ```

### Start Prometheus

1. Ensure the user invoking the AWS Lambda function has read and write permissions to Amazon Timestream. For more details see [Execution Permissions](#execution-permissions).
2. Start Prometheus. Since the remote storage options for Prometheus has been configured, Prometheus will start ingesting to Timestream through the API Gateway endpoints.

### Verification

Follow the verification steps in [README.md#verification](../README.md#verification).

### AWS Lambda Configuration Options

| Option                    | Description                                                  | Default Value                   |
| ------------------------- | ------------------------------------------------------------ | ------------------------------- |
| DefaultDatabase   		| The Prometheus label containing the database name.           | PrometheusDatabase                 |
| DefaultTable      		| The Prometheus label containing the table name.              | PrometheusMetricsTable                    |
| MemorySize                | The memory size of the AWS Lambda function.                  | 512                             |
| TimeoutInMillis           | The amount of time in milliseconds to run the connector on AWS Lambda before timing out. | 30000                           |
| ReadThrottlingBurstLimit  | The number of burst read requests per second that API Gateway permits. | 1200                             |
| WriteThrottlingBurstLimit | The number of burst write requests per second that API Gateway permits. | 1200                             |

### IAM Permissions Configuration Options

| Option               | Description                                                  | Default Value                  |
| -------------------- | ------------------------------------------------------------ | ------------------------------ |
| ExecutionPolicyName  | The name of the execution policy created for AWS Lambda. | LambdaExecutionPolicy  |

### Amazon API Gateway Configuration Options

| Option              | Description                        | Default Value |
| ------------------- | ---------------------------------- | ------------- |
| APIGatewayStageName | The stage name of the API Gateway. Stage names can contain only alphanumeric characters, hyphens, and underscores. | dev          |

The default stage name `dev` may indicate the endpoint is at `development` stage.
If the application is ready for production, set the stage name to a more appropriate value like `prod` when deploying the stack.

## Required Permissions

The template assumes the user deploying the project has administrative permissions. If the user is missing any of the required permissions the deployment will fail.

See [Troubleshooting](#troubleshooting) section for more details.

### Deployment Permissions

The user **deploying** this project **must** have the following permissions listed below. Ensure the values of `account-id` and `region` in the resources section are updated before using this template directly.

> **Note** - All permissions have limited resources except actions that cannot be limited to a specific resource. APIGateway actions cannot limit resources as the resource name is auto generated by the template. See the following documentation for cloudformation, sns, and iam limitations on actions:
[cloudformation](https://docs.aws.amazon.com/service-authorization/latest/reference/list_awscloudformation.html#awscloudformation-actions-as-permissions)
[sns](https://docs.aws.amazon.com/service-authorization/latest/reference/list_amazonsns.html#amazonsns-actions-as-permissions)
[iam](https://docs.aws.amazon.com/service-authorization/latest/reference/list_awsidentityandaccessmanagementiam.html#awsidentityandaccessmanagementiam-actions-as-permissions)

> **NOTE** - This policy is too long to be added inline during user creation, and must be created as a policy and attached to the user instead.

```json
{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Sid": "VisualEditor0",
			"Effect": "Allow",
			"Action": [
				"cloudformation:ListStacks",
				"cloudformation:GetTemplateSummary",
				"iam:ListRoles",
				"sns:ListTopics",
				"apigateway:GET",
				"apigateway:POST",
				"apigateway:PUT",
				"apigateway:TagResource"
			],
			"Resource": "*"
		},
		{
			"Sid": "VisualEditor1",
			"Effect": "Allow",
			"Action": [
				"iam:GetRole",
				"iam:CreateRole",
				"iam:AttachRolePolicy",
				"iam:PutRolePolicy",
				"iam:CreatePolicy",
				"iam:PassRole",
				"iam:GetRolePolicy"
			],
			"Resource": "arn:aws:iam::<account-id>:role/PrometheusTimestreamConnector-IAMLambdaRole-*"
		},
		{
			"Sid": "VisualEditor2",
			"Effect": "Allow",
			"Action": [
				"cloudformation:CreateChangeSet",
				"cloudformation:DescribeStacks",
				"cloudformation:DescribeStackEvents",
				"cloudformation:DescribeChangeSet",
				"cloudformation:ExecuteChangeSet",
				"cloudformation:GetTemplate",
				"cloudformation:CreateStack",
				"cloudformation:GetStackPolicy"
			],
			"Resource": [
				"arn:aws:cloudformation:<region>:<account-id>:stack/PrometheusTimestreamConnector/*",
				"arn:aws:cloudformation:<region>:<account-id>:stack/aws-sam-cli-managed-default/*",
				"arn:aws:cloudformation:<region>:aws:transform/Serverless-2016-10-31"
			]
		},
		{
			"Sid": "VisualEditor3",
			"Effect": "Allow",
			"Action": [
				"lambda:ListFunctions",
				"lambda:AddPermission",
				"lambda:CreateFunction",
				"lambda:TagResource",
				"lambda:GetFunction"
			],
			"Resource": "arn:aws:lambda:<region>:<account-id>:function:PrometheusTimestreamConnector-LambdaFunction-*"
		},
		{
			"Sid": "VisualEditor4",
			"Effect": "Allow",
			"Action": [
				"s3:GetObject",
				"s3:GetBucketPolicy",
				"s3:GetBucketLocation"
			],
			"Resource": "arn:aws:s3:::timestreamassets-<region>/timestream-prometheus-connector-linux-amd64-1.1.0.zip"
		},
		{
			"Sid": "VisualEditor5",
			"Effect": "Allow",
			"Action": [
				"s3:GetObject",
				"s3:GetBucketPolicy",
				"s3:GetBucketLocation",
				"s3:PutObject",
				"s3:PutBucketPolicy",
				"s3:PutBucketTagging",
				"s3:PutEncryptionConfiguration",
				"s3:PutBucketVersioning",
				"s3:PutBucketPublicAccessBlock",
				"s3:CreateBucket",
				"s3:DescribeJob",
				"s3:ListAllMyBuckets"
			],
			"Resource": "arn:aws:s3:::aws-sam-cli-managed-default*"
		},
		{
			"Sid": "VisualEditor6",
			"Effect": "Allow",
			"Action": [
				"cloudformation:GetTemplateSummary"
			],
			"Resource": "*",
			"Condition": {
				"StringEquals": {
					"cloudformation:TemplateUrl": [
						"https://timestreamassets-<region>.s3.amazonaws.com/template.yml"
					]
				}
			}
		}
	]
}
```

### Execution Permissions

The user **executing** this project **must** have the following permissions listed below. Ensure the values of `account-id` and `region` in the resource section are updated before using this template directly. If the name of the database and table differ from the policy resource, be sure to update their values.

> **Note** - Timestream:DescribeEndpoints resource must be `*` as specified under [security_iam_service-with-iam](https://docs.aws.amazon.com/timestream/latest/developerguide/security_iam_service-with-iam.html).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "timestream:WriteRecords",
        "timestream:Select"
      ],
      "Resource": "arn:aws:timestream:<region>:<account-id>:database/PrometheusDatabase/table/PrometheusMetricsTable"
    },
    {
      "Effect": "Allow",
      "Action": [
        "timestream:DescribeEndpoints"
      ],
      "Resource": "*"
    }
  ]
}
```

### Create Deployment and Execution Policies

#### Create Deployment Policy

1. Open the [AWS management console](https://console.aws.amazon.com/iam/) for AWS IAM.
2. Click `Policies`.
3. Click `Create policy`.
4. Click `JSON`.
5. Remove default policy and paste the Deployment policy into the Policy Editor.
6. Update values for `<account-id>` and `<region>` for your AWS account.
7. Click `Next`.
8. Enter `TimestreamPrometheusDeploymentPolicy` in the `Policy name` dialogue box.
9. Click `Create policy`.

#### Create Execution Policy

1. Open the [AWS management console](https://console.aws.amazon.com/iam/) for AWS IAM.
2. Click `Policies`.
3. Click `Create policy`.
4. Click `JSON`.
5. Remove default policy and paste the Execution policy into the Policy Editor.
6. Update values for `<account-id>` and `<region>` for your AWS account.
7. Click `Next`.
8. Enter `TimestreamPrometheusExecutionPolicy` in the `Policy name` dialogue box.
9. Click `Create policy`.

### Create and Configure Users

#### Create Deployment User

1. Open the [AWS management console](https://console.aws.amazon.com/iam) for AWS IAM.
2. Click `Users`.
3. Click `Create User`.
4. Enter `TimestreamPrometheusDeployment` in the `User name` dialogue box.
5. Click `Next`.
6. Click `Attach policies directly`.
7. Search for the policy `TimestreamPrometheusDeploymentPolicy` and select the box next to the policy.
8. Click `Next`.
9. Click `Create user`.

#### Configure Deployment User Credentials

> **Note**: This portion is only needed if the deploying method for the Prometheus Connector is using one-click deployment.

1. Open the [AWS management console](https://console.aws.amazon.com/iam) for AWS IAM.
2. Click `Users`.
3. Search for `TimestreamPrometheusDeployment` and select the user.
4. Click `Security credentials`.
6. Click `Enable console access`.
7. Click `Enable` and `Apply`.
8. Save the password to login the user when deploying using the one-click deployment method.

> **Note**: This portion is only needed if the deploying method for the Prometheus Connector is using the AWS SAM CLI.

1. Open the [AWS management console](https://console.aws.amazon.com/iam) for AWS IAM.
2. Click `Users`.
3. Search for `TimestreamPrometheusDeployment` and select the user.
4. Click `Create access key` in the Summary box.
6. Click `Application running outside AWS`.
7. Click `Next`.
8. Click `Create access key`.

Store the `Access key` and `Secret access key` in your `~/.aws/credentials` file with the following format:

```
[default]
aws_access_key_id = <access key>
aws_secret_access_key = <Secret Access Key>
```

#### Create Execution User

1. Open the [AWS management console](https://console.aws.amazon.com/iam) for AWS IAM.
2. Click `Users`.
3. Click `Create User`.
4. Enter `TimestreamPrometheusExecution` in the `User name` dialogue box.
5. Click `Next`.
6. Click `Attach policies directly`.
7. Search for the policy `TimestreamPrometheusExecutionPolicy` and select the box next to the policy.
8. Click `Next`.
9. Click `Create user`.

#### Configure Execution User Credentials

1. Open the [AWS management console](https://console.aws.amazon.com/iam) for AWS IAM.
2. Click `Users`.
3. Search for `TimestreamPrometheusExecution` and select the user.
4. Click `Create access key` in the Summary box.
6. Click `Application running outside AWS`.
7. Click `Next`.
8. Click `Create access key`.

Store the `Access key` and `Secret access key` for later to configure Prometheus for execution.

## Template IAM Permissions

Running the Prometheus Connector on AWS Lambda allows for a serverless workflow. This section details the [IAM](https://docs.aws.amazon.com/IAM/latest/UserGuide/introduction.html) permissions created by the template to integrate the Prometheus Connector with Amazon API Gateway and AWS Lambda.

### Execution Policy

The `LambdaExecutionPolicy` created by the template allows the lambda function to output logs to cloudwatch. See [README#IAM Role and Policy Configuration](../README.md#iam-role-and-policy-configuration) for the json policy.

### Template Role

The `TimestreamLambdaRole` is the role used by the template in order to permit AWS lambda and API Gateway deployment. See [README#IAM Role and Policy Configuration](../README.md#iam-role-and-policy-configuration) for the json role used.

## Conclusion

Following the above steps you should be able to monitor your Timestream database using Prometheus. Ensure all items in the following list can be verified to ensure the guide has been completed correctly:
- The `PrometheusMetricsTable` table in the `PrometheusDatabase` database is empty.
- AWS CLI is configured with the correct region wishing to deploy connector to.
- User access key id is set.
- User secret access key is set.

### Confirming Application Functionality

Before running Prometheus, the result of the following AWS CLI command should show a `ScalarValue` of `0` within `Data`, if you have been following this document step-by-step:

```shell
aws timestream-query query --query-string "SELECT count() FROM PrometheusDatabase.PrometheusMetricsTable"
```

Next, start Prometheus

```shell
./prometheus
```

On macOS the first time running Prometheus may fail due to the developer being unable to be verified. To continue, you must grant Prometheus execution in `System Settings -> Privacy & Security -> Security -> "prometheus" was blocked from use because it is not from an identified developer. -> Allow Anyway`.  

After successfully starting Prometheus and seeing no errors reported by Prometheus again run

```shell
aws timestream-query query --query-string "SELECT count() FROM PrometheusDatabase.PrometheusMetricsTable"
```

You should now see some non-zero data value within `Data`, which verifies that the Prometheus instance can ingest data into `PrometheusMetricsTable`.

Next, in order to verify that Prometheus can make a successful read request, add data to your table by running

```shell
aws timestream-write write-records --database-name PrometheusDatabase --table-name PrometheusMetricsTable --records '[{"Dimensions":[{"DimensionValueType": "VARCHAR", "Name": "job","Value": "prometheus"},{"DimensionValueType": "VARCHAR","Name": "instance","Value": "localhost:9090"}],"MeasureName":"prometheus_temperature","MeasureValue":"98.76","TimeUnit":"SECONDS","Time":"1694446844"}]'
```

Open the Prometheus web interface (default `localhost:9090`) and within the execution bar run

```
prometheus_temperature[15d]
```

And verify that data is displayed.

> **Note**: The time range (`15d`) must be large enough to trigger the Prometheus read from external endpoint. If the time range is too small then only local data will be read.

### Cleanup

1. Delete cloudformation stack and S3 artifacts

   ```shell
   sam delete --stack-name PrometheusConnector
   ```

2. Delete table

   ```shell
   aws timestream-write delete-table --database-name <PrometheusDatabase> --table-name <PrometheusMetricsTable>
   ```

3. Delete database

   ```shell
   aws timestream-write delete-database --database-name <PrometheusDatabase>
   ```

   > **NOTE**: Cleaning up resources will require additional IAM permissions added to the base required for deployment under [Deployment Permissions](#deployment-permissions)

Required Permissions:
- "apigateway:DELETE"
- "s3:DeleteBucket"
- "s3:DeleteObjectVersion"
- "s3:DeleteObject"
- "s3:DeleteBucketPolicy"
- "iam:DeleteUserPolicy"
- "iam:DeletePolicy"
- "iam:DeleteRole"
- "iam:DetachRolePolicy"
- "iam:DeleteRolePolicy"
- "cloudformation:DeleteStack"
- "lambda:RemovePermission"
- "lambda:DeleteFunction"

## Troubleshooting

### Security Constraints Not Satisfied

If the following error occurred while running `sam deploy`,

```	shell
Error: Security Constraints Not Satisfied!
```

Ensure the following:

1. When executing `sam deploy`, enter `y` for the following question instead of the default `N`:
   **LambdaFunction may not have authorization defined, Is this okay?** [y/N]: y

   The stack will now be created with the following warning: `LambdaFunction may not have authorization defined.`

   This behaviour occurs because the API Gateway triggers configured for the AWS Lambda function do not have authorization defined. This is fine because authorization is done in the Prometheus Connector instead of the API Gateway.

### Role Permission Error

An error occurred during deployment due to invalid permissions.

Do the following:

1. Ensure the user deploying the project has administrative privileges and all required deployment permissions.
2. See all the required permissions in [Deployment Permissions](#deployment-permissions).
3. Redeploy the project.

### Conflicting Resources Error

The deployment fails due to existing resources.

If this error occurred after a failed deployment:

1. Open the AWS Management Console for CloudFormation and delete the failed stack.
2. Redeploy on AWS console.

If this error occurred in a new deployment:

1. Rename the conflict resource name to something else.
2. Redeploy on AWS console.

See the list below for parameters whose values that may result in resource conflicts:
- ExecutionPolicyName

### AWS Lambda Timeout or HTTP Status 404 Not Found

If the Lambda `TimeoutInMillis` parameter is too small or a PromQL query exceeds the `TimeoutInMillis` value a 

```shell
remote_read: remote server https://api-id.execute-api.region.amazonaws.com/dev/read returned HTTP status 404 Not Found: {"message":"Not Found"}
``` 

error could be returned. If you encounter this error first try overriding the default value for `TimeoutInMillis` (30 seconds) with a greater value using the [`--parameter-overrides` option for `sam deploy`](#aws-cli-deployment).

## Caveats

This SAM template does not enable TLS encryption by default between Prometheus and the Prometheus Connector.

During **development**, ensure the following:

1. Regularly rotate IAM user access keys, see [Rotating access keys](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html#Using_RotateAccessKey).
2. Follow the [best practices](https://docs.aws.amazon.com/timestream/latest/developerguide/security_iam_id-based-policy-examples.html#security_iam_service-with-iam-policy-best-practices).

During **production**, enable TLS encryption through Amazon API Gateway, see [Configuring mutual TLS authentication for an HTTP API](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-mutual-tls.html).

## License

This project is licensed under the Apache 2.0 License.
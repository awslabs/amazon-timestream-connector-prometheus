# timestream-prometheus-connector

## Overview

This [serverless application](https://aws.amazon.com/serverless/) consists of the following:
- [Amazon API Gateway](https://docs.aws.amazon.com/apigateway/latest/developerguide/welcome.html) that listens for [Prometheus remote read and write](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) requests;
- [AWS Lambda function](https://docs.aws.amazon.com/lambda/latest/dg/welcome.html) that stores the received Prometheus metrics in [Amazon Timestream](https://docs.aws.amazon.com/timestream/latest/developerguide/what-is-timestream.html).

This application is meant to be used as a getting started guide and does not configure TLS encryption between Prometheus and the API Gateway. This is not recommended to be used directly for production.
To enable TLS encryption for production, see [Configuring mutual TLS authentication for an HTTP API](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-mutual-tls.html).

### Prerequisites
1. [Create a Timestream database](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.db.using-console).
  ```shell
   aws timestream-write create-database --database-name <exampleDatabase>
  ```
2. [Create a Timestream table](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.table.using-console).
  ```shell
   aws timestream-write create-table --database-name <exampleDatabase> --table-name <exampleTable>
  ```
3. Download [Prometheus](https://prometheus.io/download) (or reuse your existing Prometheus instance).

> **NOTE:** The user deploying this application must have administrative privileges due to the number of permissions required for deployment. For a detailed list of permissions see [Required Permissions](#required-permissions).

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
| US East (N. Virginia)     | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-east-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-east-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| US East (Ohio)            | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-east-2&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-east-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| US West (N. California)   | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-west-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-west-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| US West (Oregon)          | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=us-west-2&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=us-west-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| Canada (Central)          | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ca-central-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ca-central-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      |
| South America (São Paulo) | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=sa-east-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=sa-east-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| Europe (Stockholm)        | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-north-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)          | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-north-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        |
| Europe (Ireland)          | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-west-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-west-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| Europe (London)           | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-west-2&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-west-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| Europe (Paris)            | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-west-3&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-west-3#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| Europe (Frankfurt)        | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=eu-central-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        | [Launch](https://console.aws.amazon.com/cloudformation/home?region=eu-central-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      |
| Middle East (Bahrain)     | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=me-south-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)          | [Launch](https://console.aws.amazon.com/cloudformation/home?region=me-south-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        |
| Middle East (UAE)         | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=me-central-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        | [Launch](https://console.aws.amazon.com/cloudformation/home?region=me-central-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      |
| Africa (Cape Town)        | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=af-south-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)          | [Launch](https://console.aws.amazon.com/cloudformation/home?region=af-south-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        |
| Asia Pacific (Hong Kong)  | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-east-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)           | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-east-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)         |
| Asia Pacific (Tokyo)      | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-northeast-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-northeast-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)    |
| Asia Pacific (Seoul)      | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-northeast-2&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-northeast-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)    |
| Asia Pacific (Singapore)  | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-southeast-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-southeast-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)    |
| Asia Pacific (Sydney)     | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-southeast-2&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-southeast-2#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)    |
| Asia Pacific (Mumbai)     | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.aws.amazon.com/cloudformation/designer/home?region=ap-south-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)          | [Launch](https://console.aws.amazon.com/cloudformation/home?region=ap-south-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        |
| China (Beijing)           | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.amazonaws.cn/cloudformation/designer/home?region=cn-north-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)            | [Launch](https://console.amazonaws.cn/cloudformation/home?region=cn-north-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)          |
| China (Ningxia)           | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.amazonaws.cn/cloudformation/designer/home?region=cn-northwest-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)        | [Launch](https://console.amazonaws.cn/cloudformation/home?region=cn-northwest-1#/stacks/new?stackName=PrometheusTimestreamConnector&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml)      |
| AWS GovCloud (US-West)    | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.amazonaws-us-gov.com/cloudformation/designer/home?region=us-gov-west-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) | [Launch](https://console.amazonaws-us-gov.com/cloudformation/home?region=us-gov-west-1#/stacks/new?stackName=NeptuneQuickStart&amp;templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |
| AWS GovCloud (US-East)    | [View](https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |  [View in Designer](https://console.amazonaws-us-gov.com/cloudformation/designer/home?region=us-gov-east-1&templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) | [Launch](https://console.amazonaws-us-gov.com/cloudformation/home?region=us-gov-east-1#/stacks/new?stackName=NeptuneQuickStart&amp;templateURL=https://test-one-click-prometheus-development.s3.amazonaws.com/template.yml) |

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
    sam deploy --parameter-overrides "TimeoutInMillis=60000 PrometheusDatabaseLabel=<CustomDatabaseLabel>"
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
   2. PrometheusDatabaseLabel: The [Prometheus label](https://prometheus.io/docs/practices/naming/#labels) containing the database name.
   2. PrometheusTableLabel: The Prometheus label containing the table name.

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
    
    Key                 PrometheusDatabaseLabel                                                                                                                                                                                                  
    Description         The Prometheus label containing the database name                                                                                                                                                                                 
    Value               PrometheusDatabaseLabel                                                                                                                                                                                                                                                                                               
    
    Key                 PrometheusTableLabel                                                                                                                                                                                                  
    Description         The Prometheus label containing the table name                                                                                                                                                                            
    Value               PrometheusTableLabel                                                                                                                                               
    ------------------------------------------------------------------------------------------------------------
    ```

   To view all the stack information open the for more details see [Viewing AWS CloudFormation stack data and resources on the AWS Management Console](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-console-view-stack-data-resources.html).

## Configuration

> **NOTE:** All configuration options are *case-sensitive*.

### Configure Prometheus

To let the Lambda function know which database/table destination is for a Prometheus time series,
two additional labels need to be added in every Prometheus time series through [Prometheus relabel config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config).
1. Open the configuration file for Prometheus, the file is usually named `prometheus.yml`.

2. Replace the `InvokeWriteURL` and `InvokeReadURL` with the API Gateway URLs from deployment, and provide the appropriate IAM credentials in `basic_auth` before adding the following sections to the configuration file:

```yaml
global:
  scrape_interval:    60s
  evaluation_interval: 60s

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

  write_relabel_configs:
      - source_labels: ["__name__"]
        regex: .*
        replacement: exampleTable
        target_label: PrometheusTableLabel
      - source_labels: ["__name__"]
        regex: .*
        replacement: exampleDatabase
        target_label: PrometheusDatabaseLabel

remote_read:
# Update the value to the InvokeReadURL returned when deploying the stack.
- url: "InvokeReadURL"

  # Update the username and password to a valid IAM access key and secret access key.
  basic_auth:
      username: accessKey
      password_file: passwordFile
```

The `write_relabel_configs` section adds two additional labels to all Prometheus time series specifying the ingestion destination for each of the time series. These two labels will only be used by the connector and will not be ingested into Timestream.
> **NOTE:** The values specified in `target_label` must match the values specified for parameters `PrometheusDatabaseLabel` and `PrometheusTableLabel` during deployment. Users can omit the `write_relabel_configs` section if using default database and tables, see [README#Standard Configuration Options](../README.md#standard-configuration-options) for details.

### Start Prometheus

1. Ensure the user invoking the AWS Lambda function has read and write permissions to Amazon Timestream. For more details see [Execution Permissions](#execution-permissions).
2. Start Prometheus. Since the remote storage options for Prometheus has been configured, Prometheus will start ingesting to Timestream through the API Gateway endpoints.

### Verification
Follow the verification steps in [GETTING_STARTED.MD#verification](../GETTING_STARTED.md#verification).

### AWS Lambda Configuration Options

| Option                    | Description                                                  | Default Value                   |
| ------------------------- | ------------------------------------------------------------ | ------------------------------- |
| PrometheusDatabaseLabel   | The Prometheus label containing the database name.           | PrometheusDatabaseLabel              |
| PrometheusTableLabel      | The Prometheus label containing the table name.              | PrometheusTableLabel                 |
| MemorySize                | The memory size of the AWS Lambda function.                  | 512                             |
| TimeoutInMillis           | The amount of time in milliseconds to run the connector on AWS Lambda before timing out. | 30000                           |
| ReadThrottlingBurstLimit  | The number of burst read requests per second that API Gateway permits. | 1200                             |
| WriteThrottlingBurstLimit | The number of burst write requests per second that API Gateway permits. | 1200                             |

`PrometheusDatabaseLabel` and `PrometheusTableLabel` are required for multi-destination to specify where the data should be stored.
[Configure Prometheus](#configure-prometheus) step 2 contains a configuration example for multi-destination.

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

The user **deploying** this project **must** have the following permission allowing the template to perform specific actions:

A policy template with the required deployment permissions listed below; ensure the values of `account-id` and `region` in the resources section are updated before using this template directly:

```json
{
	"Version": "2012-10-17",
	"Statement": [
		{
			"Sid": "VisualEditor0",
			"Effect": "Allow",
			"Action": [
				"serverlessrepo:CreateCloudFormationTemplate",
				"serverlessrepo:GetCloudFormationTemplate",
				"serverlessrepo:CreateCloudFormationChangeSet",
				"cloudformation:CreateChangeSet",
				"cloudformation:DescribeStacks",
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
			"Resource": "arn:aws:iam::<account-id>:role/PrometheusTimestreamConnector-IAMLambdaRole-*"
		},
		{
			"Sid": "VisualEditor2",
			"Effect": "Allow",
			"Action": [
				"cloudformation:DescribeStackEvents",
				"cloudformation:DescribeChangeSet",
				"cloudformation:ExecuteChangeSet",
				"cloudformation:GetTemplate",
				"cloudformation:CreateStack",
				"cloudformation:GetStackPolicy"
			],
			"Resource": [
				"arn:aws:cloudformation:<region>:<account-id>:stack/PrometheusTimestreamConnector/*",
				"arn:aws:cloudformation:<region>:<account-id>:stack/aws-sam-cli-managed-default/*"
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
			// TODO: Update with public s3 bucket with template and connector
			"Resource": "arn:aws:s3:::<s3-bucket>/*"
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
		}
	]
}
```

The user **executing** this project **must** have the following permission allowing the template to perform specific actions:

A policy template with the required execution permissions listed below; ensure the values of `account-id`, `region`, `exampleDatabase`, and `exampleTable` in the resources section are updated before using this template directly:

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
      "Resource": "arn:aws:timestream:<region>:<account-id>:database/<exampleDatabase>/table/<exampleTable>"
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

## Template IAM Permissions

Running the Prometheus Connector on AWS Lambda allows for a serverless workflow. This section details the [IAM](https://docs.aws.amazon.com/IAM/latest/UserGuide/introduction.html) permissions created by the template to integrate the Prometheus Connector with Amazon API Gateway and AWS Lambda.

### Template Role

The `LambdaExecutionPolicy` created by the template allows the lambda function to output logs to cloudwatch. See [README#IAM Role and Policy Configuration](../README.md#iam-role-and-policy-configuration) for the json policy.

### Execution Policy

The `TimestreamLambdaRole` is the role used by the template in order to permit AWS lambda and APIGateway deployment. See [README#IAM Role and Policy Configuration](../README.md#iam-role-and-policy-configuration) for the json role used.

## Conclusion

Following the above steps you should be able to monitor your Timestream database using Prometheus. Ensure all items in the following list can be verified to ensure the guide has been completed correctly:
- The `exampleTable` table in the `exampleDatabase` database is empty.
- AWS CLI is configured with the correct region wishing to deploy connector to.
- User access key id is set.
- User secret access key is set.

### Confirming Application Functionality

Before running Prometheus, the result of the following AWS CLI command should show a `ScalarValue` of `0` within `Data`, if you have been following this document step-by-step:
```shell
aws timestream-query query --query-string "SELECT count() FROM exampleDatabase.exampleTable"
```
Next start Prometheus
```shell
./prometheus
```
On macOS the first time running Prometheus may fail due to the developer being unable to be verified. To continue, you must grant Prometheus execution in `System Settings -> Privacy & Security -> Security -> "prometheus" was blocked from use because it is not from an identified developer. -> Allow Anyway`.  

After successfully starting Prometheus and seeing no errors reported by Prometheus again run
```shell
aws timestream-query query --query-string "SELECT count() FROM exampleDatabase.exampleTable"
```
You should now see some non-zero data value within `Data`, which verifies that the Prometheus instance can ingest data into `exampleTable`.

Next, in order to verify that Prometheus can make a succesful read request, add data to your table by running
```shell
aws timestream-write write-records --database-name exampleDatabase --table-name exampleTable --records '[{"Dimensions":[{"DimensionValueType": "VARCHAR", "Name": "job","Value": "prometheus"},{"DimensionValueType": "VARCHAR","Name": "instance","Value": "localhost:9090"}],"MeasureName":"prometheus_temperature","MeasureValue":"98.76","TimeUnit":"SECONDS","Time":"1694446844"}]'
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
  aws timestream-write delete-table --database-name <exampleDatabase> --table-name <exampleTable>
  ```

3. Delete database
  ```shell
  aws timestream-write delete-database --database-name <exampleDatabase>
  ```

> **NOTE:** Cleaning up resources will require additional IAM permissions added to the base required for deployment under [Deployment Permissions](#deployment-permissions)

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

Do as follows:

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
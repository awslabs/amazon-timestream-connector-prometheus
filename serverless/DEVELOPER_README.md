# timestream-prometheus-connector

## Overview

This serverless application consists of the following:
- Amazon API Gateway that listens for [Prometheus remote read and write](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) requests;
- AWS Lambda function that stores the received Prometheus metrics in Amazon Timestream.

This application is meant to be used as a getting started guide and does not configure TLS encryption between Prometheus and the API Gateway. This is not recommended to be used directly for production.
To enable TLS encryption for production, see [Configuring mutual TLS authentication for an HTTP API](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-mutual-tls.html).

### Prerequisites
1. [Create a Timestream database](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.db.using-console)
2. [Create a Timestream table](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.table.using-console)
3. Download [Prometheus](https://github.com/prometheus/prometheus)

> **NOTE:** The user deploying this application must have administrative privileges due to the number of permissions required for deployment. For a detailed list of permissions see [Required Permissions](#required-permissions).

## Getting Started

To start using the Prometheus remote storage connector for Timestream, there are multiple steps involved:

1. **Deployment** &mdash; deploy the endpoints through Amazon API Gateway and deploy the Prometheus Connector on AWS Lambda.
2. **Configure Prometheus** &mdash; configure the remote storage endpoints for Prometheus.
3. **Invoke AWS Lambda Function** &mdash; update the permissions for the users.

### Deployment

There are two template available to deploy the application:
- [Simple template](simple-template.yml) &mdash; deploys the application with all resources created with pre-determined values.
- [Advanced template](advanced-template.yml) &mdash; provides parameters to configure the resources being created.

The steps to deploy a template is as follows:
1. From the repository root, run one of the following commands to deploy the desired template:

   - **Simple template**
   ```shell
   sam deploy --template simple-template.yml --guided --capabilities CAPABILITY_NAMED_IAM
   ```
   - **Advanced template**
   ```shell
   sam deploy --template advanced-template.yml --guided --capabilities CAPABILITY_NAMED_IAM
   ```

   The command creates a new stack as well as a default stack called `aws-sam-cli-managed-default`. The default stack manages a S3 bucket hosting all the deployment artifacts:

   - the SAM template;
   - the precompiled connector binary for Linux.

   To use a specific S3 bucket run:

    ```shell
    sam deploy --template advanced-template.yml --guided --capabilities CAPABILITY_NAMED_IAM --s3-bucket custom-bucket
    ```

   While stepping through the configuration, enter `y` for the step `LambdaFunction may not have authorization defined, Is this okay? [y/N]`. For more details, see [Security Constraints Not Satisfied](#security-constraints-not-satisfied).

   An example configuration for the advanced template:
   ```
   Setting default arguments for 'sam deploy'
   =========================================
   Stack Name [sam-app]: 
   AWS Region [us-east-1]: 
   Parameter LambdaFunctionName [prometheusConnectorLambda]: 
   Parameter PrometheusDatabaseLabel [prometheusDatabaseLabel]: 
   Parameter PrometheusTableLabel [prometheusTableLabel]: 
   Parameter MemorySize [512]: 
   Parameter TimeoutInMillis [15000]: 
   Parameter ReadThrottlingBurstLimit [1200]: 
   Parameter WriteThrottlingBurstLimit [1200]: 
   Parameter RoleName [timestreamLambdaRole]: 
   Parameter ExecutionPolicyName [lambdaBasicExecutionPolicy]: 
   Parameter TimestreamPolicyName [timestreamReadAndWritePolicy]: 
   Parameter APIGatewayStageName [prod]: 
   #Shows you resources changes to be deployed and require a 'Y' to initiate deploy
   Confirm changes before deploy [y/N]: 
   #SAM needs permission to be able to create roles to connect to the resources in your template
   Allow SAM CLI IAM role creation [Y/n]: 
   LambdaFunction may not have authorization defined, Is this okay? [y/N]: y
   LambdaFunction may not have authorization defined, Is this okay? [y/N]: y
   Save arguments to configuration file [Y/n]: 
   SAM configuration file [samconfig.toml]: 
   SAM configuration environment [default]: 
   ```

2. The deployment will have the following outputs upon completion:

   1. InvokeReadURL: The remote read URL for Prometheus.
   2. InvokeWriteURL: The remote write URL for Prometheus.
   2. PrometheusDatabaseLabel: The Prometheus label containing the database name.
   2. PrometheusTableLabel: The Prometheus label containing the table name.

   An example of the output:

    ```
    CloudFormation outputs from deployed stack
    ------------------------------------------------------------------------------------------------------------
    Outputs                                                                                                                                                                                                                             
    ------------------------------------------------------------------------------------------------------------
    Key                 InvokeReadURL                                                                                                                                                                                                   
    Description         Remote read URL for Prometheus                                                                                                                                                                                  
    Value               https://api-id.execute-api.us-east-1.amazonaws.com/prod/read                                                                                                                                                                                                                                                                                      
    
    Key                 InvokeWriteURL                                                                                                                                                                                                  
    Description         Remote write URL for Prometheus                                                                                                                                                                                 
    Value               https://api-id.execute-api.us-east-1.amazonaws.com/prod/write                                                                                                                                                                                                                                                                                               
    
    Key                 PrometheusDatabaseLabel                                                                                                                                                                                                  
    Description         The Prometheus label containing the database name                                                                                                                                                                                 
    Value               prometheusDatabaseLabel                                                                                                                                                                                                                                                                                               
    
    Key                 PrometheusTableLabel                                                                                                                                                                                                  
    Description         The Prometheus label containing the table name                                                                                                                                                                            
    Value               prometheusTableLabel                                                                                                                                               
    ------------------------------------------------------------------------------------------------------------
    ```

   To view all the stack information open the for more details see [Viewing AWS CloudFormation stack data and resources on the AWS Management Console](https://docs.aws.amazon.com/AWSCloudFormation/latest/UserGuide/cfn-console-view-stack-data-resources.html).

### Configure Prometheus

To let the Lambda function know which database/table destination is for a Prometheus time series,
two additional labels need to be added in every Prometheus time series through [Prometheus relabel config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config).
1. Open the configuration file for Prometheus, the file is usually named `prometheus.yml`.

2. Replace the `InvokeWriteURL` and `InvokeReadURL` with the API Gateway URLs from deployment, and provide the appropriate IAM credentials in `basic_auth` before adding the following sections to the configuration file:

   ```yaml
    remote_write:
      # Update the value to the InvokeWriteURL returned when deploying the stack.
      - url: "InvokeWriteURL"
       
        # Update the username and password to a valid IAM user access key and secret access key.
        basic_auth:
          username: accessKey
          password: secretAccessKey
      
        write_relabel_configs:
           - source_labels: ["__name__"]
             regex: .*
             replacement: exampleTable
             target_label: prometheusTableLabel
           - source_labels: ["__name__"]
             regex: .*
             replacement: exampleDatabase
             target_label: prometheusDatabaseLabel

    remote_read:
      # Update the value to the InvokeReadURL returned when deploying the stack.
      - url: "InvokeReadURL"
   
        # Update the username and password to a valid IAM user access key and secret access key.
        basic_auth:
          username: accessKey
          password: secretAccessKey
   ```

   The `write_relabel_configs` section adds two additional labels to all Prometheus time series specifying the ingestion destination for each of the time series. These two labels will only be used by the connector and will not be ingested into Timestream.

   > **NOTE:** The values specified in `target_label` must match the values specified for parameters `PrometheusDatabaseLabel` and `PrometheusTableLabel` during deployment.

3. See a full example as follows:
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
    
      # Update the username and password to a valid IAM access key and secret access key.
      basic_auth:
          username: accessKey
          password: secretAccessKey
    
      write_relabel_configs:
        - source_labels: ["__name__"]
          regex: .*
          replacement: exampleTable
          target_label: prometheusTableLabel
        - source_labels: ["__name__"]
          regex: .*
          replacement: exampleDatabase
          target_label: prometheusDatabaseLabel
    
    remote_read:
    # Update the value to the InvokeReadURL returned when deploying the stack.
    - url: "InvokeReadURL"
    
      # Update the username and password to a valid IAM access key and secret access key.
      basic_auth:
          username: accessKey
          password: secretAccessKey
   ```

### Start Prometheus

1. Ensure the user invoking the AWS Lambda function has read and write permissions to Amazon Timestream. For more details see [Execution Permissions](#execution-permissions).
2. Start Prometheus. Since the remote storage options for Prometheus has been configured, Prometheus will start ingesting to Timestream through the API Gateway endpoints.

## Configuration

> **NOTE:** All configuration options are *case-sensitive*.

### AWS Lambda Configuration Options

| Option                    | Description                                                  | Default Value                   |
| ------------------------- | ------------------------------------------------------------ | ------------------------------- |
| LambdaFunctionName        | Name of the AWS Lambda function running the connector.       | prometheusConnectorLambda |
| PrometheusDatabaseLabel   | The Prometheus label containing the database name.           | prometheusDatabaseLabel              |
| PrometheusTableLabel      | The Prometheus label containing the table name.              | prometheusTableLabel                 |
| MemorySize                | The memory size of the AWS Lambda function.                  | 512                             |
| TimeoutInMillis           | The amount of time in milliseconds to run the connector on AWS Lambda before timing out. | 15000                           |
| ReadThrottlingBurstLimit  | The number of burst read requests per second that API Gateway permits. | 1200                             |
| WriteThrottlingBurstLimit | The number of burst write requests per second that API Gateway permits. | 1200                             |

`PrometheusDatabaseLabel` and `PrometheusTableLabel` are required for multi-destination to specify where the data should be stored.
[Configure Prometheus](#configure-prometheus) step 2 contains a configuration example for multi-destination.

### IAM Permissions Configuration Options

| Option               | Description                                                  | Default Value                  |
| -------------------- | ------------------------------------------------------------ | ------------------------------ |
| RoleName             | The name of the service role created for the AWS Lambda function. | timestreamLambdaRole |
| ExecutionPolicyName  | The name of the basic execution policy created for AWS Lambda. | lambdaBasicExecutionPolicy  |

### Amazon API Gateway Configuration Options

| Option              | Description                        | Default Value |
| ------------------- | ---------------------------------- | ------------- |
| APIGatewayStageName | The stage name of the API Gateway. Stage names can contain only alphanumeric characters, hyphens, and underscores. | prod          |
The default stage name `prod` may indicate the endpoint is at `production` stage.
If the application is not ready for production, set the stage name to a more appropriate value like `dev` when deploying the stack with `advanced-template.yml`.

## Required Permissions

The template assumes the user deploying the project has administrative permissions. If the user is missing any of the required permissions the deployment will fail.
See [Troubleshooting](#troubleshooting) section for more details.

### Deployment Permissions

The user **deploying** this project **must** have the following permission allowing the template to perform specific actions:

| No   | Action Performed by the Template                             | Required Permissions                                         |
| ---- | ------------------------------------------------------------ | ------------------------------------------------------------ |
| 1    | Create a S3 bucket to store the SAM template and the AWS Lambda function code | "s3:*"                                                       |
| 2    | Create a service role for the AWS Lambda function            | "iam:CreateRole"<br />"iam:PutRolePolicy"                    |
| 3    | Create and attach the following permissions to the service role | "iam:CreatePolicy"<br />"iam:AttachRolePolicy"               |
| 4    | Clean up created resources in case the deployment fails      | "iam:DetachRolePolicy"<br />"iam:DeleteRole"<br />"iam:DeletePolicy"<br />"iam:DeleteUserPolicy" |

A policy template with all the required deployment permissions mentioned above:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "s3:*",
        "iam:CreateRole",
        "iam:PutRolePolicy",
        "iam:CreatePolicy",
        "iam:AttachRolePolicy",
        "iam:DetachRolePolicy",
        "iam:DeleteRole",
        "iam:DeletePolicy",
        "iam:DeleteUserPolicy"
      ],
      "Resource": "*"
    }
  ]
}
```

### Execution Permissions

The user **invoking** the AWS Lambda function **must** have the following permissions:

| No   | Description                                                  | Required Permissions |
| ---- | ------------------------------------------------------------ | -------------------- |
| 1    | Invoke the AWS Lambda function with Prometheus remote read and remote write requests. | "timestream:WriteRecords", "timestream:Select", "timestream:DescribeEndpoints"      |

The following sample policy provides read and write permissions to the `exampleDatabase` in the `us-east-1` region of the IAM user account with ID `123456789`. For more example and all Amazon Timestream permissions see the official [documentation](https://docs.aws.amazon.com/timestream/latest/developerguide/security-iam.html).
The `"timestream:DescribeEndpoints"` permission allows the AWS SDK for Go to look up the correct Amazon Timestream API endpoint, for more details see [How the Endpoint Discovery Pattern Works](https://docs.aws.amazon.com/timestream/latest/developerguide/Using-API.endpoint-discovery.how-it-works.html).

> **NOTE:** Replace the value for "Resource" from to the appropriate database ARN. The role ARN can be found on the AWS Management Console for [Amazon Timestream](https://console.aws.amazon.com/timestream).

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
      "Resource": "arn:aws:timestream:us-east-1:123456789:database/exampleDatabase"
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

## Troubleshooting

### Security Constraints Not Satisfied

If the following error occurred while running `sam deploy --capabilities CAPABILITY_NAMED_IAM --guided`,

```	shell
Error: Security Constraints Not Satisfied!
```

Ensure the following:

1. When executing `sam deploy --capabilities CAPABILITY_NAMED_IAM --guided`, enter `y` for the following question instead of the default `N`:
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
- LambdaFunctionName
- RoleName
- ExecutionPolicyName

## Caveats
This SAM template does not enable TLS encryption by default between Prometheus and the Prometheus Connector.

During **development**, ensure the following:
1. Regularly rotate IAM user access keys, see [Rotating access keys](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html#Using_RotateAccessKey).
2. Follow the [best practices](https://docs.aws.amazon.com/timestream/latest/developerguide/security_iam_id-based-policy-examples.html#security_iam_service-with-iam-policy-best-practices).

During **production**, enable TLS encryption through Amazon API Gateway, see [Configuring mutual TLS authentication for an HTTP API](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-mutual-tls.html).

## License

This project is licensed under the Apache 2.0 License.
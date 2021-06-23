# timestream-prometheus-connector

## Overview

This serverless application consists of the following:
- Amazon API Gateway that listens for [Prometheus remote write](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) requests;
- AWS Lambda function that stores the received Prometheus metrics in Amazon Timestream.

This application is meant to be used as a getting started guide and does not configure TLS encryption between Prometheus and the API Gateway. This is not recommended to be used directly for production.
To enable TLS encryption for production, see [Configuring mutual TLS authentication for an HTTP API](https://docs.aws.amazon.com/apigateway/latest/developerguide/http-api-mutual-tls.html).

This application can only be deployed once per region. Deploying this template multiple times will fail due to resource conflicts. 
To deploy another stack, deploy with the advanced serverless application `timestream-prometheus-connector-advanced` which allows configuring the resources.

### Prerequisites
1. [Create a Timestream database](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.db.using-console)
2. [Create a Timestream table](https://docs.aws.amazon.com/timestream/latest/developerguide/console_timestream.html#console_timestream.table.using-console)
3. Download [Prometheus](https://github.com/prometheus/prometheus)

> **NOTE**: The user deploying this application must have administrative privileges due to the number of permissions required for deployment. For a detailed list of permissions see the **Required Permissions** section.

## Getting Started

To start using the Prometheus remote storage connector for Timestream, there are multiple steps involved:

1. **Deployment** &mdash; deploy the endpoints through Amazon API Gateway and deploy the Prometheus Connector on AWS Lambda.
2. **Configure Prometheus** &mdash; configure the remote storage endpoints for Prometheus.
3. **Invoke AWS Lambda Function** &mdash; update the permissions for the users.

### Deployment

1. Enter an "Application Name".
2. Check the “I acknowledge that this app creates custom IAM roles”.
3. Press Deploy.

When deploying the application, there are several options that could be configured.
This deployment will set all the options to the default values. For more control over the options, deploy with the advanced serverless application `timestream-prometheus-connector-advanced`.

#### AWS Lambda Configuration Options
| Option                    | Description                                                  | Default Value                   |
| :------------------------ | :----------------------------------------------------------- | :------------------------------ |
| LambdaFunctionName        | Name of the AWS Lambda function running the connector.       | prometheusConnectorLambda |
| PrometheusDatabaseLabel   | The Prometheus label containing the database name.           | prometheusDatabaseLabel              |
| PrometheusTableLabel      | The Prometheus label containing the table name.              | prometheusTableLabel                 |
| MemorySize                | The memory size of the AWS Lambda function.                  | 512                             |
| TimeoutInMillis           | The amount of time in milliseconds to run the connector on AWS Lambda before timing out. | 15000                           |
| WriteThrottlingBurstLimit | The number of burst write requests per second that API Gateway permits. | 1200                             |

#### IAM Permissions Configuration Options

| Option               | Description                                                  | Default Value                  |
| :------------------- | :----------------------------------------------------------- | :----------------------------- |
| RoleName             | The name of the service role created for the AWS Lambda function. | timestreamLambdaRole |
| ExecutionPolicyName  | The name of the basic execution policy created for AWS Lambda. | lambdaBasicExecutionPolicy  |

#### Amazon API Gateway Configuration Options

| Option              | Description                        | Default Value |
| :------------------ | :--------------------------------- | :------------ |
| APIGatewayStageName | The stage name of the API Gateway. Stage names can contain only alphanumeric characters, hyphens, and underscores. | prod          |
The default stage name `prod` may indicate the endpoint is at `production` stage.
If the application is not ready for production, set the stage name to a more appropriate value like `dev` when deploying the stack with the advanced serverless application `timestream-prometheus-connector-advanced`.

### Configure Prometheus

To let the Lambda function know which database/table destination is for a Prometheus time series,
two additional labels need to be added in every Prometheus time series through [Prometheus relabel config](https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config).
1. Open the configuration file for Prometheus, the file is usually named `prometheus.yml`.

2. Replace the `InvokeWriteURL` with the API Gateway URL from deployment, and provide the appropriate IAM credentials in `basic_auth` before adding the following sections to the configuration file:

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
   ```

### Start Prometheus

1. Ensure the user invoking the AWS Lambda function has write permissions to Amazon Timestream. For more details see the **Execution Permissions** section.
2. Start Prometheus. Since the remote storage options for Prometheus has been configured, Prometheus will start ingesting to Timestream through the API Gateway endpoints.

## Required Permissions

The template assumes the user deploying the project has administrative permissions. If the user is missing any of the required permissions the deployment will fail.
See the **Troubleshooting** section for more details.

### Deployment Permissions

The user **deploying** this project **must** have the following permission allowing the template to perform specific actions:

| No   | Action Performed by the Template                             | Required Permissions                                         |
| :--- | :----------------------------------------------------------- | :----------------------------------------------------------- |
| 1    | Create a S3 bucket to store the SAM template and the AWS Lambda function code | "s3:*"                                                       |
| 2    | Create a service role for the AWS Lambda function            | "iam:CreateRole", "iam:PutRolePolicy"                    |
| 3    | Create and attach the following permissions to the service role | "iam:CreatePolicy", "iam:AttachRolePolicy"               |
| 4    | Clean up created resources in case the deployment fails      | "iam:DetachRolePolicy", "iam:DeleteRole", "iam:DeletePolicy", "iam:DeleteUserPolicy" |

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
| :--- | :----------------------------------------------------------- | :------------------- |
| 1    | Invoke the AWS Lambda function with Prometheus remote write requests. | "timestream:WriteRecords", "timestream:DescribeEndpoints"      |

The following sample policy provides write permissions to the `exampleDatabase` in the `us-east-1` region of the IAM user account with ID `123456789`. For more example and all Amazon Timestream permissions see the official [documentation](https://docs.aws.amazon.com/timestream/latest/developerguide/security-iam.html).
The `"timestream:DescribeEndpoints"` permission allows the AWS SDK for Go to look up the correct Amazon Timestream API endpoint, for more details see [How the Endpoint Discovery Pattern Works](https://docs.aws.amazon.com/timestream/latest/developerguide/Using-API.endpoint-discovery.how-it-works.html).

> **NOTE:** Replace the value for "Resource" from to the appropriate database ARN. The role ARN can be found on the AWS Management Console for [Amazon Timestream](https://console.aws.amazon.com/timestream).

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Effect": "Allow",
      "Action": [
        "timestream:WriteRecords"
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
2. See all the required permissions in the **Deployment Permissions** section.
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

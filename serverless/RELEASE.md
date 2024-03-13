# Release Process

This documentation outlines the steps required to package and publish the AWS Serverless Application defined by the AWS Serverless Application Model (AWS SAM) templates.

A template is available: [template.yml](template.yml) &mdash; provides parameters to configure the resources being created.
The template deploys an AWS Lambda function and an Amazon API Gateway that listens for [Prometheus remote write/read](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) requests and stores metrics to Amazon Timestream.

## Prerequisites

1. **Install the AWS SAM CLI** &mdash; see [Installing the AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-install.html) to install the CLI on the desired platform.
2. **Set up AWS credentials** &mdash; see [Setting up AWS credentials](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-getting-started-set-up-credentials.html) from the AWS Serverless Application Model Developer Guide.

## Package and Publish the Application

### Buckets

The Amazon Timestream Prometheus connector is deployed in a public S3 bucket in seven regions:

- `us-east-1`
- `us-east-2`
- `us-west-2`
- `ap-southeast-2`
- `ap-northeast-1`
- `eu-central-1`
- `eu-west-1`

The buckets are named `timestreamassets-<region>`, where `<region>` matches a region in the above list.

### Steps

1. Open the [AWS Management Console for S3](https://s3.console.aws.amazon.com/s3/).

2. Use an existing public S3 bucket used for the Amazon Timestream Prometheus connector. This guide uses the bucket `timestreamassets-<region>`, where `<region>` matches any of the regions listed in "[Buckets](#buckets)."

3. Open the bucket on the AWS Console, and choose the `Permissions` tab.

4. Click the `Edit` button in the `Bucket Policy` section.

5. If this policy does not already exist, paste the following policy in the editor, replacing `<region>` and `<version>` appropriately:
    ```json
    {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Sid": "AllowExternalAccountToGetObjects",
                "Effect": "Allow",
                "Principal": {
                    "AWS": [
                        "*"
                    ]
                },
                "Action": "s3:GetObject",
                "Resource": [
                    "arn:aws:s3:::timestreamassets-<region>/template.yml",
                    "arn:aws:s3:::timestreamassets-<region>/timestream-prometheus-connector-linux-amd64-<version>.zip"
                ]
            }
        ]
    }
    ```
    This policy provides the AWS Serverless Application Repository read access to the example bucket `timestreamassets-<region>`. 
   
4. Run the following command to package the desired template, replacing `<region>` with your desired region:

    - **Linux and MacOS**
        ```shell
        sam package \
        --template-file template.yml \
        --output-template-file packaged.yml \
        --s3-bucket timestreamassetts-<region>
        ```

    - **Windows**
        ```shell
        sam package ^
        --template-file template.yml ^
        --output-template-file packaged.yml ^
        --s3-bucket timestreamassets-<region>
        ```

    The command will upload the template and the Prometheus Connector binary to the S3 bucket and output a new template called `packaged.yml`.
    The `packaged.yml` has the same content as the template, but all references to the Prometheus Connector binary will be replaced by references to the S3 bucket. For more information see the [SAM CLI reference](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-package.html).
    
4. Run the following command to publish the package:
    
    > **NOTE**: Run the command with `<region>` set to the appropriate region.
   
    - **Linux and MacOS**
        ```shell
        sam publish \
        --template packaged.yml \
        --region <region>
        ```
      
    - **Windows**
        ```shell
        sam publish ^
        --template packaged.yml ^
        --region <region>
        ```
    This command will publish the application to the AWS Serverless Application Repository in your chosen region. For more details, see the [SAM CLI reference](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-publish.html).

## License

This project is licensed under the Apache 2.0 License.

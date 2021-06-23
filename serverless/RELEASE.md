# Release Process

This documentation outlines the steps required to package and publish the AWS Serverless Application defined by the AWS Serverless Application Model (AWS SAM) templates.
There are two template available:
1. [Simple template](simple-template.yml) &mdash; deploys the application with all resources created with pre-determined values.
2. [Advanced template](advanced-template.yml) &mdash; provides parameters to configure the resources being created.
Both templates deploy an AWS Lambda function and an Amazon API Gateway that listens for [Prometheus remote write](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations) requests and stores metrics to Amazon Timestream.

## Prerequisites

1. **Install the AWS SAM CLI** &mdash; see [Installing the AWS SAM CLI](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-sam-cli-install.html) to install the CLI on the desired platform.
2. **Set up AWS credentials** &mdash; see [Setting up AWS credentials](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/serverless-getting-started-set-up-credentials.html) from the AWS Serverless Application Model Developer Guide.

## Package and Publish the Application

> **NOTE**: The steps as follows assume the application will be published to `us-east-1`, if the application needs to be published to a different region, ensure the S3 bucket is created in the appropriate region.

1. Open the [AWS Management Console for S3](https://s3.console.aws.amazon.com/s3/).

2. Create a new bucket. The bucket will be used to store the SAM template and the function code for the Prometheus Connector. This guide will name the bucket `timestream-prometheus-connector-artifacts`.

3. After creating the bucket, open the bucket on the AWS Console, and choose the `Permissions` tab.

4. Click the `Edit` button in the `Bucket Policy` section.

5. Paste the following policy in the editor:
    ```json
    {
        "Version": "2012-10-17",
        "Statement": [
            {
                "Effect": "Allow",
                "Principal": {
                    "Service": "serverlessrepo.amazonaws.com"
                },
                "Action": "s3:GetObject",
                "Resource": "arn:aws:s3:::timestream-prometheus-connector-template-artifacts/*"
            }
        ]
    }
    ```
    This policy provides the AWS Serverless Application Repository read access to the example bucket `timestream-prometheus-connector-template-artifacts`. 
    Update the bucket name `timestream-prometheus-connector-template-artifacts` for the `Resource` parameter of policy if necessary.
   
4. Run one of the following commands to package the desired template:
    
    1. **Simple template**
       
        - **Linux and MacOS**
            ```shell
            sam package \
            --template-file simple-template.yml \
            --output-template-file packaged.yml \
            --s3-bucket timestream-prometheus-connector-template-artifacts
            ```
        
        - **Windows**
            ```shell
            sam package ^
            --template-file simple-template.yml ^
            --output-template-file packaged.yml ^
            --s3-bucket timestream-prometheus-connector-template-artifacts
            ```
    2. **Advanced template**

        - **Linux and MacOS**
            ```shell
            sam package \
            --template-file advanced-template.yml \
            --output-template-file packaged.yml \
            --s3-bucket timestream-prometheus-connector-template-artifacts
            ```

        - **Windows**
             ```shell
             sam package ^
             --template-file advanced-template.yml ^
             --output-template-file packaged.yml ^
             --s3-bucket timestream-prometheus-connector-template-artifacts
             ```

    The command will upload the template and the Prometheus Connector binary to the S3 bucket and output a new template called `packaged.yml`.
    The `packaged.yml` has the same content as the simple or advanced template, but all references to the Prometheus Connector binary will be replaced by references to the S3 bucket. For more information see the [SAM CLI reference](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-package.html).
    
4. Run the following command to publish the package:
    
    > **NOTE**: If the application needs to be published in a different region, run the command with the value set to the appropriate region.
   
    - **Linux and MacOS**
        ```shell
        sam publish \
        --template packaged.yml \
        --region us-east-1
        ```
      
    - **Windows**
        ```shell
        sam publish ^
        --template packaged.yml ^
        --region us-east-1
        ```
    This command will publish the application to the AWS Serverless Application Repository in the `us-east-1` region. For more details, see the [SAM CLI reference](https://docs.aws.amazon.com/serverless-application-model/latest/developerguide/sam-cli-command-reference-sam-publish.html).

## License

This project is licensed under the Apache 2.0 License.
# Getting Started Guide for Amazon Timestream Prometheus Connector

## Table of Contents
- [Overview](#overview)
- [Terminology](#terminology)
- [Configure Amazon Timestream](#configure-amazon-timestream)
   - [Configure AWS Credentials](#configure-aws-credentials)
   - [Create a database and table on Amazon Timestream](#create-a-database-and-table-on-amazon-timestream)
- [Configure Prometheus Connector](#configure-prometheus-connector)
   - [Linux Binary](#linux-binary)
   - [Docker Image](#docker-image)
- [Configure Prometheus](#configure-prometheus)
   - [Configure TLS Encryption in Prometheus](#configure-tls-encryption-in-prometheus)
- [Verification](#verification)
- [Troubleshooting](#troubleshooting)
- [License](#license)

## Overview

This tutorial consists of the following getting started steps:

1. Set up a new Amazon Web Services (AWS) account.
2. Set up the time series database service Amazon Timestream.
3. Integrate Amazon Timestream as the monitoring system Prometheus' [remote storage](https://prometheus.io/docs/prometheus/latest/storage/#remote-storage-integrations).

## Terminology

This getting started guide defines the following terms:

- **AWS**: the cloud platform [Amazon Web Services (AWS)](https://aws.amazon.com/what-is-aws/).
- **AWS CLI**: the [AWS Command Line Interface (AWS CLI)](https://docs.aws.amazon.com/cli/latest/userguide/cli-chap-welcome.html).
- **Amazon Timestream**: the time series database service [Amazon Timestream](https://aws.amazon.com/timestream/).
- **Prometheus Connector**: the Prometheus remote storage connector for [Amazon Timestream](https://aws.amazon.com/timestream/).
- **Prometheus**: the open source monitoring system [Prometheus]( https://prometheus.io/).

## Configure Amazon Timestream

### Configure AWS Credentials

1. [Create and activate a new AWS account](https://aws.amazon.com/premiumsupport/knowledge-center/create-and-activate-aws-account/).
2. This guide uses the AWS CLI to access Amazon Timestream. To configure the AWS CLI for Amazon Timestream see [Accessing Amazon Timestream Using the AWS CLI](https://docs.aws.amazon.com/timestream/latest/developerguide/Tools.CLI.html).
3. Ensure the account or user running this has sufficient access to Amazon Timestream. See [Identity and Access Management for Amazon Timestream](https://docs.aws.amazon.com/timestream/latest/developerguide/security-iam.html) for all the policies and permissions available for Amazon Timestream.

### Create a database and table on Amazon Timestream

1. Create a database called `exampleDatabase` by running the following command in a command-line interface:

   ```shell
   aws timestream-write create-database --database-name exampleDatabase
   ```

2. Create a table called `exampleTable` within `exampleDatabase` with the following command:

   ```shell
   aws timestream-write create-table --database-name exampleDatabase --table-name exampleTable
   ```

3. Run the following `describe-table` command to ensure that the database and table creation succeeded:

   ```shell
   aws timestream-write describe-table --database-name exampleDatabase --table-name exampleTable
   ```

## Configure Prometheus Connector

Users can run the Prometheus Connector with precompiled Linux binary or Docker image. For both methods, the Prometheus Connector must have the `database-label` and `table-label` configured.
The `database-label` and `table-label` options specify the ingestion and query destination for all Prometheus metrics. The values of these options correspond to the Prometheus label names and not the Amazon Timestream databases and tables. 
For more information, see [Multi-destination Configuration](README.md#multi-destination-configuration).

### Linux Binary

1. Download the tarball containing the precompiled binary for Linux named `timestream-prometheus-connector-linux-amd64-1.1.0.tar.gz`.
2. Extract the tarball and navigate to the extracted folder by running the following commands in a terminal:
    ```shell script
    tar xvfz timestream-prometheus-connector-*.tar.gz
    cd linux
    ```
3. Run the binary with required arguments `database-label` and `table-label`.
    ```shell script
    ./timestream-prometheus-connector-linux-amd64-1.1.0 --database-label=PrometheusDatabaseLabel  --table-label=PrometheusTableLabel
    ```

   It is recommended to enable TLS encryption between Prometheus and the Prometheus Connector. To enable TLS encryption, use the following command to run the binary instead:
   ```shell
   ./timestream-prometheus-connector-linux-amd64-1.1.0 --database-label=PrometheusDatabaseLabel  --table-label=PrometheusTableLabel --tls-certificate=serverCertificate.crt --tls-key=serverPrivateKey.key
   ```
   This command assumes the TLS server certificate and the server secret key are stored in the same directory as the Prometheus Connector. 
   If the files are in a different location, specify the path to the files instead.

### Docker Image

#### Download and Install Docker
Follow the instructions for the corresponding platform to download and install Docker.
 
* **MacOS** &mdash; https://docs.docker.com/docker-for-mac/install/
* **Windows** &mdash; https://docs.docker.com/docker-for-windows/install/
* **Linux** &mdash; https://docs.docker.com/engine/install/
  
#### Download the Prometheus Connector Docker Image
1. Download the Prometheus Connector Docker image named `timestream-prometheus-connector-docker-image-1.1.0.tar.gz`.
2. Store the Docker image in a directory.

#### Load the Prometheus Connector Docker Image
1. Navigate to the directory containing the Docker image on a command-line interface.
2. Load the Docker image with the following command:
    ```shell script
    docker load < timestream-prometheus-connector-docker-image-1.1.0.tar.gz
    ```
#### Run the Prometheus Connector Docker Image
* **Linux and MacOS** &mdash; Run the Docker image with the following command:
    ```shell script
    docker run \
    -p 9201:9201 \
    timestream-prometheus-connector-docker \
    --database-label=PrometheusDatabaseLabel \
    --table-label=PrometheusTableLabel 
    ```
* **Windows** &mdash; Run the Docker image with the following command:
    ```shell script
    docker run ^
    -p 9201:9201 ^
    timestream-prometheus-connector-docker ^
    --database-label=PrometheusDatabaseLabel ^
    --table-label=PrometheusTableLabel 
    ```
  
The command does the following:
1. Publish port 9201 in the Docker container to port 9201 in the Docker host. This allows services outside of the Docker container to access the connector running on port 9201 in the Docker container.
2. Run the docker image named `timestream-prometheus-connector-docker` with required configuration options `database-label` and `table-label`.

It is recommended to enable TLS encryption between Prometheus and the Prometheus Connector. To enable TLS encryption, use the following command to run the Docker image:

   - **Linux and MacOS**

     ```shell
     docker run \
     -v $HOME/tls:/root/tls:ro \
     -p 9201:9201 \
     timestream-prometheus-connector-docker \
     --database-label=PrometheusDatabaseLabel \
     --table-label=PrometheusTableLabel \
     --tls-certificate=/root/tls/serverCertificate.crt \
     --tls-key=/root/tls/serverPrivateKey.key
     ```

   - **Windows**

     ```shell
     docker run ^
     -v "%USERPROFILE%/tls:/root/tls/:ro" ^
     -p 9201:9201 ^
     timestream-prometheus-connector-docker ^
     --database-label=PrometheusDatabaseLabel ^
     --table-label=PrometheusTableLabel ^
     --tls-certificate=/root/tls/serverCertificate.crt ^
     --tls-key=/root/tls/serverPrivateKey.key
     ```
     
   This command:
   1. Assumes the server certificate and server private key are stored in the `$HOME/tls` on Linux and MacOS or `%USERPROFILE%/tls` on Windows, but are mounted to `/root/tls` on the Docker container;
   2. Mounts the volume containing the server certificate and the server private key to a volume on the Docker container, then specify the path to the certificate and the key through the `tls-certificate` and `tls-key` configuration options. Note that the path specified must be with respect to the Docker container.

## Configure Prometheus

1. Download the appropriate tarball containing precompiled binary for Prometheus from their official [website](https://prometheus.io/download/).

2. Extract the tarball with the following command:

   ```bash
   tar xvfz prometheus-*.tar.gz
   cd prometheus-*
   ```

   Keep the command-line interface open.

3. The extracted tarball contains a precompiled binary called `prometheus` or `prometheus.exe` and a configuration file `prometheus.yml`.

4. Add the following configuration to the end of `prometheus.yml`:

   ```
   remote_write:
     - url: "http://localhost:9201/write"
   
       queue_config:
         max_samples_per_send: 100
      
       # Replace the values for username and password with valid IAM user access key and IAM user secret access key.
       basic_auth:
         username: accessKey
         password: secretAccessKey
   
       write_relabel_configs:
         # Configure ingestion destination.
         - source_labels: ["__name__"]
           regex: .*
           replacement: exampleDatabase
           target_label: PrometheusDatabaseLabel
         - source_labels: ["__name__"]
           regex: .*
           replacement: exampleTable
           target_label: PrometheusTableLabel
   ```
   
   > **NOTE**: Each Prometheus request must be authorized. Since the Prometheus Connector does not support temporary security credentials, it is recommended to use regularly [rotate IAM user access keys](https://docs.aws.amazon.com/IAM/latest/UserGuide/id_credentials_access-keys.html#Using_RotateAccessKey).
   
   This configuration serves the following functions:

   1. Configures Prometheus' remote storage destinations by setting the `url` options to the remote write endpoints, e.g. `"http://localhost:9201/write"`.

   2. Configures the Amazon Timestream ingestion destination for Prometheus time series by attaching a label indicating the destination database and another label indicating the destination table for all time series. **These labels are required to be present on all Prometheus time series sent to the Prometheus Connector.** If one of the labels cannot be found on any of the time series, the Prometheus Connector will log the error and halt the program. 

   For an example of a complete Prometheus YAML file, see [getting_started.yml](./documentation/example/getting_started.yml).

5. It is recommended to configure TLS encryption between Prometheus and the Prometheus Connector. To do so, use the `tls_config` section to specify the path to a certificate authority file, see an example in [Configure TLS Encryption in Prometheus](#configure-tls-encryption-in-prometheus).

6. Back to the command-line interface, run the precompiled binary for Prometheus with the following command:

   1. **Windows** &mdash; `prometheus --config.file=prometheus.yml`
   2. **Linux and MacOS** &mdash; `./prometheus --config.file=prometheus.yml`
   
### Configure TLS Encryption in Prometheus

It is recommended to secure the Prometheus requests with TLS encryption. This can be achieved by specifying the certificate authority file the `tls_config` section for Prometheus' remote read and remote write configuration. To generate self-signed certificates during development see the [Creating Self-signed TLS Certificates](#creating-self-signed-tls-certificates) section.

Here is an example of `remote_write` configuration with TLS, where `RootCA.pem` is within the same directory as the Prometheus configuration file:

```yaml
remote_write:
  - url: "https://localhost:9201/write"
  
   tls_config:
      # Ensure ca_file is a valid file path pointing to the CA certificate.
      ca_file: RootCA.pem
  
   basic_auth:
      # Replace the values for username and password with valid IAM user access key and IAM user secret access key.
      username: accessKey
      password: secretAccessKey
  
   write_relabel_configs:
      # Configure ingestion destination.
      - source_labels: ["__name__"]
        regex: .*
        replacement: exampleDatabase
        target_label: PrometheusDatabaseLabel
      - source_labels: ["__name__"]
        regex: .*
        replacement: exampleTable
        target_label: PrometheusTableLabel
```

## Verification

1. To verify Prometheus is running, open `http://localhost:9090/` in a browser, this opens Prometheus' [expression browser](https://prometheus.io/docs/visualization/browser/#expression-browser).

2. To verify the Prometheus Connector is ready to receive requests, ensure the following log message is printed. See the [Troubleshooting](#troubleshooting) section for other error messages.

   ```log
   level=info ts=2020-11-21T01:06:49.188Z caller=utils.go:33 message="Successfully created Timestream clients to handle read and write requests from Prometheus."
   ```

3. To verify the Prometheus Connector is ingesting data, use the AWS CLI to execute the following query:

    ```shell
    aws timestream-query query --query-string "SELECT count() FROM exampleDatabase.exampleTable"
    ```
    
    The output should look similar to the following:
    
    ```json
    {
        "Rows": [
            {
                "Data": [
                    {
                        "ScalarValue": "340"
                    }
                ]
            }
        ],
        "ColumnInfo": [
            {
                "Name": "_col0",
                "Type": {
                    "ScalarType": "BIGINT"
                }
            }
        ],
        "QueryId": "AEBQEAMYNBGX7RA"
    }
    ```
    
    This sample output indicates that 340 rows has been ingested.

## Troubleshooting

1. No Credential Providers Error
    
    Error occurred when running the Linux binary with the following message:
    ```log
    level=error ts=2020-11-21T00:22:06.203Z caller=utils.go:23 message="Unable to create a write client." error="NoCredentialProviders: no valid providers in chain. Deprecated.\n\tFor verbose messaging see aws.Config.CredentialsChainVerboseErrors"
    ```
    This error may occur when no AWS credentials can be found. Follow the steps in [Configure AWS Credentials](#configure-aws-credentials) to set up the credentials.

2. Access Denied Exception
    
    Error occurred when running the Linux binary with the following message:
    ```log
    level=error ts=2020-11-23T19:58:49.998Z caller=utils.go:23 message="Unable to create a write client." error="AccessDeniedException: User: arn:aws:iam::0000000000:user/username is not authorized to perform: timestream:DescribeEndpoints with an explicit deny"
    ```
    1. Ensure the account running the Prometheus Connector has sufficient permissions to access Timestream. See all the IAM Policies for Timestream on [How Amazon Timestream Works with IAM](https://docs.aws.amazon.com/timestream/latest/developerguide/security_iam_service-with-iam.html).
    
3. Conflicting Resources Error
    
    Error occurred when running the Docker image with the following message:
    ```log
    docker: Error response from daemon: driver failed programming external connectivity on endpoint silly_proskuriakova 
    (1823ad1d6139911298536cdab0b08b38981d83cb124ad971e2b944a51c272438): Bind for 0.0.0.0:9201 failed: port is already allocated.
    ```
    The port number is not available because it is in use. 
    
    If the resource occupying this port cannot be freed:
    1. Run the connector with a custom listen-address with the ` --web.listen-address` option and with the updated `-p` flag to publish the custom port. An example running the Docker container on port 3080 is as follows:
        ```shell script
        docker run \
        -p 3080:3080 \
        timestream-prometheus-connector-docker \
        --database-label=PrometheusDatabaseLabel \ 
        --table-label=PrometheusTableLabel \
        --web.listen-address=:3080
        ``` 
    If the port is used by a Docker container that could be removed:
    1. Use [docker rm](https://docs.docker.com/engine/reference/commandline/rm/) to remove the container.
    
4. Invalid Mount Path Error

   Error occurred when running the Docker image with the following message:
   %USERPROFILE%/tls:/root/tls/:ro
   ```log
   docker: Error response from daemon: invalid volume specification: '/host_mnt/c/Users/<user_name>/tls: /root/tls/:ro': 
   invalid mount config for type "bind": invalid mount path: ' /root/tls/:ro' mount path must be absolute.
   ```
   Ensure there are no extra spaces when setting the `-v` flag. See more details regarding the `-v` flag in Docker's [documentation](https://docs.docker.com/storage/volumes/#choose-the--v-or---mount-flag).
   
   Invalid example: 
   ```
   -v "%USERPROFILE%/tls: /root/tls/:ro"
   ```
   Valid example: 
   ```
   -v "%USERPROFILE%/tls:/root/tls/:ro"
   ```

## License

This getting started guide is licensed under the Apache 2.0 License.

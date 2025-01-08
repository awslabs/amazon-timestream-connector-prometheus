# Correctness Testing for Prometheus Connector

## Prerequisites

1. **Configure AWS Credentials**

   Ensure your AWS credentials are configured for your environment. You can set them up using:
   ```bash
   aws configure
   ```

   **Note:** MFA credentials are not supported.

2. **Create a New Timestream Database and Table**

   Execute the following command to create a new Timestream database and table:
   ```bash
   aws timestream-write create-database --database-name CorrectnessDB --region <aws-region> && \
   aws timestream-write create-table --database-name CorrectnessDB --table-name CorrectnessMetrics --region <aws-region>
   ```

## Run Correctness Tests

1. **Start the Prometheus Connector**

   Bring up the Prometheus Connector using the following command:
   ```bash
   DEFAULT_DATABASE=CorrectnessDB DEFAULT_TABLE=CorrectnessMetrics AWS_REGION=<aws-region> docker compose -f ../docker-compose.yml up -d
   ```

2. **Execute Tests**

   Run the tests with:
   ```bash
   go test -v
   ```

   *Note:* Tests typically take between 15 to 20 seconds to complete.

## Flags

The correctness test suite (`correctness_test.go`) accepts several flags to customize its behavior during correctness testing. Below is a list of available flags along with their descriptions and default values:

| **Flag** | **Description** | **Default Value** |
|----------|----------------|-------------------|
| `freshTSDB` | Indicates whether the tests should expect a clean database state. Set to `true` for a fresh database, `false` for an existing database with data. | `true` |
| `ingestionWaitTime` | Sets the wait time (in seconds) after data ingestion to allow for data consistency before tests are evaluated. | `1s` |

For example, to run against an existing Timestream database and table:

   ```bash
   go test -v -freshTSDB=false
   ```

## Clean Up

1. **Delete the Timestream Database and Table**

   Remove your newly created Timestream database and table using:
   ```bash
   aws timestream-write delete-table --database-name CorrectnessDB --table-name CorrectnessMetrics --region <aws-region> && \
   aws timestream-write delete-database --database-name CorrectnessDB --region <aws-region>
   ```

2. **Stop the Prometheus Connector**

   Bring down the Connector with:
   ```bash
   docker compose -f ../docker-compose.yml down
   ```

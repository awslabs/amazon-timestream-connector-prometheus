# Correctness Testing for Prometheus Connector

## Prerequisites
1. Ensure your AWS credentials are configured for your environment. You can configure it with `aws configure`.

2. Create a new Timestream database and table:
```
aws timestream-write create-database --database-name CorrectnessDB --region us-west-2 && aws timestream-write create-table --database-name CorrectnessDB --table-name CorrectnessMetrics
```

## Run Correctness Tests
1. Update the default database and table in [`docker-compose.yml`](https://github.com/awslabs/amazon-timestream-connector-prometheus/blob/main/docker-compose.yml) with your new Timestream database and table.

2. Bring up the Prometheus Connector:
```
`docker compose -f ../docker-compose.yml up -d`
```

3. Run `go test -v`. Tests can take between 15~20 seconds to complete.

### Tips

- To run correctness tests against an existing Timestream database, ensure the `freshTSDB` flag is disabled in the test suite.
- The `ingestionWaitTime` parameter is adjustable to account for network latency, ensuring data ingestion is complete before evaluating test outputs.

## Clean up
1. Delete your Timestream database:

```
aws timestream-write delete-table --database-name CorrDB --table-name CorrMetrics --region us-west-2 && aws timestream-write delete-database --database-name CorrDB
````

2. Bring down the Connector with:
```
docker compose -f ../docker-compose.yml down
```

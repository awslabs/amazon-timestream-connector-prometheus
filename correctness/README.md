# Correctness Testing for Prometheus Connector

## Prerequisites
Prior to running the tests in correctness_test.go, ensure the following:
1. Have a database called correctness_testing with the table named correctness_testing created.
2. Ingested data to the correctness_testing table for at least an hour.
3. Updated the basic_auth section within [correctness_testing.yml](./config/correctness_testing.yml).
4. Download the Prometheus Connector Docker image and store it in the `resources` directory in the repository root.
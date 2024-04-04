# Correctness Testing for Prometheus Connector

## Prerequisites
Prior to running the tests in correctness_test.go, ensure the following:
1. Have a database called correctness_testing with the table named correctness_testing created.
2. Ingested data to the correctness_testing table for at least an hour.
3. Updated the basic_auth section within [correctness_testing.yml](./config/correctness_testing.yml).
2. Download or build the Prometheus Connector Docker image and store it in a new directory named `resources` in the repository root.

## How to build and save the docker image
1. Execute the following command to build the docker image:
`docker buildx build . -t timestream-prometheus-connector-docker`
2. Execute the following command to save the docker image as a compressed file and update the `version` appropriately:
`docker save timestream-prometheus-connector-docker | gzip > timestream-prometheus-connector-docker-image-<version>.tar.gz`

## How to execute tests
1. Set the environment variable `PROMETHEUS_CONNECTOR_VERSION` to the version of the docker image tarball which is stored in the `resources` directory in the repository root.
e.g. `export PROMETHEUS_CONNECTOR_VERSION="1.0.0"`
2. Run the following command to execute the TLS tests:
`go test -v ./correctness`

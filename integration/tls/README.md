# TLS Encryption Integration Test for Prometheus Connector

## Prerequisites
Prior to running the tests in tls_test.go, ensure the following:
1. Updated the basic_auth section within [prometheus.yml](./config/prometheus.yml).
2. Download or build the Prometheus Connector Docker image and store it in a new directory named `resources` in the repository root.

## How to build and save the docker image
1. Execute the following command to build the docker image:
`docker build . -t timestream-prometheus-connector-docker`
2. Execute the following command to save the docker image as a compressed file and update the `version` appropriately:
`docker save timestream-prometheus-connector-docker | gzip > timestream-prometheus-connector-docker-image-<version>.tar.gz`

## How to execute tests
1. Run the following command to execute the TLS tests:
`go test -v ./integration/tls`

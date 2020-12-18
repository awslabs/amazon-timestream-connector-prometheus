# TLS Encryption Integration Test for Prometheus Connector

## Prerequisites
Prior to running the tests in tls_test.go, ensure the following:
1. Updated the basic_auth section within [prometheus.yml](./config/prometheus.yml).
2. Download the Prometheus Connector Docker image and store it in the `resources` directory in the repository root.
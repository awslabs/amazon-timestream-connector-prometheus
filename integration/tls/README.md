# TLS Encryption Integration Test for Prometheus Connector

## Prerequisites
Prior to running the tests in tls_test.go, ensure the following:
1. Updated the basic_auth section within [prometheus.yml](./config/prometheus.yml).
2. Complete the [Creating Self-signed TLS Certificates](#creating-self-signed-tls-certificates) if this has not been previously done and store the `RootCA.pem`, `ServerCertificate.crt`, `ServerPrivateKey.key`, and `InvalidPrivateKey.key` files in the `integration/tls/cert` folder.
3. Download the [latest release artifact](https://github.com/awslabs/amazon-timestream-connector-prometheus/releases/latest) or build the Prometheus Connector Docker image and store it in a new directory named `resources` in the repository root.

## How to build and save the docker image
1. Execute the following command to build the docker image:
`docker buildx build . -t timestream-prometheus-connector-docker`
2. Execute the following command to save the docker image as a compressed file and update the `version` appropriately:
`docker save timestream-prometheus-connector-docker | gzip > timestream-prometheus-connector-docker-image-<version>.tar.gz`

## Creating Self-signed TLS Certificates

The following steps generate self-signed TLS certificates using OpenSSL.

> **NOTE**: Self-signed certificates **should not** be used during production, they should only be used during development.

### Creating the Certificate Authority files

Use the following command to generate a private key and the root certificate file for the certificate authority.

```shell
openssl req -x509 -nodes -new -sha256 -days 1024 -newkey rsa:2048 -keyout RootCA.key -out RootCA.pem -subj "/C=US/ST=Washington/L=Seattle/O=Amazon Web Services/CN=host.docker.internal"
```

### Creating the Server Key

Use the following command to generate a server private key and a certificate signing request:

```shell
openssl req -days 365 -nodes -newkey rsa:2048 -keyout ServerPrivateKey.key -out ServerCertificateSigningRequest.csr -subj "/C=US/ST=Washington/L=Seattle/O=Amazon Web Services/CN=host.docker.internal"
```

### Creating the Server Certificate

Use the following command to generate the self-signed server certificate:

```shell
openssl x509 -req -sha256 -days 365 -in ServerCertificateSigningRequest.csr -CA RootCA.pem -CAkey RootCA.key -CAcreateserial -extfile <(printf "subjectAltName=DNS:host.docker.internal") -out ServerCertificate.crt
```
> **NOTE**: The value for DNS is set to **DNS:host.docker.internal** to associate the host name to the server certificate. This is required when running the Prometheus Connector from a Docker image or from the precompiled binaries.

### Creating the Invalid Private Key

Use the following command to generate the invalid private key:

```shell
openssl req -x509 -nodes -new -sha256 -days 365 -newkey rsa:2048 -keyout InvalidPrivateKey.key -subj "/C=US/ST=Washington/L=Seattle/O=Invalid Organization/CN=Invalid-CN"
```

## How to execute tests
1. Run the following command to execute the TLS tests:
`go test -v ./integration/tls`

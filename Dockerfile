# Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
# the License. A copy of the License is located at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
# CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
# and limitations under the License.

# Stage 1: Build the binary
FROM amazonlinux:latest AS build_stage
RUN yum install -y go

# Set Docker image metadata
LABEL name="timestream/timestream-prometheus-connector" \
      summary="Amazon Timestream Prometheus Connector" \
      description="This Prometheus connector receives and sends samples between Prometheus and Timestream through Prometheus' remote write and remote read protocols."

WORKDIR /tmp/timestream/

# Copy go.mod and go.sum and download all required dependencies
COPY go.mod .
COPY go.sum .
RUN go mod download

COPY . .

# Run unit tests for main.go and client.go
RUN CGO_ENABLED=0 go test -tags=unit -cover -v ./timestream ./

# Build the binary for Linux.
RUN CGO_ENABLED=0 GOOS=linux go build -o ./timestream-prometheus-connector .

# Stage 2: Copy the pre-compiled Linux binary to the final image
FROM amazonlinux:latest AS copy_stage
RUN yum install -y ca-certificates

COPY --from=build_stage /tmp/timestream/timestream-prometheus-connector /app/timestream/timestream-prometheus-connector

# Expose service endpoint
EXPOSE 9201

# Run the container
ENTRYPOINT ["./app/timestream/timestream-prometheus-connector"]

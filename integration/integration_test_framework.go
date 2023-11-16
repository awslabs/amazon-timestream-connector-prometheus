/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file provides integration test framework to start the Prometheus and the Prometheus Connector in Docker for tls_test.go and correctness_test.go.
package integration

import (
	"context"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/timestreamwrite"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"
)

type PrometheusContainerConfig struct {
	DockerImage string
	ConfigPath  string
	ImageName   string
	Binds       []string
}

type ConnectorContainerConfig struct {
	DockerImage       string
	ImageName         string
	Binds             []string
	ConnectorCommands []string
}

// CreateDockerClient creates a Docker client that runs in the background.
func CreateDockerClient(t *testing.T) (*client.Client, context.Context) {
	ctx := context.Background()
	cli, err := client.NewClientWithOpts()
	require.NoError(t, err)

	return cli, ctx
}

// StartPrometheus starts the Prometheus server in a Docker container.
func StartPrometheus(t *testing.T, cli *client.Client, ctx context.Context, config PrometheusContainerConfig) string {
	out, err := cli.ImagePull(ctx, config.DockerImage, types.ImagePullOptions{})
	require.NoError(t, err)
	// Output the pull process.
	_, err = io.Copy(os.Stdout, out)
	require.NoError(t, err)
	defer out.Close()

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: config.ImageName,
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"9090/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "9090",
				},
			},
		},
		Binds: config.Binds,
	}, nil, nil, "")

	require.NoError(t, err)
	assert.Nil(t, cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))

	return resp.ID
}

// StartConnector starts the connector in a Docker container.
func StartConnector(t *testing.T, cli *client.Client, ctx context.Context, config ConnectorContainerConfig) string {
	image, err := os.Open(config.DockerImage)
	require.NoError(t, err)

	_, err = cli.ImageLoad(ctx, image, true)
	require.NoError(t, err)

	hostConfig := &container.HostConfig{
		PortBindings: nat.PortMap{
			"9201/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "9201",
				},
			},
		},
		Binds: config.Binds,
	}

	if config.Binds != nil {
		hostConfig.Binds = config.Binds
	}

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: config.ImageName,
		Cmd:   config.ConnectorCommands,
	}, hostConfig, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))

	return resp.ID
}

// StopContainer stops and removes all containers matching the given slice of containerIDs.
func StopContainer(t *testing.T, cli *client.Client, ctx context.Context, containerIDs []string) {
	for i := range containerIDs {
		assert.Nil(t, cli.ContainerStop(ctx, containerIDs[i], container.StopOptions{}))
		assert.Nil(t, cli.ContainerRemove(ctx, containerIDs[i], types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}))
	}
}

// Setup creates new databases and tables for integration tests.
func Setup(writeClient *timestreamwrite.TimestreamWrite, destinations map[string][]string) error {
	for database, tables := range destinations {
		databaseName := aws.String(database)
		for _, table := range tables {
			tableName := aws.String(table)
			if _, err := writeClient.DescribeTable(&timestreamwrite.DescribeTableInput{DatabaseName: databaseName, TableName: tableName}); err == nil {
				if _, err = writeClient.DeleteTable(&timestreamwrite.DeleteTableInput{DatabaseName: databaseName, TableName: tableName}); err != nil {
					return err
				}
			}
		}
		if _, err := writeClient.DescribeDatabase(&timestreamwrite.DescribeDatabaseInput{DatabaseName: databaseName}); err == nil {
			if _, err = writeClient.DeleteDatabase(&timestreamwrite.DeleteDatabaseInput{DatabaseName: databaseName}); err != nil {
				return err
			}
		}

		if _, err := writeClient.CreateDatabase(&timestreamwrite.CreateDatabaseInput{DatabaseName: databaseName}); err != nil {
			return err
		}
		for _, table := range tables {
			if _, err := writeClient.CreateTable(&timestreamwrite.CreateTableInput{DatabaseName: databaseName, TableName: aws.String(table)}); err != nil {
				return err
			}
		}
	}
	return nil
}

// Shutdown removes the databases and tables created for integration tests.
func Shutdown(writeClient *timestreamwrite.TimestreamWrite, destinations map[string][]string) error {
	for database, tables := range destinations {
		databaseName := aws.String(database)
		for _, table := range tables {
			if _, err := writeClient.DeleteTable(&timestreamwrite.DeleteTableInput{DatabaseName: databaseName, TableName: aws.String(table)}); err != nil {
				return err
			}
		}
		if _, err := writeClient.DeleteDatabase(&timestreamwrite.DeleteDatabaseInput{DatabaseName: databaseName}); err != nil {
			return err
		}
	}
	return nil
}

// CreateHTTPClient creates a HTTP client to send requests.
func CreateHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
}

// CreateReadRequest creates a read request.
func CreateReadRequest(t *testing.T, query string, now time.Time, prevHour time.Time) *http.Request {
	req, err := http.NewRequest("GET", "http://localhost:9090/api/v1/query", nil)
	require.Nil(t, err)
	req.Close = true

	q := req.URL.Query()
	q.Add("query", query)
	q.Add("time", strconv.FormatInt(now.Unix(), 10))
	q.Add("_", strconv.FormatInt(prevHour.Unix(), 10))
	req.URL.RawQuery = q.Encode()

	return req
}

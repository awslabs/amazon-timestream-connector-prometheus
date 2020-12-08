/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

package correctness

import (
	"bufio"
	"context"
	"encoding/csv"
	"fmt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/docker/docker/client"
)

const (
	expectedOutputFilePath    = "data/expectedOutput.csv"
	prometheusDockerImage     = "docker.io/prom/prometheus"
	prometheusConfigPath      = "config/correctness_testing.yml"
	prometheusDockerImageName = "prom/prometheus"
	connectorDockerImageName  = "timestream-prometheus-connector-docker"
	connectorDockerImagePath  = "timestream-prometheus-connector-docker-image.tar.gz"
)

var (
	containerIDs  []string
	connectorCMDs = []string{"--database-label=timestreamDatabase", "--table-label=timestreamTable", "--log.level=debug"}
	headers       = []string{"PromQL", "Read Response"}
)

func TestQueries(t *testing.T) {
	files, err := filepath.Glob("data/*.txt")
	require.NoError(t, err)

	var promQL []string
	for _, file := range files {
		promQL = loadPromQLFromFile(t, file, promQL)
	}

	dockerClient, ctx := createDockerClient(t)
	containerIDs = append(containerIDs, startConnector(t, dockerClient, ctx))
	containerIDs = append(containerIDs, startPrometheus(t, dockerClient, ctx))

	output, err := sendRequest(promQL)
	errorCheck(t, err, dockerClient, ctx)
	err = writeToFile(output)
	errorCheck(t, err, dockerClient, ctx)

	stopContainer(t, dockerClient, ctx, containerIDs)
}

// loadPromQLFromFile loads the PromQL for correctness testing from the specified filepath.
func loadPromQLFromFile(t *testing.T, filePath string, queries []string) []string {
	file, err := os.Open(filePath)
	require.NoError(t, err)

	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		text := scanner.Text()
		if len(text) != 0 {
			queries = append(queries, scanner.Text())
		}
	}

	assert.Nil(t, scanner.Err())
	return queries
}

// createDockerClient creates a Docker client that runs in the background.
func createDockerClient(t *testing.T) (*client.Client, context.Context) {
	ctx := context.Background()
	cli, err := client.NewEnvClient()
	require.NoError(t, err)

	return cli, ctx
}

// startPrometheus starts the Prometheus server in a Docker container.
func startPrometheus(t *testing.T, cli *client.Client, ctx context.Context) string {
	out, err := cli.ImagePull(ctx, prometheusDockerImage, types.ImagePullOptions{})
	require.NoError(t, err)
	// Output the pull process.
	_, err = io.Copy(os.Stdout, out)
	require.NoError(t, err)
	defer out.Close()

	absPath, err := filepath.Abs(prometheusConfigPath)
	require.NoError(t, err)

	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: prometheusDockerImageName,
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"9090/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "9090",
				},
			},
		},
		Binds: []string{fmt.Sprintf("%s:/etc/prometheus/prometheus.yml", absPath)},
	}, nil, "")

	require.NoError(t, err)
	assert.Nil(t, cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))

	return resp.ID
}

// startConnector starts the connector in a Docker container.
func startConnector(t *testing.T, cli *client.Client, ctx context.Context) string {
	image, err := os.Open(connectorDockerImagePath)
	require.NoError(t, err)

	_, err = cli.ImageLoad(ctx, image, true)
	require.NoError(t, err)

	env := "HOME"
	if runtime.GOOS == "windows" {
		env = "USERPROFILE"
	}
	homePath := os.Getenv(env)
	resp, err := cli.ContainerCreate(ctx, &container.Config{
		Image: connectorDockerImageName,
		Cmd:   connectorCMDs,
	}, &container.HostConfig{
		PortBindings: nat.PortMap{
			"9201/tcp": []nat.PortBinding{
				{
					HostIP:   "0.0.0.0",
					HostPort: "9201",
				},
			},
		},
		Binds: []string{fmt.Sprintf("%s/.aws/credentials:/root/.aws/credentials:ro", homePath)},
	}, nil, "")

	require.NoError(t, err)
	assert.Nil(t, cli.ContainerStart(ctx, resp.ID, types.ContainerStartOptions{}))

	return resp.ID
}

// stopContainer stops and removes all containers matching the given slice of containerIDs.
func stopContainer(t *testing.T, cli *client.Client, ctx context.Context, containerIDs []string) {
	for i := range containerIDs {
		assert.Nil(t, cli.ContainerStop(ctx, containerIDs[i], nil))
		assert.Nil(t, cli.ContainerRemove(ctx, containerIDs[i], types.ContainerRemoveOptions{RemoveVolumes: true, Force: true}))
	}
}

// sendRequest executes the given slice of PromQL through the Prometheus HTTP API.
func sendRequest(queries []string) ([][]string, error) {
	output := [][]string{headers}

	httpClient := &http.Client{
		Transport: &http.Transport{
			DialContext: (&net.Dialer{
				Timeout: 3 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
		},
	}
	now := time.Now()
	prevHour := now.Add(time.Duration(-1) * time.Hour)

	for i := range queries {
		query := queries[i]
		retries := 0
		req, _ := http.NewRequest("GET", "http://localhost:9090/api/v1/query", nil)
		req.Close = true

		q := req.URL.Query()

		q.Add("query", query)
		q.Add("time", strconv.FormatInt(now.Unix(), 10))
		q.Add("_", strconv.FormatInt(prevHour.Unix(), 10))
		req.URL.RawQuery = q.Encode()

		// Requests will fail while the Prometheus server is still setting up, retry until Prometheus server is ready to receive web requests.
		for retries <= 10 {
			resp, err := httpClient.Do(req)
			if err == nil {
				body, err := ioutil.ReadAll(resp.Body)
				if err != nil {
					return output, err
				}
				output = append(output, []string{query, string(body)})
				break
			}

			time.Sleep(5 * time.Second)
			retries++
		}
	}

	return output, nil
}

// errorCheck stops and removes the Docker container if an error has occurred.
func errorCheck(t *testing.T, err error, dockerClient *client.Client, ctx context.Context) {
	if err != nil {
		stopContainer(t, dockerClient, ctx, containerIDs)
		t.Fail()
	}
}

// writeToFile writes the PromQL executed and the prompb.ReadResponse to a CSV file.
func writeToFile(output [][]string) error {
	file, err := os.OpenFile(expectedOutputFilePath, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		return err
	}

	csvWriter := csv.NewWriter(file)
	defer csvWriter.Flush()

	if err := csvWriter.WriteAll(output); err != nil {
		file.Close()
		return err
	}

	file.Close()
	return nil
}

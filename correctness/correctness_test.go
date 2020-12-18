/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This package executes all types of PromQLs Prometheus may send to the Prometheus Connector, which will output the
// responses for the PromQLs to expectedOutput.csv for manual verification. The intent of this test is to ensure the
// Prometheus Connector can properly translate PromQLs to Amazon Timestream SQLs.
//
// Prior to running the tests in this file, ensure valid IAM credentials are specified in the basic auth section within
// config/prometheus.yml.
package correctness

import (
	"bufio"
	"context"
	"encoding/csv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
	"timestream-prometheus-connector/integration"

	"github.com/docker/docker/client"
)

const (
	expectedOutputFilePath    = "data/expectedOutput.csv"
	prometheusDockerImage     = "docker.io/prom/prometheus"
	prometheusConfigPath      = "config/correctness_testing.yml"
	prometheusDockerImageName = "prom/prometheus"
	connectorDockerImageName  = "timestream-prometheus-connector-docker"
	connectorDockerImagePath  = "../resources/timestream-prometheus-connector-docker-image-1.1.0.tar.gz"
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

	dockerClient, ctx := integration.CreateDockerClient(t)

	connectorConfig := integration.ConnectorContainerConfig{
		DockerImage:       connectorDockerImagePath,
		ImageName:         connectorDockerImageName,
		ConnectorCommands: connectorCMDs,
	}

	containerIDs = append(containerIDs, integration.StartConnector(t, dockerClient, ctx, connectorConfig))

	prometheusConfig := integration.PrometheusContainerConfig{
		DockerImage: prometheusDockerImage,
		ImageName:   prometheusDockerImageName,
		ConfigPath:  prometheusConfigPath,
	}
	containerIDs = append(containerIDs, integration.StartPrometheus(t, dockerClient, ctx, prometheusConfig))

	output, err := sendRequest(t, promQL)
	errorCheck(t, err, dockerClient, ctx)
	err = writeToFile(output)
	errorCheck(t, err, dockerClient, ctx)

	integration.StopContainer(t, dockerClient, ctx, containerIDs)
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

// sendRequest executes the given slice of PromQL through the Prometheus HTTP API.
func sendRequest(t *testing.T, queries []string) ([][]string, error) {
	output := [][]string{headers}

	httpClient := integration.CreateHTTPClient()
	now := time.Now()
	prevHour := now.Add(time.Duration(-1) * time.Hour)

	for i := range queries {
		query := queries[i]
		req := integration.CreateReadRequest(t, query, now, prevHour)

		// Requests will fail while the Prometheus server is still setting up, retry until Prometheus server is ready to receive web requests.
		retries := 0
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
		integration.StopContainer(t, dockerClient, ctx, containerIDs)
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

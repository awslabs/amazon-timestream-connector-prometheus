/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file contains integration tests for HTTPS support with TLS encryption.
// Prior to running the tests in this file, ensure valid IAM credentials are specified in the basic auth section within
// config/prometheus.yml.
package tls

import (
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/timestreamquery"
	"github.com/aws/aws-sdk-go/service/timestreamwrite"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/net/context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"timestream-prometheus-connector/integration"
)

const (
	prometheusDockerImage      = "docker.io/prom/prometheus"
	prometheusConfigPath       = "config/prometheus.yml"
	prometheusDockerImageName  = "prom/prometheus"
	tlsRootCAPath              = "cert/RootCA.pem"
	tlsServerCertPath          = "cert"
	connectorDockerImageName   = "timestream-prometheus-connector-docker"
	connectorDockerImagePath   = "../../resources/timestream-prometheus-connector-docker-image-1.1.0.tar.gz"
	databaseLabelCMD           = "--database-label=timestreamDatabase"
	tableLabelCMd              = "--table-label=timestreamTable"
	defaultDatabaseCMD         = "--default-database=PrometheusDatabase"
	defaultTableCMd            = "--default-table=PrometheusTable"
	tlsCertificateCMD          = "--tls-certificate=/root/cert/serverCertificate.crt"
	tlsKeyCMD                  = "--tls-key=/root/cert/serverPrivateKey.key"
	tlsUnmatchedCertificateCMD = "--tls-certificate=/root/cert/rootCA.pem"
	tlsInvalidKeyFileCMD       = "--tls-key=/root/cert/invalidPrivateKey.key"
	region                     = "us-east-1"
	databaseLabel              = "timestreamDatabase"
	tableLabel                 = "timestreamTable"
	database                   = "tlsDB"
	table                      = "tls"
	retries                    = 6
	expectedStatusCode         = 200
)

var (
	connectorTLSCMDs                      = []string{databaseLabelCMD, tableLabelCMd, defaultDatabaseCMD, defaultTableCMd, tlsCertificateCMD, tlsKeyCMD}
	connectorCMDsWithUnmatchedCertificate = []string{databaseLabelCMD, tableLabelCMd, defaultDatabaseCMD, defaultTableCMd, tlsUnmatchedCertificateCMD, tlsKeyCMD}
	connectorCMDsWithUnmatchedKey         = []string{databaseLabelCMD, tableLabelCMd, defaultDatabaseCMD, defaultTableCMd, tlsCertificateCMD, tlsInvalidKeyFileCMD}
	connectorCMDsWithInvalidFile          = []string{databaseLabelCMD, tableLabelCMd, defaultDatabaseCMD, defaultTableCMd, tlsKeyCMD, tlsKeyCMD}
	destinations                          = map[string][]string{database: {table}}
)

func TestMain(m *testing.M) {
	testSession := session.Must(session.NewSession())
	writeClient := timestreamwrite.New(testSession, aws.NewConfig().WithRegion(region))
	if err := integration.Setup(writeClient, destinations); err != nil {
		panic(err)
	}
	code := m.Run()
	if err := integration.Shutdown(writeClient, destinations); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestHttpsSupport(t *testing.T) {
	dockerClient, ctx := integration.CreateDockerClient(t)

	bindString := []string{fmt.Sprintf("%s:/root/cert:ro", getAbsolutionPath(t, tlsServerCertPath))}

	connectorConfig := integration.ConnectorContainerConfig{
		DockerImage:       connectorDockerImagePath,
		ImageName:         connectorDockerImageName,
		Binds:             bindString,
		ConnectorCommands: connectorTLSCMDs,
	}

	var containerIDs []string
	respID := integration.StartConnector(t, dockerClient, ctx, connectorConfig)
	containerIDs = append(containerIDs, respID)

	prometheusBindString := []string{
		fmt.Sprintf("%s:/etc/prometheus/prometheus.yml", getAbsolutionPath(t, prometheusConfigPath)),
		fmt.Sprintf("%s:/etc/prometheus/RootCA.pem:ro", getAbsolutionPath(t, tlsRootCAPath))}
	prometheusConfig := integration.PrometheusContainerConfig{
		DockerImage: prometheusDockerImage,
		ImageName:   prometheusDockerImageName,
		ConfigPath:  prometheusConfigPath,
		Binds:       prometheusBindString,
	}
	containerIDs = append(containerIDs, integration.StartPrometheus(t, dockerClient, ctx, prometheusConfig))

	connectorStatusCheck(t, dockerClient, ctx, respID, 0)

	count := getDatabaseRowCount(t, database, table)
	assert.Greater(t, count, 0)

	statusCode, err := sendReadRequest(t, fmt.Sprintf("prometheus_http_requests_total{%s=\"%s\", %s=\"%s\"}", databaseLabel, database, tableLabel, table))
	require.NoError(t, err)
	assert.Equal(t, expectedStatusCode, statusCode)

	integration.StopContainer(t, dockerClient, ctx, containerIDs)
}

func TestHttpsSupportWithInvalidCertificate(t *testing.T) {
	var containerIDs []string
	type testCase []struct {
		testName string
		command  []string
	}

	invalidTestCase := testCase{
		{"test with unmatched certificate", connectorCMDsWithUnmatchedCertificate},
		{"test with unmatched key", connectorCMDsWithUnmatchedKey},
		{"test with invalid file type", connectorCMDsWithInvalidFile},
	}

	bindString := []string{fmt.Sprintf("%s:/root/cert:ro", getAbsolutionPath(t, tlsServerCertPath))}

	dockerClient, ctx := integration.CreateDockerClient(t)
	for _, test := range invalidTestCase {
		connectorConfig := integration.ConnectorContainerConfig{
			DockerImage:       connectorDockerImagePath,
			ImageName:         connectorDockerImageName,
			Binds:             bindString,
			ConnectorCommands: test.command,
		}

		t.Run(test.testName, func(t *testing.T) {
			respID := integration.StartConnector(t, dockerClient, ctx, connectorConfig)
			containerIDs = append(containerIDs, respID)
			connectorStatusCheck(t, dockerClient, ctx, respID, 1)
		})
	}

	integration.StopContainer(t, dockerClient, ctx, containerIDs)
}

// getAbsolutionPath gets the absolution path of the giving file.
func getAbsolutionPath(t *testing.T, path string) string {
	absPath, err := filepath.Abs(path)
	require.NoError(t, err)

	return absPath
}

// sendReadRequest sends a read request to Amazon Timestream.
func sendReadRequest(t *testing.T, query string) (int, error) {
	httpClient := integration.CreateHTTPClient()

	now := time.Now()
	prevHour := now.Add(time.Duration(-1) * time.Hour)
	req := integration.CreateReadRequest(t, query, now, prevHour)

	resp, err := httpClient.Do(req)
	for i := 0; i < retries; i++ {
		resp, err = httpClient.Do(req)
		if err == nil && resp != nil {
			break
		}
		time.Sleep(10 * time.Second)
	}
	assert.NotNil(t, resp)
	return resp.StatusCode, err
}

// connectorStatusCheck checks if the exit code of the Prometheus Connector response is as expected.
func connectorStatusCheck(t *testing.T, dockerClient *client.Client, ctx context.Context, respID string, expectedExitCode int) {
	var jsonRes types.ContainerJSON
	var err error

	for i := 0; i < retries; i++ {
		// Busy wait for a minute to give the containers time to send the first request.
		jsonRes, err = dockerClient.ContainerInspect(ctx, respID)
		out, _ := dockerClient.ContainerLogs(ctx, respID, types.ContainerLogsOptions{ShowStdout: true})
		_ = out
		require.NoError(t, err)
		assert.NotNil(t, jsonRes.State)
		if jsonRes.State.ExitCode == 1 {
			break
		}
		time.Sleep(10 * time.Second)
	}
	assert.Equal(t, expectedExitCode, jsonRes.State.ExitCode)
}

// getDatabaseRowCount gets the number of rows in a specific table.
func getDatabaseRowCount(t *testing.T, database string, table string) int {
	queryInput := &timestreamquery.QueryInput{QueryString: aws.String(fmt.Sprintf("SELECT count(*) from %s.%s", database, table))}

	sess, err := session.NewSession(&aws.Config{Region: aws.String(region)})
	require.NoError(t, err)
	querySvc := timestreamquery.New(sess)
	out, err := querySvc.Query(queryInput)
	require.NoError(t, err)

	count, err := strconv.Atoi(*out.Rows[0].Data[0].ScalarValue)
	require.NoError(t, err)

	return count
}

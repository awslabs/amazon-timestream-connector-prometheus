/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// These integration tests create real Amazon Timestream
// write client and send Prometheus remote write requests
// directly to the clients. These tests do not create a
// real Prometheus server nor create a local Prometheus
// Connector server.
package integration

import (
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/timestreamwrite"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"math/rand"
	"os"
	"testing"
	"time"
	"timestream-prometheus-connector/timestream"
)

var (
	logger             = log.NewNopLogger()
	nowUnix            = time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	destinations       = map[string][]string{database: {table}, database2: {table2}}
	writeClient        = timestreamwrite.New(session.Must(session.NewSession()), aws.NewConfig().WithRegion(region))
	awsCredentials     = writeClient.Config.Credentials
	emptyCredentials   = credentials.NewStaticCredentials("", "", "")
	invalidCredentials = credentials.NewStaticCredentials("accessKey", "secretKey", "")
)

func TestMain(m *testing.M) {
	if err := Setup(writeClient, destinations); err != nil {
		panic(err)
	}
	code := m.Run()
	if err := Shutdown(writeClient, destinations); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestWriteClient(t *testing.T) {
	req := &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		createTimeSeriesTemplate(database, table),
	}}

	tsLongMetric := createTimeSeriesTemplate(database, table)
	tsLongMetric.Labels[0].Value = "a_very_long_long_long_long_long_test_metric_that_will_be_over_sixty_bytes"
	reqLongMetric := &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		tsLongMetric,
	}}

	tsLongLabel := createTimeSeriesTemplate(database, table)
	tsLongLabel.Labels[3].Name = "a_very_long_long_long_long_long_label_name_that_will_be_over_sixty_bytes"
	reqLongLabel := &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		tsLongMetric,
	}}

	// This time series has multiple destinations and contains 100 samples to each destination.
	var timeSeriesBatch []*prompb.TimeSeries
	for i := 0; i < numRecords; i++ {
		timeSeriesBatch = append(timeSeriesBatch, createTimeSeriesTemplate(database2, table2))
		timeSeriesBatch = append(timeSeriesBatch, createTimeSeriesTemplate(database, table))
	}
	reqBatch := &prompb.WriteRequest{Timeseries: timeSeriesBatch}

	timeSeriesBatchFail := append(timeSeriesBatch, createTimeSeriesTemplate(database2, table2))
	timeSeriesBatchFail = append(timeSeriesBatchFail, createTimeSeriesTemplate(database, table))
	reqBatchFail := &prompb.WriteRequest{Timeseries: timeSeriesBatchFail}

	awsConfigs := &aws.Config{Region: aws.String(region)}
	clientEnableFailOnLongLabelName := createClient(t, logger, databaseLabel, tableLabel, awsConfigs, true, false)
	clientDisableFailOnLongLabelName := createClient(t, logger, databaseLabel, tableLabel, awsConfigs, false, false)

	type testCase []struct {
		testName    string
		request     *prompb.WriteRequest
		credentials *credentials.Credentials
	}

	successTestCase := testCase{
		{"write normal request", req, awsCredentials},
		{"write request with long metric name", reqLongMetric, awsCredentials},
		{"write request with long label value", reqLongLabel, awsCredentials},
		{"write request with multi-destination and 100 samples per destination per request", reqBatch, awsCredentials},
	}
	for _, test := range successTestCase {
		t.Run(test.testName, func(t *testing.T) {
			_, err := clientDisableFailOnLongLabelName.WriteClient().Write(test.request, test.credentials)
			assert.Nil(t, err)
		})
	}

	invalidTestCase := testCase{
		{"write request with failing long metric name", reqLongMetric, awsCredentials},
		{"write request with failing long label value", reqLongLabel, awsCredentials},
		{"write request with multi-destination and more than 100 samples per destination", reqBatchFail, awsCredentials},
		{"write request with no AWS credentials", reqBatchFail, emptyCredentials},
		{"write request with invalid AWS credentials", reqBatchFail, invalidCredentials},
	}
	for _, test := range invalidTestCase {
		t.Run(test.testName, func(t *testing.T) {
			_, err := clientEnableFailOnLongLabelName.WriteClient().Write(test.request, test.credentials)
			assert.NotNil(t, err)
		})
	}
}

// randomTimestamp generates a random timestamp within the memory store retention in Timestream
func randomTimestamp() int64 {
	delta := int64(time.Duration(memStoreRetentionHour) * time.Hour / time.Millisecond)
	min := nowUnix - delta

	return rand.Int63n(delta) + min
}

// createTimeSeriesTemplate creates a new TimeSeries object with default Labels and Samples.
func createTimeSeriesTemplate(database string, table string) *prompb.TimeSeries {
	randomTime := randomTimestamp()
	return &prompb.TimeSeries{
		Labels: []*prompb.Label{
			{
				Name:  model.MetricNameLabel,
				Value: writeMetricName,
			},
			{
				Name:  databaseLabel,
				Value: database,
			},
			{
				Name:  tableLabel,
				Value: table,
			},
			{
				Name:  "label_1",
				Value: "value_1",
			},
			{
				Name:  "test",
				Value: "TestWriteRead",
			},
		},
		Samples: []prompb.Sample{
			{
				Timestamp: randomTime,
				Value:     value,
			},
		},
	}
}

// createClient creates a new Timestream client containing a Timestream query client and a Timestream write client.
func createClient(t *testing.T, logger log.Logger, database, table string, configs *aws.Config, failOnLongMetricLabelName bool, failOnInvalidSample bool) *timestream.Client {
	client := timestream.NewBaseClient(database, table)
	client.NewWriteClient(logger, configs, failOnLongMetricLabelName, failOnInvalidSample, "")
	return client
}

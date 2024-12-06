/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// These integration tests create real Amazon Timestream read and write clients and send Prometheus remote read and
// write requests directly to the clients. These tests do not create a real Prometheus server nor create a local
// Prometheus Connector server.
package integration

import (
	"context"
	"math/rand"
	"os"
	"testing"
	"time"

	"timestream-prometheus-connector/timestream"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/timestreamwrite"
	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
)

var (
	logger             = log.NewNopLogger()
	nowUnix            = time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	endUnix            = nowUnix + 30000
	destinations       = map[string][]string{database: {table}, database2: {table2}}
	writeClient        *timestreamwrite.Client
	awsCredentials     aws.CredentialsProvider
	emptyCredentials   aws.CredentialsProvider = credentials.NewStaticCredentialsProvider("", "", "")
	invalidCredentials aws.CredentialsProvider = credentials.NewStaticCredentialsProvider("accessKey", "secretKey", "")
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(region))
	if err != nil {
		panic(err)
	}
	awsCredentials = cfg.Credentials

	writeClient = timestreamwrite.NewFromConfig(cfg)
	if err := Setup(ctx, writeClient, destinations); err != nil {
		panic(err)
	}
	code := m.Run()
	if err := Shutdown(ctx, writeClient, destinations); err != nil {
		panic(err)
	}
	os.Exit(code)
}

func TestWriteClient(t *testing.T) {
	ctx := context.Background()
	req := &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		createTimeSeriesTemplate(),
	}}

	tsLongMetric := createTimeSeriesTemplate()
	tsLongMetric.Labels[0].Value = "a_very_long_long_long_long_long_test_metric_that_will_be_over_sixty_bytes"
	reqLongMetric := &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		tsLongMetric,
	}}

	tsLongLabel := createTimeSeriesTemplate()
	tsLongLabel.Labels[1].Name = "a_very_long_long_long_long_long_label_name_that_will_be_over_sixty_bytes"
	reqLongLabel := &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		tsLongLabel,
	}}

	var timeSeriesBatch []*prompb.TimeSeries
	for i := 0; i < numRecords; i++ {
		timeSeriesBatch = append(timeSeriesBatch, createTimeSeriesTemplate())
	}
	reqBatch := &prompb.WriteRequest{Timeseries: timeSeriesBatch}

	// Request with more than 100 samples
	var largeTimeSeriesBatch []*prompb.TimeSeries
	for i := 0; i < largeNumRecords; i++ {
		largeTimeSeriesBatch = append(largeTimeSeriesBatch, createTimeSeriesTemplate())
	}
	largeReqBatch := &prompb.WriteRequest{Timeseries: largeTimeSeriesBatch}

	timeSeriesBatchFail := append(timeSeriesBatch, createTimeSeriesTemplate())
	timeSeriesBatchFail = append(timeSeriesBatchFail, createTimeSeriesTemplate())
	reqBatchFail := &prompb.WriteRequest{Timeseries: timeSeriesBatchFail}

	clientEnableFailOnLongLabelName := createClient(t, logger, database, table, awsCredentials, true, false)
	clientDisableFailOnLongLabelName := createClient(t, logger, database, table, awsCredentials, false, false)

	type testCase []struct {
		testName string
		request  *prompb.WriteRequest
	}

	successTestCase := testCase{
		{"write normal request", req},
		{"write request with long metric name", reqLongMetric},
		{"write request with long label value", reqLongLabel},
		{"write request with 100 samples per request", reqBatch},
		{"write request with more than 100 samples per request", largeReqBatch},
	}
	for _, test := range successTestCase {
		t.Run(test.testName, func(t *testing.T) {
			err := clientDisableFailOnLongLabelName.WriteClient().Write(ctx, test.request, awsCredentials)
			assert.Nil(t, err)
		})
	}
	invalidTestCases := []struct {
		name           string
		request        *prompb.WriteRequest
		creds          aws.CredentialsProvider
		allowLongLabel bool
	}{
		{"write request with failing long metric name", reqLongMetric, invalidCredentials, false},
		{"write request with failing long label value", reqLongLabel, invalidCredentials, false},
		{"write request with no AWS credentials", reqBatchFail, emptyCredentials, true},
		{"write request with invalid AWS credentials", reqBatchFail, invalidCredentials, true},
	}

	for _, tc := range invalidTestCases {
		t.Run(tc.name, func(t *testing.T) {
			var client *timestream.Client
			if tc.allowLongLabel {
				client = createClient(t, logger, database, table, tc.creds, true, false)
			} else {
				client = clientEnableFailOnLongLabelName
			}
			err := client.WriteClient().Write(ctx, tc.request, invalidCredentials)
			assert.NotNil(t, err)
		})
	}

}

func TestQueryClient(t *testing.T) {
	ctx := context.Background()
	writeReq := createWriteRequest()
	request, expectedResponse := createValidReadRequest()

	requestWithInvalidRegex := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: nowUnix,
				EndTimestampMs:   endUnix,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, queryMetricName),
					createLabelMatcher(prompb.LabelMatcher_RE, model.JobLabel, invalidRegex),
				},
				Hints: createReadHints(),
			},
		},
	}

	requestWithInvalidMatcher := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: nowUnix,
				EndTimestampMs:   endUnix,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(invalidMatcher, model.MetricNameLabel, queryMetricName),
				},
				Hints: createReadHints(),
			},
		},
	}

	clientDisableFailOnLongLabelName := createClient(t, logger, database, table, awsCredentials, false, false)

	err := clientDisableFailOnLongLabelName.WriteClient().Write(ctx, writeReq, awsCredentials)
	assert.Nil(t, err)

	invalidTestCase := []struct {
		testName            string
		request             *prompb.ReadRequest
		credentialsProvider aws.CredentialsProvider
	}{
		{"read with invalid regex", requestWithInvalidRegex, awsCredentials},
		{"read with invalid matcher", requestWithInvalidMatcher, awsCredentials},
		{"read with no AWS credentials", request, emptyCredentials},
		{"read with invalid AWS credentials", request, invalidCredentials},
	}

	for _, test := range invalidTestCase {
		t.Run(test.testName, func(t *testing.T) {
			response, err := clientDisableFailOnLongLabelName.QueryClient().Read(context.Background(), test.request, test.credentialsProvider)
			assert.NotNil(t, err)
			assert.Nil(t, response)
		})
	}

	t.Run("read normal request", func(t *testing.T) {
		response, err := clientDisableFailOnLongLabelName.QueryClient().Read(ctx, request, awsCredentials)
		assert.Nil(t, err)
		assert.NotNil(t, response)
		assert.True(t, cmp.Equal(expectedResponse, response), "Actual response does not match expected response.")
	})
}

// randomTimestamp generates a random timestamp within the memory store retention in Timestream
func randomTimestamp() int64 {
	delta := int64(time.Duration(memStoreRetentionHour) * time.Hour / time.Millisecond)
	min := nowUnix - delta

	return rand.Int63n(delta) + min
}

// createTimeSeriesTemplate creates a new TimeSeries object with default Labels and Samples.
func createTimeSeriesTemplate() *prompb.TimeSeries {
	randomTime := randomTimestamp()
	return &prompb.TimeSeries{
		Labels: []*prompb.Label{
			{
				Name:  model.MetricNameLabel,
				Value: writeMetricName,
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

// createLabelMatcher creates a Prometheus LabelMatcher object with parameters.
func createLabelMatcher(matcherType prompb.LabelMatcher_Type, name string, value string) *prompb.LabelMatcher {
	return &prompb.LabelMatcher{
		Type:  matcherType,
		Name:  name,
		Value: value,
	}
}

// createReadHints creates a Prometheus ReadHints object with mock StartMs and EndMs.
func createReadHints() *prompb.ReadHints {
	return &prompb.ReadHints{
		StepMs:  0,
		Func:    "",
		StartMs: nowUnix,
		EndMs:   endUnix,
	}
}

// createClient creates a new Timestream client containing a Timestream query client and a Timestream write client.
func createClient(t *testing.T, logger log.Logger, database, table string, credentials aws.CredentialsProvider, failOnLongMetricLabelName bool, failOnInvalidSample bool) *timestream.Client {
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
		config.WithCredentialsProvider(credentials),
	)
	if err != nil {
		t.Fatalf("failed to load AWS config: %v", err)
	}

	client := timestream.NewBaseClient(database, table)
	client.NewQueryClient(logger, cfg)
	client.NewWriteClient(logger, cfg, failOnLongMetricLabelName, failOnInvalidSample)
	return client
}

// createWriteRequest creates a write request for query test.
func createWriteRequest() *prompb.WriteRequest {
	return &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{
		{
			Labels: []*prompb.Label{
				{
					Name:  model.MetricNameLabel,
					Value: queryMetricName,
				},
				{
					Name:  model.JobLabel,
					Value: jobName,
				},
			},

			Samples: []prompb.Sample{
				{
					Timestamp: nowUnix,
					Value:     value,
				},
			},
		},
	}}
}

// createValidReadRequest creates a read request and expected read response for positive query test.
func createValidReadRequest() (*prompb.ReadRequest, *prompb.ReadResponse) {
	readReq := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: nowUnix,
				EndTimestampMs:   endUnix,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, queryMetricName),
				},
				Hints: &prompb.ReadHints{
					StepMs:  0,
					Func:    "",
					StartMs: nowUnix,
					EndMs:   endUnix,
				},
			},
		},
	}

	expectedResponse := &prompb.ReadResponse{
		Results: []*prompb.QueryResult{
			{
				Timeseries: []*prompb.TimeSeries{
					{
						Labels: []*prompb.Label{
							{
								Name:  model.JobLabel,
								Value: jobName,
							},
							{
								Name:  model.MetricNameLabel,
								Value: queryMetricName,
							},
						},
						Samples: []prompb.Sample{
							{
								Value:     value,
								Timestamp: nowUnix,
							},
						},
					},
				},
			},
		},
	}

	return readReq, expectedResponse
}

/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file contains unit tests for client.go.
package timestream

import (
	goErrors "errors"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/private/protocol"
	"github.com/aws/aws-sdk-go/service/timestreamwrite"
	"github.com/aws/aws-sdk-go/service/timestreamwrite/timestreamwriteiface"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"math"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"
	"timestream-prometheus-connector/errors"
)

var (
	mockLogger         = log.NewNopLogger()
	mockUnixTime       = time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	mockCounter        = prometheus.NewCounter(prometheus.CounterOpts{})
	mockHistogram      = prometheus.NewHistogram(prometheus.HistogramOpts{})
	mockAwsConfigs     = &aws.Config{}
	mockCredentials    = credentials.AnonymousCredentials
)

const (
	mockDatabaseLabel = "timestreamDatabaseName"
	mockTableLabel    = "timestreamTableName"
	mockTableName     = "prom"
	mockDatabaseName  = "promDB"
	mockTableName2    = "prom2"
	mockDatabaseName2 = "promDB2"
	mockRegion        = "us-east-1"
	mockLongMetric    = "prometheus_remote_storage_queue_highest_sent_timestamp_seconds"
	metricName        = "go_gc_duration_seconds"
	measureValueStr   = "0.001995"
	measureValue      = 0.001995
)

type mockTimestreamWriteClient struct {
	mock.Mock
	timestreamwriteiface.TimestreamWriteAPI
}

func (m *mockTimestreamWriteClient) WriteRecords(input *timestreamwrite.WriteRecordsInput) (*timestreamwrite.WriteRecordsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*timestreamwrite.WriteRecordsOutput), args.Error(1)
}

func TestClientNewClient(t *testing.T) {
	client := NewBaseClient(mockDatabaseLabel, mockTableLabel)
	client.NewWriteClient(mockLogger, &aws.Config{Region: aws.String(mockRegion)}, true, true, "")

	assert.NotNil(t, client.writeClient)
	assert.Equal(t, mockLogger, client.writeClient.logger)

	writeConfig := client.writeClient.config
	assert.NotNil(t, writeConfig)
}

func TestWriteClientWrite(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		expectedInput := createNewWriteRecordsInputTemplate()
		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				// Sort the records in the WriteRecordsInput by their time, and sort the Dimension by dimension names.
				sortRecords(writeInput)
				sortRecords(expectedInput)

				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		_, err := c.writeClient.Write(createNewRequestTemplate(), mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("success writing one timeSeries with more than one sample", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				// Sort the records in the WriteRecordsInput by their time, and sort the Dimension by dimension names.
				sortRecords(writeInput)
				sortRecords(expectedInput)

				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples = append(req.Timeseries[0].Samples, prompb.Sample{
			Timestamp: mockUnixTime,
			Value:     measureValue,
		})

		_, err := c.writeClient.Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("success writing samples to different destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInputPromTable := createNewWriteRecordsInputTemplate()
		expectedInputAnotherTable := createNewWriteRecordsInputTemplate()
		expectedInputAnotherTable.DatabaseName = aws.String(mockDatabaseName2)
		expectedInputAnotherTable.TableName = aws.String(mockTableName2)

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInputPromTable)
				sortRecords(expectedInputAnotherTable)
				return reflect.DeepEqual(writeInput, expectedInputPromTable) || reflect.DeepEqual(writeInput, expectedInputAnotherTable)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithDifferentDestination())

		_, err := c.writeClient.Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 2)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("success writing samples to the same destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplate())

		_, err := c.writeClient.Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("missing database name and table name in write series", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithoutDatabaseLabelAndTableLabel())

		_, err := c.writeClient.Write(req, mockCredentials)
		expectedErr := errors.NewMissingDatabaseWithWriteError(mockDatabaseLabel, createTimeSeriesTemplateWithoutDatabaseLabelAndTableLabel())
		assert.Equal(t, err, expectedErr)
	})

	t.Run("missing database name in write series", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithoutDatabaseLabel())

		_, err := c.writeClient.Write(req, mockCredentials)
		expectedErr := errors.NewMissingDatabaseWithWriteError(mockDatabaseLabel, createTimeSeriesTemplateWithoutDatabaseLabel())
		assert.Equal(t, err, expectedErr)
	})

	t.Run("missing table name in write series", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithoutTableLabel())

		_, err := c.writeClient.Write(req, mockCredentials)
		expectedErr := errors.NewMissingTableWithWriteError(mockTableLabel, createTimeSeriesTemplateWithoutTableLabel())
		assert.Equal(t, err, expectedErr)
	})

	t.Run("500 error code from multi-destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		internalServerError := &timestreamwrite.InternalServerException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 500},
		}

		mockTimestreamWriteClient.On("WriteRecords", createNewWriteRecordsInputTemplate()).Return(&timestreamwrite.WriteRecordsOutput{}, internalServerError)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()

		_, err := c.WriteClient().Write(input, mockCredentials)
		assert.Equal(t, internalServerError, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("400 error code before 500 from multi-destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		inputDiffDst := createNewWriteRecordsInputTemplate()
		inputDiffDst.DatabaseName = aws.String(mockDatabaseName2)
		inputDiffDst.TableName = aws.String(mockTableName2)

		validationError := &timestreamwrite.ValidationException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 400},
		}
		internalServerError := &timestreamwrite.InternalServerException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 500},
		}

		mockTimestreamWriteClient.On("WriteRecords", createNewWriteRecordsInputTemplate()).Return(&timestreamwrite.WriteRecordsOutput{}, validationError)
		mockTimestreamWriteClient.On("WriteRecords", inputDiffDst).Return(&timestreamwrite.WriteRecordsOutput{}, internalServerError)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()
		input.Timeseries = append(input.Timeseries, createTimeSeriesTemplateWithDifferentDestination())

		_, err := c.WriteClient().Write(input, mockCredentials)
		assert.Equal(t, internalServerError, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 2)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("500 error code before 400 from multi-destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		inputDiffDst := createNewWriteRecordsInputTemplate()
		inputDiffDst.DatabaseName = aws.String(mockDatabaseName2)
		inputDiffDst.TableName = aws.String(mockTableName2)

		validationError := &timestreamwrite.ValidationException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 400},
		}
		internalServerError := &timestreamwrite.InternalServerException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 500},
		}

		mockTimestreamWriteClient.On("WriteRecords", createNewWriteRecordsInputTemplate()).Return(&timestreamwrite.WriteRecordsOutput{}, internalServerError)
		mockTimestreamWriteClient.On("WriteRecords", inputDiffDst).Return(&timestreamwrite.WriteRecordsOutput{}, validationError)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()
		input.Timeseries = append(input.Timeseries, createTimeSeriesTemplateWithDifferentDestination())

		_, err := c.WriteClient().Write(input, mockCredentials)
		assert.Equal(t, internalServerError, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 2)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("error from convertToRecords due to missing ingestion database destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()
		input.Timeseries[0].Labels = []*prompb.Label{
			{
				Name:  model.MetricNameLabel,
				Value: metricName,
			},
			{
				Name:  "label_1",
				Value: "value_1",
			},
			{
				Name:  mockTableLabel,
				Value: mockTableName,
			},
		}

		_, err := c.WriteClient().Write(input, mockCredentials)
		assert.IsType(t, &errors.MissingDatabaseWithWriteError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("error from convertToRecords due to missing ingestion table destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()
		input.Timeseries[0].Labels = []*prompb.Label{
			{
				Name:  model.MetricNameLabel,
				Value: metricName,
			},
			{
				Name:  "label_1",
				Value: "value_1",
			},
			{
				Name:  mockDatabaseLabel,
				Value: mockDatabaseName,
			},
		}

		_, err := c.WriteClient().Write(input, mockCredentials)
		assert.IsType(t, &errors.MissingTableWithWriteError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("error from WriteRecords()", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		expectedInput := createNewWriteRecordsInputTemplate()
		requestError := &timestreamwrite.ValidationException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 404},
		}

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, requestError)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		_, err := c.WriteClient().Write(createNewRequestTemplate(), mockCredentials)
		assert.Equal(t, requestError, err)

		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("valid timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		expectedInput := createNewWriteRecordsInputTemplate()
		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			})).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
	})

	t.Run("NaN timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.NaN()
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.InvalidSampleValueError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("NaN timeSeries with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.NaN()
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("Inf timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.Inf(1)
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.NotNil(t, err)

		req.Timeseries[0].Samples[0].Value = math.Inf(-1)
		_, err = c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.InvalidSampleValueError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("Inf timeSeries with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.Inf(1)
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		req.Timeseries[0].Samples[0].Value = math.Inf(-1)
		_, err = c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long metric name with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[0].Value = mockLongMetric
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.LongLabelNameError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long metric name with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[0].Value = mockLongMetric
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long dimension name with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[1].Name = mockLongMetric
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.LongLabelNameError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long dimension name with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[1].Name = mockLongMetric
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("unknown SDK error", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		unknownSDKErr := errors.NewSDKNonRequestError(goErrors.New(""))
		mockTimestreamWriteClient.On("WriteRecords", createNewWriteRecordsInputTemplate()).Return(&timestreamwrite.WriteRecordsOutput{}, unknownSDKErr)

		initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		req := createNewRequestTemplate()
		_, err := c.WriteClient().Write(req, mockCredentials)
		assert.Equal(t, unknownSDKErr, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
	})
}

// sortRecords sorts the slice of Record in the WriteRecordsInput by time, and sorts the slice of Dimension by dimension names.
func sortRecords(writeInput *timestreamwrite.WriteRecordsInput) {
	inputRecords := writeInput.Records
	for _, record := range inputRecords {
		dimensions := record.Dimensions
		sort.SliceStable(dimensions, func(i, j int) bool {
			return *dimensions[i].Name < *dimensions[j].Name
		})
	}

	sort.SliceStable(inputRecords, func(i, j int) bool {
		int1, _ := strconv.Atoi(*inputRecords[i].Time)
		int2, _ := strconv.Atoi(*inputRecords[j].Time)
		return int1 < int2
	})
}

// createNewRequestTemplate creates a template of prompb.WriteRequest pointer for unit tests.
func createNewRequestTemplate() *prompb.WriteRequest {
	return &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{createTimeSeriesTemplate()}}
}

// createTimeSeriesTemplate creates a new TimeSeries object with default Labels and Samples.
func createTimeSeriesTemplate() *prompb.TimeSeries {
	return &prompb.TimeSeries{
		Labels: []*prompb.Label{
			{
				Name:  model.MetricNameLabel,
				Value: metricName,
			},
			{
				Name:  "label_1",
				Value: "value_1",
			},
			{
				Name:  mockDatabaseLabel,
				Value: mockDatabaseName,
			},
			{
				Name:  mockTableLabel,
				Value: mockTableName,
			},
		},
		Samples: []prompb.Sample{
			{
				Timestamp: mockUnixTime,
				Value:     measureValue,
			},
		},
	}
}

// createTimeSeriesTemplateWithoutDatabaseLabelAndTableLabel creates a default template without database label and table label.
func createTimeSeriesTemplateWithoutDatabaseLabelAndTableLabel() *prompb.TimeSeries {
	series := createTimeSeriesTemplate()
	series.Labels = series.Labels[:2]
	return series
}

// createTimeSeriesTemplateWithoutDatabaseLabel creates a default template without database label.
func createTimeSeriesTemplateWithoutDatabaseLabel() *prompb.TimeSeries {
	series := createTimeSeriesTemplate()
	series.Labels = append(series.Labels[:2], series.Labels[3:]...)
	return series
}

// createTimeSeriesTemplateWithoutTableLabel creates a default template without table label.
func createTimeSeriesTemplateWithoutTableLabel() *prompb.TimeSeries {
	series := createTimeSeriesTemplate()
	series.Labels = series.Labels[:3]
	return series
}

// createTimeSeriesTemplateWithDifferentDestination creates a new TimeSeries object with default Labels and Samples with a different destination than createTimeSeriesTemplate.
func createTimeSeriesTemplateWithDifferentDestination() *prompb.TimeSeries {
	return &prompb.TimeSeries{
		Labels: []*prompb.Label{
			{
				Name:  model.MetricNameLabel,
				Value: metricName,
			},
			{
				Name:  "label_1",
				Value: "value_1",
			},
			{
				Name:  mockDatabaseLabel,
				Value: mockDatabaseName2,
			},
			{
				Name:  mockTableLabel,
				Value: mockTableName2,
			},
		},
		Samples: []prompb.Sample{
			{
				Timestamp: mockUnixTime,
				Value:     measureValue,
			},
		},
	}
}

// createNewRecordTemplate creates a template of timestreamwrite.Record pointer for unit tests.
func createNewRecordTemplate() *timestreamwrite.Record {
	return &timestreamwrite.Record{
		Dimensions: []*timestreamwrite.Dimension{
			&(timestreamwrite.Dimension{
				Name:  aws.String("label_1"),
				Value: aws.String("value_1")}),
		},
		MeasureName:      aws.String(metricName),
		MeasureValue:     aws.String(measureValueStr),
		MeasureValueType: aws.String("DOUBLE"),
		Time:             aws.String(strconv.FormatInt(mockUnixTime, 10)),
		TimeUnit:         aws.String(timestreamwrite.TimeUnitMilliseconds),
	}
}

// createNewWriteRecordsInputTemplate creates a template of timestreamwrite.WriteRecordsInput pointer for unit tests.
func createNewWriteRecordsInputTemplate() *timestreamwrite.WriteRecordsInput {
	input := &timestreamwrite.WriteRecordsInput{
		DatabaseName: aws.String(mockDatabaseName),
		TableName:    aws.String(mockTableName),
		Records:      []*timestreamwrite.Record{createNewRecordTemplate()},
	}
	return input
}

// createNewWriteClientTemplate creates a template of WriteClient pointer for unit tests.
func createNewWriteClientTemplate(c *Client) *WriteClient {
	return &WriteClient{
		client:             c,
		logger:             mockLogger,
		ignoredSamples:     mockCounter,
		receivedSamples:    mockCounter,
		writeRequests:      mockCounter,
		writeExecutionTime: mockHistogram,
		config:             mockAwsConfigs,
	}
}

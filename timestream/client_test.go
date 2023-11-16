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
	"fmt"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/private/protocol"
	"github.com/aws/aws-sdk-go/service/timestreamquery"
	"github.com/aws/aws-sdk-go/service/timestreamquery/timestreamqueryiface"
	"github.com/aws/aws-sdk-go/service/timestreamwrite"
	"github.com/aws/aws-sdk-go/service/timestreamwrite/timestreamwriteiface"
	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
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
	mockEndUnixTime    = mockUnixTime + 30000
	mockAwsConfigs     = &aws.Config{}
	mockCredentials    = credentials.AnonymousCredentials
	startUnixInSeconds = mockUnixTime / millisToSecConversionRate
	endUnixInSeconds   = mockEndUnixTime / millisToSecConversionRate
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
	instance          = "localhost:9090"
	metricName        = "go_gc_duration_seconds"
	job               = "prometheus"
	measureValueStr   = "0.001995"
	invalidValue      = "invalidValue"
	invalidTime       = "invalidTime"
	timestamp1        = "2020-10-01 15:02:02.000000000"
	timestamp2        = "2020-10-01 20:00:00.000000000"
	quantile          = "0.5"
	instanceRegex     = "9090*"
	jobRegex          = "pro*"
	invalidRegex      = "(?P<login>\\w+)"
	unixTime1         = 1601564522000
	unixTime2         = 1601582400000
	measureValue      = 0.001995
	invalidMatcher    = 10
	functionType      = "func(*timestreamquery.QueryOutput, bool) bool"
)

type mockTimestreamWriteClient struct {
	mock.Mock
	timestreamwriteiface.TimestreamWriteAPI
}

func (m *mockTimestreamWriteClient) WriteRecords(input *timestreamwrite.WriteRecordsInput) (*timestreamwrite.WriteRecordsOutput, error) {
	args := m.Called(input)
	return args.Get(0).(*timestreamwrite.WriteRecordsOutput), args.Error(1)
}

type mockTimestreamQueryClient struct {
	mock.Mock
	timestreamqueryiface.TimestreamQueryAPI
}

func (m *mockTimestreamQueryClient) QueryPages(input *timestreamquery.QueryInput, f func(page *timestreamquery.QueryOutput, lastPage bool) bool) error {
	args := m.Called(input, f)
	return args.Error(0)
}

func TestClientNewClient(t *testing.T) {
	client := NewBaseClient(mockDatabaseLabel, mockTableLabel, mockDatabaseName, mockTableName)
	client.NewWriteClient(mockLogger, &aws.Config{Region: aws.String(mockRegion)}, true, true)

	assert.NotNil(t, client.writeClient)
	assert.Equal(t, mockLogger, client.writeClient.logger)

	writeConfig := client.writeClient.config
	assert.NotNil(t, writeConfig)
}

func TestClientNewQueryClient(t *testing.T) {
	// Mock the instantiation of query client newClients does not create a real query client.
	queryInput := &timestreamquery.QueryInput{QueryString: aws.String("SELECT 1")}
	mockTimestreamQueryClient := new(mockTimestreamQueryClient)
	mockTimestreamQueryClient.On("QueryPages", queryInput,
		mock.AnythingOfType(functionType)).Return(nil)

	client := NewBaseClient(mockDatabaseLabel, mockTableLabel, mockDatabaseName, mockTableName)
	client.NewQueryClient(mockLogger, &aws.Config{Region: aws.String(mockRegion)})

	assert.NotNil(t, client.queryClient)
	assert.Equal(t, mockLogger, client.queryClient.logger)

	queryConfig := client.queryClient.config
	assert.NotNil(t, queryConfig)
}

func TestQueryClientRead(t *testing.T) {
	response := &prompb.ReadResponse{Results: []*prompb.QueryResult{{}}}
	request := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: mockUnixTime,
				EndTimestampMs:   mockEndUnixTime,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, metricName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockDatabaseLabel, mockDatabaseName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockTableLabel, mockTableName),
				},
				Hints: createReadHints(),
			},
		},
	}
	requestWithoutMapping := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: mockUnixTime,
				EndTimestampMs:   mockEndUnixTime,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, metricName),
				},
				Hints: createReadHints(),
			},
		},
	}

	requestMissingTable := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: mockUnixTime,
				EndTimestampMs:   mockEndUnixTime,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, metricName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockDatabaseLabel, mockDatabaseName),
				},
				Hints: createReadHints(),
			},
		},
	}

	requestMissingDatabase := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: mockUnixTime,
				EndTimestampMs:   mockEndUnixTime,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, metricName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockTableLabel, mockTableName),
				},
				Hints: createReadHints(),
			},
		},
	}

	queryInput := &timestreamquery.QueryInput{
		QueryString: aws.String(fmt.Sprintf("SELECT * FROM %s.%s WHERE %s = '%s' AND %s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)",
			mockDatabaseName, mockTableName, measureNameColumnName, metricName, timeColumnName, startUnixInSeconds, endUnixInSeconds)),
	}

	queryOutput := &timestreamquery.QueryOutput{
		ColumnInfo: createColumnInfo(),
		NextToken:  aws.String("nextToken"),
		QueryId:    aws.String("QueryID"),
		Rows: []*timestreamquery.Row{
			{
				Data: createDatumWithInstance(
					true,
					instance,
					measureValueStr,
					metricName,
					timestamp1),
			},
			{
				Data: createDatumWithInstance(
					true,
					instance,
					measureValueStr,
					metricName,
					timestamp2),
			},
			{
				Data: createDatumWithJob(
					true,
					job,
					measureValueStr,
					metricName,
					timestamp1),
			},
			{
				Data: []*timestreamquery.Datum{
					{ScalarValue: aws.String(instance)},
					{ScalarValue: aws.String(job)},
					{ScalarValue: aws.String(measureValueStr)},
					{ScalarValue: aws.String(metricName)},
					{ScalarValue: aws.String(timestamp1)},
				},
			},
		},
	}

	queryOutputWithInvalidMeasureValue := &timestreamquery.QueryOutput{
		ColumnInfo: createColumnInfo(),
		Rows: []*timestreamquery.Row{
			{
				Data: createDatumWithInstance(
					true,
					instance,
					invalidValue,
					metricName,
					timestamp1),
			},
		},
	}

	queryOutputWithInvalidTime := &timestreamquery.QueryOutput{
		ColumnInfo: createColumnInfo(),
		Rows: []*timestreamquery.Row{
			{
				Data: createDatumWithInstance(
					true,
					instance,
					measureValueStr,
					metricName,
					timestamp1),
			},
			{
				Data: createDatumWithInstance(
					true,
					instance,
					measureValueStr,
					metricName,
					invalidTime),
			},
		},
	}

	queryWithMatcherTypes := []*prompb.Query{
		{
			StartTimestampMs: mockUnixTime,
			EndTimestampMs:   mockEndUnixTime,
			Matchers: []*prompb.LabelMatcher{
				createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, metricName),
				createLabelMatcher(prompb.LabelMatcher_EQ, mockDatabaseLabel, mockDatabaseName),
				createLabelMatcher(prompb.LabelMatcher_EQ, mockTableLabel, mockTableName),
				createLabelMatcher(prompb.LabelMatcher_NEQ, model.QuantileLabel, quantile),
				createLabelMatcher(prompb.LabelMatcher_RE, model.JobLabel, jobRegex),
				createLabelMatcher(prompb.LabelMatcher_NRE, model.InstanceLabel, instanceRegex),
			},
			Hints: createReadHints(),
		},
	}

	expectedBuildCommand := []*timestreamquery.QueryInput{
		{
			QueryString: aws.String(fmt.Sprintf("SELECT * FROM %s.%s WHERE %s = '%s' AND quantile != '%s' AND REGEXP_LIKE(job, '%s') AND NOT REGEXP_LIKE(instance, '%s') AND %s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)",
				mockDatabaseName, mockTableName, measureNameColumnName, metricName, quantile, jobRegex, instanceRegex, timeColumnName, startUnixInSeconds, endUnixInSeconds)),
		},
	}

	requestWithInvalidMatcher := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: mockUnixTime,
				EndTimestampMs:   mockEndUnixTime,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(invalidMatcher, model.MetricNameLabel, metricName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockDatabaseLabel, mockDatabaseName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockTableLabel, mockTableName),
				},
				Hints: createReadHints(),
			},
		},
	}

	requestWithInvalidRegex := &prompb.ReadRequest{
		Queries: []*prompb.Query{
			{
				StartTimestampMs: mockUnixTime,
				EndTimestampMs:   mockEndUnixTime,
				Matchers: []*prompb.LabelMatcher{
					createLabelMatcher(prompb.LabelMatcher_EQ, model.MetricNameLabel, metricName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockDatabaseLabel, mockDatabaseName),
					createLabelMatcher(prompb.LabelMatcher_EQ, mockTableLabel, mockTableName),
					createLabelMatcher(prompb.LabelMatcher_RE, model.JobLabel, invalidRegex),
				},
				Hints: createReadHints(),
			},
		},
	}

	queryInputWithInvalidRegex := &timestreamquery.QueryInput{
		QueryString: aws.String(fmt.Sprintf("SELECT * FROM %s.%s WHERE %s = '%s' AND REGEXP_LIKE(job, '%s') AND %s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)",
			mockDatabaseName, mockTableName, measureNameColumnName, metricName, invalidRegex, timeColumnName, startUnixInSeconds, endUnixInSeconds)),
	}

	t.Run("success", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		mockTimestreamQueryClient.On("QueryPages", queryInput,
			mock.AnythingOfType(functionType)).Return(nil)
		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return mockTimestreamQueryClient, nil
		}

		c := &Client{
			writeClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		readResponse, err := c.queryClient.Read(request, mockCredentials)
		assert.Nil(t, err)
		assert.Equal(t, response, readResponse)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("success without default db and table", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		mockTimestreamQueryClient.On("QueryPages", queryInput,
			mock.AnythingOfType(functionType)).Return(nil)
		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return mockTimestreamQueryClient, nil
		}

		c := &Client{
			writeClient:   nil,
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		readResponse, err := c.queryClient.Read(request, mockCredentials)
		assert.Nil(t, err)
		assert.Equal(t, response, readResponse)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("success without mapping", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		mockTimestreamQueryClient.On("QueryPages", queryInput,
			mock.AnythingOfType(functionType)).Return(nil)
		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return mockTimestreamQueryClient, nil
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		readResponse, err := c.queryClient.Read(requestWithoutMapping, mockCredentials)
		assert.Nil(t, err)
		assert.Equal(t, response, readResponse)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("error from buildCommands with missing database name in request", func(t *testing.T) {
		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return new(mockTimestreamQueryClient), nil
		}

		c := &Client{
			writeClient:   nil,
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(requestMissingDatabase, mockCredentials)
		assert.IsType(t, &errors.MissingDatabaseWithQueryError{}, err)
	})

	t.Run("error from buildCommands with missing table name in request", func(t *testing.T) {
		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return new(mockTimestreamQueryClient), nil
		}

		c := &Client{
			writeClient:   nil,
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(requestMissingTable, mockCredentials)
		assert.IsType(t, &errors.MissingTableWithQueryError{}, err)
	})

	t.Run("error from QueryPages()", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		serverError := &timestreamquery.InternalServerException{}
		mockTimestreamQueryClient.On("QueryPages", queryInput,
			mock.AnythingOfType(functionType)).Return(serverError)

		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return mockTimestreamQueryClient, nil
		}

		c := &Client{
			writeClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(request, mockCredentials)
		assert.Equal(t, serverError, err)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("success convert result", func(t *testing.T) {
		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		queryResult, err := c.queryClient.convertToResult(&prompb.QueryResult{}, queryOutput)
		assert.Nil(t, err)
		assert.Equal(t, createExpectedQueryResult(), queryResult)
	})

	t.Run("error from convertToResult with invalid measureValue", func(t *testing.T) {
		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		queryResultWithInvalidValue, err := c.queryClient.convertToResult(&prompb.QueryResult{}, queryOutputWithInvalidMeasureValue)
		assert.NotNil(t, err)
		assert.NotNil(t, queryResultWithInvalidValue)
		assert.Nil(t, queryResultWithInvalidValue.Timeseries)
	})

	t.Run("error from convertToResult with invalid time", func(t *testing.T) {
		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		queryResultWithInvalidTime, err := c.queryClient.convertToResult(&prompb.QueryResult{}, queryOutputWithInvalidTime)
		assert.NotNil(t, err)
		assert.NotNil(t, queryResultWithInvalidTime)
		assert.Nil(t, queryResultWithInvalidTime.Timeseries)
	})

	t.Run("convert result with empty queryOutput", func(t *testing.T) {
		c := &Client{
			writeClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		emptyQueryOutput := &timestreamquery.QueryOutput{}
		queryResult, err := c.queryClient.convertToResult(&prompb.QueryResult{}, emptyQueryOutput)
		assert.Nil(t, err)
		assert.True(t, cmp.Equal(&prompb.QueryResult{}, queryResult))
	})

	t.Run("success build command", func(t *testing.T) {
		c := &Client{
			writeClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		buildCommand, _, err := c.queryClient.buildCommands(queryWithMatcherTypes)
		assert.Nil(t, err)
		assert.Equal(t, expectedBuildCommand, buildCommand)
	})

	t.Run("error from buildCommand with unknown matcher type", func(t *testing.T) {
		c := &Client{
			writeClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(requestWithInvalidMatcher, mockCredentials)
		assert.IsType(t, &errors.UnknownMatcherError{}, err)
	})

	t.Run("error from queryPages with invalid regex", func(t *testing.T) {
		validationError := &timestreamquery.ValidationException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 400},
		}
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		mockTimestreamQueryClient.On("QueryPages", queryInputWithInvalidRegex,
			mock.AnythingOfType(functionType)).Return(validationError)

		initQueryClient = func(config *aws.Config) (timestreamqueryiface.TimestreamQueryAPI, error) {
			return mockTimestreamQueryClient, nil
		}

		c := &Client{
			writeClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(requestWithInvalidRegex, mockCredentials)
		assert.Equal(t, validationError, err)

		mockTimestreamQueryClient.AssertExpectations(t)
	})
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		err := c.writeClient.Write(createNewRequestTemplate(), mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples = append(req.Timeseries[0].Samples, prompb.Sample{
			Timestamp: mockUnixTime,
			Value:     measureValue,
		})

		err := c.writeClient.Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("success writing one timeSeries with more than one sample without mapping", func(t *testing.T) {
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		reqWithoutMapping := createNewRequestTemplateWithoutMapping()
		reqWithoutMapping.Timeseries[0].Samples = append(reqWithoutMapping.Timeseries[0].Samples, prompb.Sample{
			Timestamp: mockUnixTime,
			Value:     measureValue,
		})

		errWm := c.writeClient.Write(reqWithoutMapping, mockCredentials)
		assert.Nil(t, errWm)

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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithDifferentDestination())

		err := c.writeClient.Write(req, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplate())

		err := c.writeClient.Write(req, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:   nil,
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithoutDatabaseLabelAndTableLabel())

		err := c.writeClient.Write(req, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:   nil,
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithoutDatabaseLabel())

		err := c.writeClient.Write(req, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:   nil,
			databaseLabel: mockDatabaseLabel,
			tableLabel:    mockTableLabel,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplateWithoutTableLabel())

		err := c.writeClient.Write(req, mockCredentials)
		expectedErr := errors.NewMissingTableWithWriteError(mockTableLabel, createTimeSeriesTemplateWithoutTableLabel())
		assert.Equal(t, err, expectedErr)
	})

	t.Run("500 error code from multi-destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		internalServerError := &timestreamwrite.InternalServerException{
			RespMetadata: protocol.ResponseMetadata{StatusCode: 500},
		}

		mockTimestreamWriteClient.On("WriteRecords", createNewWriteRecordsInputTemplate()).Return(&timestreamwrite.WriteRecordsOutput{}, internalServerError)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()

		err := c.WriteClient().Write(input, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()
		input.Timeseries = append(input.Timeseries, createTimeSeriesTemplateWithDifferentDestination())

		err := c.WriteClient().Write(input, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		input := createNewRequestTemplate()
		input.Timeseries = append(input.Timeseries, createTimeSeriesTemplateWithDifferentDestination())

		err := c.WriteClient().Write(input, mockCredentials)
		assert.Equal(t, internalServerError, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 2)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("error from convertToRecords due to missing ingestion database destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:   nil,
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

		err := c.WriteClient().Write(input, mockCredentials)
		assert.IsType(t, &errors.MissingDatabaseWithWriteError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("error from convertToRecords due to missing ingestion table destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:   nil,
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

		err := c.WriteClient().Write(input, mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		err := c.WriteClient().Write(createNewRequestTemplate(), mockCredentials)
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

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
	})

	t.Run("NaN timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.NaN()
		err := c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.InvalidSampleValueError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("NaN timeSeries with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.NaN()
		err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("Inf timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.Inf(1)
		err := c.WriteClient().Write(req, mockCredentials)
		assert.NotNil(t, err)

		req.Timeseries[0].Samples[0].Value = math.Inf(-1)
		err = c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.InvalidSampleValueError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("Inf timeSeries with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.Inf(1)
		err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		req.Timeseries[0].Samples[0].Value = math.Inf(-1)
		err = c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long metric name with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[0].Value = mockLongMetric
		err := c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.LongLabelNameError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long metric name with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[0].Value = mockLongMetric
		err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long dimension name with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[1].Name = mockLongMetric
		err := c.WriteClient().Write(req, mockCredentials)
		assert.IsType(t, &errors.LongLabelNameError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long dimension name with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[1].Name = mockLongMetric
		err := c.WriteClient().Write(req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("unknown SDK error", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		unknownSDKErr := errors.NewSDKNonRequestError(goErrors.New(""))
		mockTimestreamWriteClient.On("WriteRecords", createNewWriteRecordsInputTemplate()).Return(&timestreamwrite.WriteRecordsOutput{}, unknownSDKErr)

		initWriteClient = func(config *aws.Config) (timestreamwriteiface.TimestreamWriteAPI, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			databaseLabel:   mockDatabaseLabel,
			tableLabel:      mockTableLabel,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		req := createNewRequestTemplate()
		err := c.WriteClient().Write(req, mockCredentials)
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

// createNewRequestTemplate creates a template of prompb.WriteRequest pointer for unit tests.
func createNewRequestTemplateWithoutMapping() *prompb.WriteRequest {
	return &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{createTimeSeriesTemplateWithoutMapping()}}
}

// createTimeSeriesTemplate creates a new TimeSeries object with no mapping Labels and Samples.
func createTimeSeriesTemplateWithoutMapping() *prompb.TimeSeries {
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
		MeasureValueType: aws.String(timestreamquery.ScalarTypeDouble),
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

// createNewQueryClientTemplate creates a template of QueryClient pointer for unit tests.
func createNewQueryClientTemplate(c *Client) *QueryClient {
	return &QueryClient{
		client:            c,
		logger:            mockLogger,
		readRequests:      mockCounter,
		readExecutionTime: mockHistogram,
		config:            mockAwsConfigs,
	}
}

// createColumnInfo creates a Timestream ColumnInfo for constructing QueryOutput.
func createColumnInfo() []*timestreamquery.ColumnInfo {
	return []*timestreamquery.ColumnInfo{
		{
			Name: aws.String(model.InstanceLabel),
			Type: &timestreamquery.Type{
				ScalarType: aws.String(timestreamquery.ScalarTypeVarchar),
			},
		},
		{
			Name: aws.String(model.JobLabel),
			Type: &timestreamquery.Type{
				ScalarType: aws.String(timestreamquery.ScalarTypeVarchar),
			},
		},
		{
			Name: aws.String(measureValueColumnName),
			Type: &timestreamquery.Type{
				ScalarType: aws.String(timestreamquery.ScalarTypeDouble),
			},
		},
		{
			Name: aws.String(measureNameColumnName),
			Type: &timestreamquery.Type{
				ScalarType: aws.String(timestreamquery.ScalarTypeVarchar),
			},
		},
		{
			Name: aws.String(timeColumnName),
			Type: &timestreamquery.Type{
				ScalarType: aws.String(timestreamquery.ScalarTypeTimestamp),
			},
		},
	}
}

// createDatumWithInstance creates a Timestream Datum object with instance.
func createDatumWithInstance(isNullValue bool, instance string, measureValue string, measureName string, time string) []*timestreamquery.Datum {
	return []*timestreamquery.Datum{
		{ScalarValue: aws.String(instance)},
		{NullValue: aws.Bool(isNullValue)},
		{ScalarValue: aws.String(measureValue)},
		{ScalarValue: aws.String(measureName)},
		{ScalarValue: aws.String(time)},
	}
}

// createDatumWithJob creates a Timestream Datum object with job.
func createDatumWithJob(isNullValue bool, job string, measureValue string, measureName string, time string) []*timestreamquery.Datum {
	return []*timestreamquery.Datum{
		{NullValue: aws.Bool(isNullValue)},
		{ScalarValue: aws.String(job)},
		{ScalarValue: aws.String(measureValue)},
		{ScalarValue: aws.String(measureName)},
		{ScalarValue: aws.String(time)},
	}
}

// createExpectedQueryResult creates a expected queryResult for read unit test.
func createExpectedQueryResult() *prompb.QueryResult {
	return &prompb.QueryResult{
		Timeseries: []*prompb.TimeSeries{
			{
				Labels: []*prompb.Label{
					{
						Name:  model.InstanceLabel,
						Value: instance,
					},
					{
						Name:  model.MetricNameLabel,
						Value: metricName,
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     measureValue,
						Timestamp: unixTime1,
					},
					{
						Value:     measureValue,
						Timestamp: unixTime2,
					},
				},
			},
			{
				Labels: []*prompb.Label{
					{
						Name:  model.JobLabel,
						Value: job,
					},
					{
						Name:  model.MetricNameLabel,
						Value: metricName,
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     measureValue,
						Timestamp: unixTime1,
					},
				},
			},
			{
				Labels: []*prompb.Label{
					{
						Name:  model.InstanceLabel,
						Value: instance,
					},
					{
						Name:  model.JobLabel,
						Value: job,
					},
					{
						Name:  model.MetricNameLabel,
						Value: metricName,
					},
				},
				Samples: []prompb.Sample{
					{
						Value:     measureValue,
						Timestamp: unixTime1,
					},
				},
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
		StartMs: mockUnixTime,
		EndMs:   mockEndUnixTime,
	}
}

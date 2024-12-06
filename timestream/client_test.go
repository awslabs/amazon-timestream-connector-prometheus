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
	"context"
	goErrors "errors"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/timestreamquery"
	qtypes "github.com/aws/aws-sdk-go-v2/service/timestreamquery/types"
	"github.com/aws/aws-sdk-go-v2/service/timestreamwrite"
	wtypes "github.com/aws/aws-sdk-go-v2/service/timestreamwrite/types"
	"github.com/go-kit/log"
	"github.com/google/go-cmp/cmp"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"timestream-prometheus-connector/errors"
)

var (
	mockLogger      = log.NewNopLogger()
	mockUnixTime    = time.Now().UnixNano() / (int64(time.Millisecond) / int64(time.Nanosecond))
	mockCounter     = prometheus.NewCounter(prometheus.CounterOpts{})
	mockHistogram   = prometheus.NewHistogram(prometheus.HistogramOpts{})
	mockEndUnixTime = mockUnixTime + 30000
	mockCredentials = aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("mockAccessKey", "mockSecretKey", "mockSessionToken"))
	mockAwsConfigs  = aws.Config{
		Credentials: mockCredentials,
		Region:      "us-east-1",
	}
	startUnixInSeconds = mockUnixTime / millisToSecConversionRate
	endUnixInSeconds   = mockEndUnixTime / millisToSecConversionRate
)

const (
	mockTableName    = "prom"
	mockDatabaseName = "promDB"
	mockRegion       = "us-east-1"
	mockLongMetric   = "prometheus_remote_storage_queue_highest_sent_timestamp_seconds"
	instance         = "localhost:9090"
	metricName       = "go_gc_duration_seconds"
	job              = "prometheus"
	measureValueStr  = "0.001995"
	invalidValue     = "invalidValue"
	invalidTime      = "invalidTime"
	timestamp1       = "2020-10-01 15:02:02.000000000"
	timestamp2       = "2020-10-01 20:00:00.000000000"
	quantile         = "0.5"
	instanceRegex    = "9090*"
	jobRegex         = "pro*"
	invalidRegex     = "(?P<login>\\w+)"
	unixTime1        = 1601564522000
	unixTime2        = 1601582400000
	measureValue     = 0.001995
	invalidMatcher   = 10
	functionType     = "func(*timestreamquery.QueryOutput, bool) bool"
)

type mockPaginator struct {
	mock.Mock
}

func newMockPaginator(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) *mockPaginator {
	return &mockPaginator{}
}

func (m *mockPaginator) HasMorePages() bool {
	args := m.Called()
	if result := args.Get(0); result != nil {
		return result.(bool)
	}
	return false
}

func (m *mockPaginator) NextPage(ctx context.Context) (*timestreamquery.QueryOutput, error) {
	args := m.Called(ctx)
	if result := args.Get(0); result != nil {
		return result.(*timestreamquery.QueryOutput), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockTimestreamWriteClient struct {
	mock.Mock
}

func (m *mockTimestreamWriteClient) WriteRecords(
	ctx context.Context,
	input *timestreamwrite.WriteRecordsInput,
	optFns ...func(*timestreamwrite.Options),
) (*timestreamwrite.WriteRecordsOutput, error) {
	args := m.Called(ctx, input, optFns)
	if result := args.Get(0); result != nil {
		return result.(*timestreamwrite.WriteRecordsOutput), args.Error(1)
	}
	return nil, args.Error(1)
}

type mockTimestreamQueryClient struct {
	mock.Mock
	*timestreamquery.Client
}

func TestClientNewWriteClient(t *testing.T) {
	client := NewBaseClient(mockDatabaseName, mockTableName)
	client.NewWriteClient(mockLogger, aws.Config{Region: mockRegion}, true, true)

	assert.NotNil(t, client.writeClient)
	assert.Equal(t, mockLogger, client.writeClient.logger)

	writeConfig := client.writeClient.config
	assert.NotNil(t, writeConfig)
}

func TestClientNewQueryClient(t *testing.T) {
	client := NewBaseClient(mockDatabaseName, mockTableName)
	client.NewQueryClient(mockLogger, aws.Config{Region: mockRegion})

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
		Rows: []qtypes.Row{
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
				Data: []qtypes.Datum{
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
		Rows: []qtypes.Row{
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
		Rows: []qtypes.Row{
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
					createLabelMatcher(prompb.LabelMatcher_RE, model.JobLabel, invalidRegex),
				},
				Hints: createReadHints(),
			},
		},
	}

	queryInputWithInvalidRegex := &timestreamquery.QueryInput{
		QueryString: aws.String(fmt.Sprintf(
			"SELECT * FROM %s.%s WHERE %s = '%s' AND REGEXP_LIKE(job, '%s') AND %s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)",
			mockDatabaseName,
			mockTableName,
			measureNameColumnName,
			metricName,
			invalidRegex,
			timeColumnName,
			startUnixInSeconds,
			endUnixInSeconds,
		)),
	}

	t.Run("success", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		initQueryClient = func(config aws.Config) (*timestreamquery.Client, error) {
			return mockTimestreamQueryClient.Client, nil
		}

		mockPaginator := newMockPaginator(mockTimestreamQueryClient.Client, queryInput)
		mockPaginator.On("HasMorePages").Return(false, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, nil)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}
		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		readResponse, err := c.queryClient.Read(context.Background(), request, mockCredentials)
		assert.Nil(t, err)
		assert.Equal(t, response, readResponse)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("success without mapping", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		initQueryClient = func(config aws.Config) (*timestreamquery.Client, error) {
			return mockTimestreamQueryClient.Client, nil
		}
		mockPaginator := newMockPaginator(mockTimestreamQueryClient.Client, queryInput)
		mockPaginator.On("HasMorePages").Return(false, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, nil)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		readResponse, err := c.queryClient.Read(context.Background(), request, mockCredentials)
		assert.Nil(t, err)
		assert.Equal(t, response, readResponse)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("error from buildCommands with missing database name in request", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		initQueryClient = func(config aws.Config) (*timestreamquery.Client, error) {
			return mockTimestreamQueryClient.Client, nil
		}

		mockPaginator := newMockPaginator(mockTimestreamQueryClient.Client, queryInput)
		mockPaginator.On("HasMorePages").Return(false, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, nil)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		c := &Client{
			writeClient: nil,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(context.Background(), request, mockCredentials)
		assert.IsType(t, &errors.MissingDatabaseError{}, err)
	})

	t.Run("success from NextPage() using data helpers", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		initQueryClient = func(config aws.Config) (*timestreamquery.Client, error) {
			return mockTimestreamQueryClient.Client, nil
		}

		mockPaginator := newMockPaginator(mockTimestreamQueryClient.Client, queryInput)
		mockPaginator.On("HasMorePages").Return(false, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, nil)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		readResponse, err := c.queryClient.Read(context.Background(), request, mockCredentials)

		assert.NoError(t, err)
		assert.NotNil(t, readResponse)
		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("error from NextPage()", func(t *testing.T) {
		serverError := &qtypes.InternalServerException{Message: aws.String("Server error")}

		mockPaginator := new(mockPaginator)
		mockPaginator.On("HasMorePages").Return(true, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, serverError)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(context.Background(), request, mockCredentials)
		assert.Equal(t, serverError, err)

		mockPaginator.AssertExpectations(t)
	})

	t.Run("error from NextPage() with invalid regex", func(t *testing.T) {
		validationError := &wtypes.ValidationException{Message: aws.String("Validation error occurred")}
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)

		mockPaginator := newMockPaginator(mockTimestreamQueryClient.Client, queryInputWithInvalidRegex)
		mockPaginator.On("HasMorePages").Return(true, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, validationError)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		initQueryClient = func(config aws.Config) (*timestreamquery.Client, error) {
			return mockTimestreamQueryClient.Client, nil
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(context.Background(), requestWithInvalidRegex, mockCredentials)
		assert.Equal(t, validationError, err)

		mockTimestreamQueryClient.AssertExpectations(t)
	})

	t.Run("success convert result", func(t *testing.T) {
		c := &Client{
			queryClient:     nil,
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
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		buildCommand, _, err := c.queryClient.buildCommands(queryWithMatcherTypes)
		assert.Nil(t, err)
		assert.Equal(t, expectedBuildCommand, buildCommand)
	})

	t.Run("error from buildCommand with unknown matcher type", func(t *testing.T) {
		mockPaginator := new(mockPaginator)
		mockPaginator.On("HasMorePages").Return(false, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, nil)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(context.Background(), requestWithInvalidMatcher, mockCredentials)
		assert.IsType(t, &errors.UnknownMatcherError{}, err)
	})

	t.Run("error from buildCommands with missing table name in request", func(t *testing.T) {
		mockTimestreamQueryClient := new(mockTimestreamQueryClient)
		initQueryClient = func(config aws.Config) (*timestreamquery.Client, error) {
			return mockTimestreamQueryClient.Client, nil
		}

		mockPaginator := newMockPaginator(mockTimestreamQueryClient.Client, queryInput)
		mockPaginator.On("HasMorePages").Return(false, nil)
		mockPaginator.On("NextPage", mock.Anything).Return(nil, nil)
		initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
			return mockPaginator
		}

		c := &Client{
			writeClient:     nil,
			defaultDataBase: mockDatabaseName,
		}
		c.queryClient = createNewQueryClientTemplate(c)

		_, err := c.queryClient.Read(context.Background(), request, mockCredentials)
		assert.IsType(t, &errors.MissingTableError{}, err)
	})
}

func TestWriteClientWrite(t *testing.T) {
	t.Run("success", func(t *testing.T) {

		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		expectedInput := createNewWriteRecordsInputTemplate()

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				// Sort the records in the WriteRecordsInput by their time, and sort the Dimension by dimension names.
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		err := c.writeClient.Write(context.Background(), createNewRequestTemplate(), mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertCalled(t, "WriteRecords", mock.Anything, expectedInput, mock.Anything)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("success writing one timeSeries with more than one sample", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples = append(req.Timeseries[0].Samples, prompb.Sample{
			Timestamp: mockUnixTime,
			Value:     measureValue,
		})

		err := c.writeClient.Write(context.Background(), req, mockCredentials)
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
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
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

		errWm := c.writeClient.Write(context.Background(), reqWithoutMapping, mockCredentials)
		assert.Nil(t, errWm)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("success writing samples to the same destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)

				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplate())

		err := c.writeClient.Write(context.Background(), req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("missing database name in write series", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)

				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient: nil,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplate())

		err := c.writeClient.Write(context.Background(), req, mockCredentials)
		expectedErr := errors.NewMissingDatabaseWithWriteError("", createTimeSeriesTemplate())
		assert.Equal(t, err, expectedErr)
	})

	t.Run("missing table name in write series", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		expectedInput := createNewWriteRecordsInputTemplate()
		expectedInput.Records = append(expectedInput.Records, createNewRecordTemplate())

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)

				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		req := createNewRequestTemplate()
		req.Timeseries = append(req.Timeseries, createTimeSeriesTemplate())

		err := c.writeClient.Write(context.Background(), req, mockCredentials)
		expectedErr := errors.NewMissingTableWithWriteError("", createTimeSeriesTemplate())
		assert.Equal(t, err, expectedErr)
	})

	t.Run("error from convertToRecords due to missing ingestion database destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient: nil,
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
		}

		err := c.WriteClient().Write(context.Background(), input, mockCredentials)
		assert.IsType(t, &errors.MissingDatabaseWithWriteError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("error from convertToRecords due to missing ingestion table destination", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
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
		}

		err := c.WriteClient().Write(context.Background(), input, mockCredentials)
		assert.IsType(t, &errors.MissingTableWithWriteError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("error from WriteRecords()", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		expectedInput := createNewWriteRecordsInputTemplate()
		requestError := &wtypes.ValidationException{
			Message: aws.String("Validation error occurred"),
		}

		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, requestError)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		err := c.WriteClient().Write(context.Background(), createNewRequestTemplate(), mockCredentials)
		assert.Equal(t, requestError, err)

		mockTimestreamWriteClient.AssertExpectations(t)
	})

	t.Run("valid timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		expectedInput := createNewWriteRecordsInputTemplate()
		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			mock.MatchedBy(func(writeInput *timestreamwrite.WriteRecordsInput) bool {
				sortRecords(writeInput)
				sortRecords(expectedInput)
				return reflect.DeepEqual(writeInput, expectedInput)
			}),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{}, nil)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 1)
	})

	t.Run("NaN timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.NaN()
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.IsType(t, &errors.InvalidSampleValueError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("NaN timeSeries with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.NaN()
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("Inf timeSeries with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}
		ctx := context.Background()

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.Inf(1)
		err := c.WriteClient().Write(ctx, req, mockCredentials)
		assert.NotNil(t, err)

		req.Timeseries[0].Samples[0].Value = math.Inf(-1)
		err = c.WriteClient().Write(ctx, req, mockCredentials)
		assert.IsType(t, &errors.InvalidSampleValueError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("Inf timeSeries with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}
		ctx := context.Background()

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnInvalidSample = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Samples[0].Value = math.Inf(1)
		err := c.WriteClient().Write(ctx, req, mockCredentials)
		assert.Nil(t, err)

		req.Timeseries[0].Samples[0].Value = math.Inf(-1)
		err = c.WriteClient().Write(ctx, req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long metric name with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[0].Value = mockLongMetric
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.IsType(t, &errors.LongLabelNameError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long metric name with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[0].Value = mockLongMetric
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long dimension name with fail-fast enabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = true

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[1].Name = mockLongMetric
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.IsType(t, &errors.LongLabelNameError{}, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("long dimension name with fail-fast disabled", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)
		c.writeClient.failOnLongMetricLabelName = false

		req := createNewRequestTemplate()
		req.Timeseries[0].Labels[1].Name = mockLongMetric
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
		assert.Nil(t, err)

		mockTimestreamWriteClient.AssertNumberOfCalls(t, "WriteRecords", 0)
	})

	t.Run("unknown SDK error", func(t *testing.T) {
		mockTimestreamWriteClient := new(mockTimestreamWriteClient)
		unknownSDKErr := errors.NewSDKNonRequestError(goErrors.New(""))
		mockTimestreamWriteClient.On(
			"WriteRecords",
			mock.Anything,
			createNewWriteRecordsInputTemplate(),
			mock.Anything,
		).Return(&timestreamwrite.WriteRecordsOutput{},
			unknownSDKErr)

		initWriteClient = func(config aws.Config) (TimestreamWriteClient, error) {
			return mockTimestreamWriteClient, nil
		}

		c := &Client{
			queryClient:     nil,
			defaultDataBase: mockDatabaseName,
			defaultTable:    mockTableName,
		}
		c.writeClient = createNewWriteClientTemplate(c)

		req := createNewRequestTemplate()
		err := c.WriteClient().Write(context.Background(), req, mockCredentials)
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
	return &prompb.WriteRequest{Timeseries: []*prompb.TimeSeries{createTimeSeriesTemplate()}}
}

// createNewRecordTemplate creates a template of timestreamwrite.Record pointer for unit tests.
func createNewRecordTemplate() wtypes.Record {
	return wtypes.Record{
		Dimensions: []wtypes.Dimension{
			(wtypes.Dimension{
				Name:  aws.String("label_1"),
				Value: aws.String("value_1")}),
		},
		MeasureName:      aws.String(metricName),
		MeasureValue:     aws.String(measureValueStr),
		MeasureValueType: wtypes.MeasureValueTypeDouble,
		Time:             aws.String(strconv.FormatInt(mockUnixTime, 10)),
		TimeUnit:         wtypes.TimeUnitMilliseconds,
	}
}

// createNewWriteRecordsInputTemplate creates a template of timestreamwrite.WriteRecordsInput pointer for unit tests.
func createNewWriteRecordsInputTemplate() *timestreamwrite.WriteRecordsInput {
	input := &timestreamwrite.WriteRecordsInput{
		DatabaseName: aws.String(mockDatabaseName),
		TableName:    aws.String(mockTableName),
		Records:      []wtypes.Record{createNewRecordTemplate()},
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
func createColumnInfo() []qtypes.ColumnInfo {
	return []qtypes.ColumnInfo{
		{
			Name: aws.String(model.InstanceLabel),
			Type: &qtypes.Type{
				ScalarType: qtypes.ScalarTypeVarchar,
			},
		},
		{
			Name: aws.String(model.JobLabel),
			Type: &qtypes.Type{
				ScalarType: qtypes.ScalarTypeVarchar,
			},
		},
		{
			Name: aws.String(measureValueColumnName),
			Type: &qtypes.Type{
				ScalarType: qtypes.ScalarTypeDouble,
			},
		},
		{
			Name: aws.String(measureNameColumnName),
			Type: &qtypes.Type{
				ScalarType: qtypes.ScalarTypeVarchar,
			},
		},
		{
			Name: aws.String(timeColumnName),
			Type: &qtypes.Type{
				ScalarType: qtypes.ScalarTypeTimestamp,
			},
		},
	}
}

// createDatumWithInstance creates a Timestream Datum object with instance.
func createDatumWithInstance(isNullValue bool, instance string, measureValue string, measureName string, time string) []qtypes.Datum {
	return []qtypes.Datum{
		{ScalarValue: aws.String(instance)},
		{NullValue: aws.Bool(isNullValue)},
		{ScalarValue: aws.String(measureValue)},
		{ScalarValue: aws.String(measureName)},
		{ScalarValue: aws.String(time)},
	}
}

// createDatumWithJob creates a Timestream Datum object with job.
func createDatumWithJob(isNullValue bool, job string, measureValue string, measureName string, time string) []qtypes.Datum {
	return []qtypes.Datum{
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

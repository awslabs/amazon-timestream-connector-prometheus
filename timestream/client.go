/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file does the following:
// 1. converts the Prometheus read requests and write requests to Amazon Timestream queries and records;
// 2. sends the queries and records to Amazon Timestream through the read and write clients;
// 3. converts the query results from Amazon Timestream to Prometheus read responses.
package timestream

import (
	"context"
	goErrors "errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/timestreamquery"
	qtypes "github.com/aws/aws-sdk-go-v2/service/timestreamquery/types"
	"github.com/aws/aws-sdk-go-v2/service/timestreamwrite"
	wtypes "github.com/aws/aws-sdk-go-v2/service/timestreamwrite/types"
	"github.com/aws/smithy-go"
	"github.com/aws/smithy-go/transport/http"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	prometheusClientModel "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"

	"timestream-prometheus-connector/errors"
)

type labelOperation string
type longMetricsOperation func(measureValueName string) (labelOperation, error)

var addUserAgentMiddleware = middleware.AddUserAgentKey("Prometheus Connector/" + Version)

// Store the initialization function calls to allow unit tests to mock the creation of real clients.
var initWriteClient = func(cfg aws.Config) (TimestreamWriteClient, error) {
	client := timestreamwrite.NewFromConfig(cfg, func(o *timestreamwrite.Options) {
		o.APIOptions = append(o.APIOptions, addUserAgentMiddleware)
	})
	return client, nil
}

var initQueryClient = func(cfg aws.Config) (*timestreamquery.Client, error) {
	client := timestreamquery.NewFromConfig(cfg, func(o *timestreamquery.Options) {
		o.APIOptions = append(o.APIOptions, addUserAgentMiddleware)
	})
	return client, nil
}

var initPaginatorFactory = func(timestreamQuery *timestreamquery.Client, queryInput *timestreamquery.QueryInput) Paginator {
	return &TimestreamPaginator{
		paginator: timestreamquery.NewQueryPaginator(timestreamQuery, queryInput),
	}
}

// recordDestinationMap is a nested map that stores slices of Records based on the ingestion destination.
// Below is an example of the map structure:
//
//	records := map[string]map[string][]wtypes.Record{
//			"database1": map[string][]wtypes.Record{
//					"table1":[]wtypes.Record{record1, record2},
//					"table2":[]wtypes.Record{record3},
//			},
//			"database2": map[string]string{
//					"table3":[]wtypes.Record{record4, record5},
//					"table4":[]wtypes.Record{record6},
//			},
type recordDestinationMap map[string]map[string][]wtypes.Record

const (
	maxWriteBatchLength         int            = 100
	maxMeasureNameLength        int            = 60
	ignored                     labelOperation = "Ignored"
	failed                      labelOperation = "Failed"
	unmodified                  labelOperation = "Unmodified"
	timeColumnName              string         = "time"
	measureValueColumnName      string         = "measure_value::double"
	measureNameColumnName       string         = "measure_name"
	timestampLayout             string         = "2006-01-02 15:04:05.000000000"
	millisToSecConversionRate                  = int64(time.Second) / int64(time.Millisecond)
	nanosToMillisConversionRate                = int64(time.Millisecond) / int64(time.Nanosecond)
)

type QueryClient struct {
	client            *Client
	config            aws.Config
	logger            log.Logger
	readExecutionTime prometheus.Histogram
	readRequests      prometheus.Counter
	timestreamQuery   *timestreamquery.Client
}

type WriteClient struct {
	client                    *Client
	config                    aws.Config
	logger                    log.Logger
	ignoredSamples            prometheus.Counter
	receivedSamples           prometheus.Counter
	writeRequests             prometheus.Counter
	writeExecutionTime        prometheus.Histogram
	timestreamWrite           TimestreamWriteClient
	failOnLongMetricLabelName bool
	failOnInvalidSample       bool
}

type Client struct {
	queryClient     *QueryClient
	writeClient     *WriteClient
	defaultDataBase string
	defaultTable    string
}

type TimestreamWriteClient interface {
	WriteRecords(ctx context.Context, input *timestreamwrite.WriteRecordsInput, optFns ...func(*timestreamwrite.Options)) (*timestreamwrite.WriteRecordsOutput, error)
}

// Paginator defines the interface for Timestream pagination
type Paginator interface {
	HasMorePages() bool
	NextPage(ctx context.Context) (*timestreamquery.QueryOutput, error)
}

// TimestreamPaginator wraps the actual Timestream paginator to support mocking in unit tests
type TimestreamPaginator struct {
	paginator *timestreamquery.QueryPaginator
}

func (tp *TimestreamPaginator) HasMorePages() bool {
	return tp.paginator.HasMorePages()
}

func (tp *TimestreamPaginator) NextPage(ctx context.Context) (*timestreamquery.QueryOutput, error) {
	return tp.paginator.NextPage(ctx)
}

type PaginatorFactory func(queryInput *timestreamquery.QueryInput) Paginator

// NewBaseClient creates a Timestream Client object with the ingestion destination labels.
func NewBaseClient(defaultDataBase, defaultTable string) *Client {
	client := &Client{
		defaultDataBase: defaultDataBase,
		defaultTable:    defaultTable,
	}

	return client
}

// NewQueryClient creates a new Timestream query client with the given set of configuration.
func (c *Client) NewQueryClient(logger log.Logger, configs aws.Config) {
	c.queryClient = &QueryClient{
		client: c,
		logger: logger,
		config: configs,
		readRequests: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "timestream_connector_read_requests_total",
				Help: "The total number of query requests to Timestream.",
			},
		),
		readExecutionTime: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "timestream_connector_read_duration_seconds",
				Help:    "The total execution time for the read requests.",
				Buckets: prometheus.DefBuckets,
			},
		),
	}
}

// NewWriteClient creates a new Timestream write client with a given set of configurations.
func (c *Client) NewWriteClient(logger log.Logger, configs aws.Config, failOnLongMetricLabelName bool, failOnInvalidSample bool) {
	c.writeClient = &WriteClient{
		client:                    c,
		logger:                    logger,
		config:                    configs,
		failOnLongMetricLabelName: failOnLongMetricLabelName,
		failOnInvalidSample:       failOnInvalidSample,
		ignoredSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "timestream_connector_ignored_samples_total",
				Help: "The total number of samples not sent to Timestream due to long metric/label name and unsupported non-finite float values (Inf, -Inf, NaN).",
			},
		),
		receivedSamples: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "timestream_connector_received_samples_total",
				Help: "The total number of samples received by the Prometheus connector.",
			},
		),
		writeRequests: prometheus.NewCounter(
			prometheus.CounterOpts{
				Name: "timestream_connector_write_requests_total",
				Help: "The total number of data ingestion requests to Timestream.",
			},
		),
		writeExecutionTime: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "timestream_connector_write_duration_seconds",
				Help:    "The total execution time for the write requests.",
				Buckets: prometheus.DefBuckets,
			},
		),
	}
}

// Write sends the prompb.WriteRequest to timestreamwriteiface.TimestreamWriteAPI
func (wc *WriteClient) Write(ctx context.Context, req *prompb.WriteRequest, credentialsProvider aws.CredentialsProvider) error {
	wc.config.Credentials = credentialsProvider
	var err error
	wc.timestreamWrite, err = initWriteClient(wc.config)
	if err != nil {
		LogError(wc.logger, "Unable to construct a new session with the given credentials.", err)
		return err
	}

	LogInfo(wc.logger, fmt.Sprintf("%d records requested for ingestion from Prometheus.", len(req.Timeseries)))

	recordMap := make(recordDestinationMap)
	recordMap, err = wc.convertToRecords(req.Timeseries, recordMap)
	if err != nil {
		LogError(wc.logger, "Unable to convert the received Prometheus write request to Timestream Records.", err)
		return err
	}

	var sdkErr error
	for database, tableMap := range recordMap {
		for table, records := range tableMap {
			recordLen := len(records)
			// Timestream will return an error if more than 100 records are sent in a batch.
			// Therefore, records should be chunked if there are more than 100 of them
			for chunkStartIndex := 0; chunkStartIndex < recordLen; chunkStartIndex += maxWriteBatchLength {
				chunkEndIndex := chunkStartIndex + maxWriteBatchLength
				if chunkEndIndex > recordLen {
					chunkEndIndex = recordLen
				}

				currentChunkSize := chunkEndIndex - chunkStartIndex

				writeRecordsInput := &timestreamwrite.WriteRecordsInput{
					DatabaseName: aws.String(database),
					TableName:    aws.String(table),
					Records:      records[chunkStartIndex:chunkEndIndex],
				}

				begin := time.Now()
				_, err = wc.timestreamWrite.WriteRecords(ctx, writeRecordsInput)
				duration := time.Since(begin).Seconds()

				if err != nil {
					sdkErr = wc.handleSDKErr(req, err, sdkErr)
				} else {
					LogInfo(wc.logger, fmt.Sprintf("Successfully wrote %d records to Database: %s, Table: %s", currentChunkSize, database, table))

					recordsIgnored := getCounterValue(wc.ignoredSamples)
					if recordsIgnored > 0 {
						LogInfo(wc.logger, fmt.Sprintf("%d records were rejected for ingestion to Timestream. See Troubleshooting in the README for possible reasons, or enable debug logging for more details.", recordsIgnored))
					}
				}

				wc.writeExecutionTime.Observe(duration)
				wc.writeRequests.Inc()
			}
		}
	}

	return sdkErr
}

// Read converts the Prometheus prompb.ReadRequest into Timestream queries and return
// the result set as Prometheus prompb.ReadResponse.
func (qc *QueryClient) Read(
	ctx context.Context,
	req *prompb.ReadRequest,
	credentialsProvider aws.CredentialsProvider,
) (*prompb.ReadResponse, error) {
	qc.config.Credentials = credentialsProvider
	var err error
	qc.timestreamQuery, err = initQueryClient(qc.config)
	if err != nil {
		LogError(qc.logger, "Unable to construct a new session with the given credentials", err)
		return nil, err
	}
	queryInputs, isRelatedToRegex, err := qc.buildCommands(req.Queries)
	if err != nil {
		LogError(qc.logger, "Error occurred while translating Prometheus query.", err)
		return nil, err
	}

	results := []*prompb.QueryResult{{}}
	resultSet := results[0]

	begin := time.Now()
	var queryPageError error

	for _, queryInput := range queryInputs {
		paginator := initPaginatorFactory(qc.timestreamQuery, queryInput)
		for paginator.HasMorePages() {
			page, err := paginator.NextPage(ctx)
			if err != nil {
				queryPageError = err
				LogError(qc.logger, "Error occurred while fetching the next page of results.", err)
				break
			}

			resultSet, err = qc.convertToResult(resultSet, page)
			qc.readRequests.Inc()
			if err != nil {
				LogError(qc.logger, "Error occurred while converting the Timestream query results to Prometheus QueryResults", err)
				return nil, err
			}

		}

		if queryPageError != nil {
			var apiError *smithy.GenericAPIError
			if goErrors.As(queryPageError, &apiError) && apiError.Code == "ValidationException" && isRelatedToRegex {
				LogError(qc.logger, "Error occurred due to unsupported query. Please validate the regular expression used in the query. Check the documentation for unsupported RE2 syntax.", queryPageError)
				return nil, queryPageError
			}

			LogError(qc.logger, "Error occurred while querying Timestream pages.", queryPageError)
			return nil, queryPageError
		}
	}

	duration := time.Since(begin).Seconds()
	qc.readExecutionTime.Observe(duration)

	return &prompb.ReadResponse{
		Results: results,
	}, nil
}

// handleSDKErr parses and logs the error from SDK (if any)
func (wc *WriteClient) handleSDKErr(req *prompb.WriteRequest, currErr error, errToReturn error) error {
	var responseError *http.ResponseError
	if !goErrors.As(currErr, &responseError) {
		LogError(wc.logger, fmt.Sprintf("Error occurred while ingesting Timestream Records. %d records failed to be written", len(req.Timeseries)), currErr)
		return currErr
	}

	if errToReturn == nil {
		errToReturn = currErr
	}

	statusCode := responseError.HTTPStatusCode()
	switch statusCode / 100 {
	case 4:
		LogDebug(wc.logger, "Error occurred while ingesting data due to invalid write request. "+
			"Some Prometheus Samples were not ingested into Timestream, please review the write request and check the documentation for troubleshooting.",
			"request", req)
	case 5:
		errToReturn = currErr
		LogDebug(wc.logger, "Internal server error occurred. Samples will be retried by Prometheus", "request", req)
	}

	return errToReturn
}

// convertToRecords converts a slice of *prompb.TimeSeries to a slice of wtypes.Record
func (wc *WriteClient) convertToRecords(series []*prompb.TimeSeries, recordMap recordDestinationMap) (recordDestinationMap, error) {
	var operationOnLongMetrics longMetricsOperation
	if wc.failOnLongMetricLabelName {
		operationOnLongMetrics = func(measureValueName string) (labelOperation, error) {
			if len(measureValueName) > maxMeasureNameLength {
				err := errors.NewLongLabelNameError(measureValueName, maxMeasureNameLength)
				LogError(wc.logger, "fail-on-long-label flag is enabled for long metric name.", err)
				return failed, err
			}
			return unmodified, nil
		}
	} else {
		operationOnLongMetrics = func(measureValueName string) (labelOperation, error) {
			if len(measureValueName) > maxMeasureNameLength {
				wc.ignoredSamples.Inc()
				LogDebug(wc.logger, "fail-on-long-label flag is disabled for metric name. Time series ignored.", "ignoredMeasureName", measureValueName)
				return ignored, nil
			}

			return unmodified, nil
		}
	}
	return processTimeSeries(wc, operationOnLongMetrics, series, recordMap)
}

// processTimeSeries processes a slice of *prompb.TimeSeries to a slice of wtypes.Record
func processTimeSeries(wc *WriteClient, operationOnLongMetrics longMetricsOperation, series []*prompb.TimeSeries, recordMap recordDestinationMap) (recordDestinationMap, error) {
	for _, timeSeries := range series {
		var dimensions []wtypes.Dimension
		var err error
		var operation labelOperation
		var databaseName string
		var tableName string
		wc.receivedSamples.Add(float64(len(timeSeries.Samples)))

		metricLabels, measureValueName := convertToMap(timeSeries.Labels)

		databaseName = wc.client.defaultDataBase
		tableName = wc.client.defaultTable

		if len(databaseName) == 0 {
			err = errors.NewMissingDatabaseWithWriteError(wc.client.defaultDataBase, timeSeries)
			return nil, err
		}

		if len(tableName) == 0 {
			err = errors.NewMissingTableWithWriteError(wc.client.defaultTable, timeSeries)
			return nil, err
		}

		operation, err = operationOnLongMetrics(measureValueName)
		switch operation {
		case failed:
			return nil, err
		case ignored:
			continue
		default:
		}

		dimensions, operation, err = processMetricLabels(metricLabels, operationOnLongMetrics)
		switch operation {
		case failed:
			return nil, err
		case ignored:
			continue
		default:
		}

		recordMap[databaseName] = getOrCreateRecordMapEntry(recordMap, databaseName)

		var records []wtypes.Record

		if recordMap[databaseName][tableName] != nil {
			records = recordMap[databaseName][tableName]
		}

		records, err = wc.appendRecords(records, timeSeries, dimensions, measureValueName)
		if err != nil {
			return nil, err
		}

		if len(records) == 0 {
			LogInfo(wc.logger, "No valid Timestream Records can be ingested.")
			continue
		}

		recordMap[databaseName][tableName] = records

	}
	return recordMap, nil
}

// processMetricLabels processes metricLabels to a wtypes.Record
func processMetricLabels(metricLabels map[string]string, operationOnLongMetrics longMetricsOperation) ([]wtypes.Dimension, labelOperation, error) {
	var operation labelOperation
	var dimensions []wtypes.Dimension
	var err error
	for name, value := range metricLabels {
		// Each label in the metricLabels map contains a characteristic/dimension of the metric, which maps to wtypes.Dimension
		operation, err = operationOnLongMetrics(name)
		switch operation {
		case failed:
			return nil, operation, err
		case ignored:
			return nil, operation, nil
		default:
			dimensions = append(dimensions, wtypes.Dimension{
				Name:  aws.String(name),
				Value: aws.String(value),
			})
		}
	}
	return dimensions, operation, nil
}

// getOrCreateRecordMapEntry gets record map entry
func getOrCreateRecordMapEntry(recordMap recordDestinationMap, databaseName string) map[string][]wtypes.Record {
	if recordMap[databaseName] == nil {
		recordMap[databaseName] = make(map[string][]wtypes.Record)
	}
	return recordMap[databaseName]
}

// convertToMap converts the slice of Labels to a Map and retrieves the measure value name.
func convertToMap(labels []*prompb.Label) (map[string]string, string) {
	// measureValueName is the Prometheus metric name that maps to MeasureName of a wtypes.Record
	var measureValueName string

	metric := make(map[string]string, len(labels))
	for _, label := range labels {
		metric[label.Name] = label.Value
	}
	measureValueName = metric[model.MetricNameLabel]
	delete(metric, model.MetricNameLabel)

	return metric, measureValueName
}

// appendRecords converts each valid Prometheus Sample to a Timestream Record and append the Record to the given slice of records.
func (wc *WriteClient) appendRecords(records []wtypes.Record, timeSeries *prompb.TimeSeries, dimensions []wtypes.Dimension, measureValueName string) ([]wtypes.Record, error) {
	var operationOnInvalidSample func(timeSeriesValue float64) (labelOperation, error)
	if wc.failOnInvalidSample {
		operationOnInvalidSample = func(timeSeriesValue float64) (labelOperation, error) {
			if math.IsNaN(timeSeriesValue) || math.IsInf(timeSeriesValue, 0) {
				// Log and fail on samples with non-finite values.
				err := errors.NewInvalidSampleValueError(timeSeriesValue)
				LogError(wc.logger, "Timestream only accepts finite IEEE Standard 754 floating-point precision. Non-finite sample value will fail the program with fail-on-invalid-sample-value enabled.", err, "timeSeries", timeSeries)
				return failed, err
			}
			return unmodified, nil
		}
	} else {
		operationOnInvalidSample = func(timeSeriesValue float64) (labelOperation, error) {
			if math.IsNaN(timeSeriesValue) || math.IsInf(timeSeriesValue, 0) {
				// Log and ignore; continue to the next sample.
				wc.ignoredSamples.Inc()
				LogDebug(wc.logger, "Timestream only accepts finite IEEE Standard 754 floating point precision. Samples with NaN, Inf and -Inf are ignored.", "timeSeries", timeSeries)
				return ignored, nil
			}
			return unmodified, nil
		}
	}

	for _, sample := range timeSeries.Samples {
		// sample.Value is the measured value of a metric which maps to the MeasureValue in wtypes.Record
		timeSeriesValue := sample.Value
		operation, err := operationOnInvalidSample(timeSeriesValue)

		switch operation {
		case failed:
			return records, err
		case ignored:
			continue
		default:
		}

		records = append(records, wtypes.Record{
			Dimensions:       dimensions,
			MeasureName:      aws.String(measureValueName),
			MeasureValue:     aws.String(strconv.FormatFloat(timeSeriesValue, 'f', 6, 64)),
			MeasureValueType: wtypes.MeasureValueTypeDouble,
			Time:             aws.String(strconv.FormatInt(sample.Timestamp, 10)),
			TimeUnit:         wtypes.TimeUnitMilliseconds,
		})
	}

	return records, nil
}

// buildCommands builds a list of queries from the given Prometheus queries.
func (qc *QueryClient) buildCommands(queries []*prompb.Query) ([]*timestreamquery.QueryInput, bool, error) {
	var timestreamQueries []*timestreamquery.QueryInput
	var isRelatedToRegex = false
	for _, query := range queries {
		var matcherName string
		var matchers []string
		for _, matcher := range query.Matchers {
			switch matcher.Name {
			case model.MetricNameLabel:
				matcherName = measureNameColumnName
			default:
				matcherName = matcher.Name
			}

			switch matcher.Type {
			case prompb.LabelMatcher_EQ:
				matchers = append(matchers, fmt.Sprintf("%s = '%s'", matcherName, matcher.Value))
			case prompb.LabelMatcher_NEQ:
				matchers = append(matchers, fmt.Sprintf("%s != '%s'", matcherName, matcher.Value))
			case prompb.LabelMatcher_RE:
				matchers = append(matchers, fmt.Sprintf("REGEXP_LIKE(%s, '%s')", matcherName, matcher.Value))
				isRelatedToRegex = true
			case prompb.LabelMatcher_NRE:
				matchers = append(matchers, fmt.Sprintf("NOT REGEXP_LIKE(%s, '%s')", matcherName, matcher.Value))
				isRelatedToRegex = true
			default:
				err := errors.NewUnknownMatcherError()
				LogError(qc.logger, "Invalid query with unknown matcher.", err)
				return nil, isRelatedToRegex, err
			}
		}

		if len(qc.client.defaultDataBase) == 0 {
			err := errors.NewMissingDatabaseError(qc.client.defaultDataBase)
			LogError(qc.logger, "The database name must be set through the --default-database flag.", err)
			return nil, isRelatedToRegex, err
		}

		if len(qc.client.defaultTable) == 0 {
			err := errors.NewMissingTableError(qc.client.defaultTable)
			LogError(qc.logger, "The table name must set through the --default-table flag.", err)
			return nil, isRelatedToRegex, err
		}

		if query.GetHints() != nil {
			matchers = append(matchers, fmt.Sprintf("%s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)", timeColumnName, query.GetHints().StartMs/millisToSecConversionRate, query.GetHints().EndMs/millisToSecConversionRate))
		} else {
			matchers = append(matchers, fmt.Sprintf("%s BETWEEN FROM_UNIXTIME(%d) AND FROM_UNIXTIME(%d)", timeColumnName, query.StartTimestampMs/millisToSecConversionRate, query.EndTimestampMs/millisToSecConversionRate))
		}

		timestreamQueries = append(timestreamQueries, &timestreamquery.QueryInput{
			QueryString: aws.String(fmt.Sprintf("SELECT * FROM %s.%s WHERE %v", qc.client.defaultDataBase, qc.client.defaultTable, strings.Join(matchers, " AND "))),
		})
	}

	return timestreamQueries, isRelatedToRegex, nil
}

// convertToResult converts the Timestream QueryOutput to Prometheus QueryResult.
func (qc *QueryClient) convertToResult(results *prompb.QueryResult, page *timestreamquery.QueryOutput) (*prompb.QueryResult, error) {
	var timeSeries []*prompb.TimeSeries
	rows := page.Rows

	if len(rows) == 0 {
		LogInfo(qc.logger, "No results returned for the PromQL.")
		return results, nil
	}

	for _, row := range rows {

		labels, samples, err := qc.constructLabels(row.Data, page.ColumnInfo)
		if err != nil {
			LogDebug(qc.logger, "Error occurred when constructing Prometheus Labels from Timestream QueryOutput with Row", "row", row)
			return results, err
		}
		timeSeries = constructTimeSeries(labels, samples, timeSeries)
	}

	results.Timeseries = append(results.Timeseries, timeSeries...)
	return results, nil
}

// constructLabels converts the given row to the corresponding Prometheus Label and Sample.
func (qc *QueryClient) constructLabels(row []qtypes.Datum, metadata []qtypes.ColumnInfo) ([]*prompb.Label, prompb.Sample, error) {
	var labels []*prompb.Label
	var sample prompb.Sample

	for i, datum := range row {

		if datum.NullValue == nil {
			column := metadata[i]
			switch *column.Name {
			case timeColumnName:
				timestamp, err := time.Parse(timestampLayout, *datum.ScalarValue)
				if err != nil {
					err := fmt.Errorf("error occurred while parsing '%s' as a timestamp", *datum.ScalarValue)
					LogError(qc.logger, "Invalid datum type retrieved from Timestream", err)
					return labels, sample, err
				}
				sample.Timestamp = timestamp.UnixNano() / nanosToMillisConversionRate

			case measureValueColumnName:
				val, err := strconv.ParseFloat(*datum.ScalarValue, 64)
				if err != nil {
					err := fmt.Errorf("error occurred while parsing '%s' as a float", *datum.ScalarValue)
					LogError(qc.logger, "Invalid datum type retrieved from Timestream", err)
					return labels, sample, err
				}
				sample.Value = val

			case measureNameColumnName:
				labels = append(labels, &prompb.Label{
					Name:  model.MetricNameLabel,
					Value: *datum.ScalarValue,
				})

			default:
				labels = append(labels, &prompb.Label{
					Name:  *column.Name,
					Value: *datum.ScalarValue,
				})
			}
		}
	}
	return labels, sample, nil
}

// constructTimeSeries constructs a TimeSeries in the query result.
func constructTimeSeries(labels []*prompb.Label, sample prompb.Sample, currentTimeSeries []*prompb.TimeSeries) []*prompb.TimeSeries {
	// anyMatch records if the label match one of the labels in current TimeSeries.
	anyMatch := false
	for _, timeSeries := range currentTimeSeries {
		if compareLabels(timeSeries.GetLabels(), labels) {
			timeSeries.Samples = append(timeSeries.GetSamples(), sample)
			anyMatch = true
			break
		}
	}

	if !anyMatch {
		currentTimeSeries = addNewTimeSeries(currentTimeSeries, labels, sample)
	}

	return currentTimeSeries
}

// addNewTimeSeries adds a new TimeSeries to the current slice of TimeSeries.
func addNewTimeSeries(currentTimeSeries []*prompb.TimeSeries, labels []*prompb.Label, sample prompb.Sample) []*prompb.TimeSeries {
	currentTimeSeries = append(
		currentTimeSeries,
		&prompb.TimeSeries{
			Labels:  labels,
			Samples: []prompb.Sample{sample},
		})
	return currentTimeSeries
}

// compareLabels compares two slices of labels with each label name and value. If they are equal, return true. Otherwise, return false.
func compareLabels(labels1 []*prompb.Label, labels2 []*prompb.Label) bool {
	if len(labels1) != len(labels2) {
		return false
	}
	for i, label := range labels1 {
		if label.Name != labels2[i].Name || label.Value != labels2[i].Value {
			return false
		}
	}
	return true
}

// Name gets the name of the query client.
func (qc QueryClient) Name() string {
	return "Timestream query client"
}

// Name gets the name of the write client.
func (wc WriteClient) Name() string {
	return "Timestream write client"
}

// QueryClient gets the query client.
func (c *Client) QueryClient() *QueryClient {
	return c.queryClient
}

// WriteClient gets the write client.
func (c *Client) WriteClient() *WriteClient {
	return c.writeClient
}

// Describe implements prometheus.Collector.
func (c *Client) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.writeClient.ignoredSamples.Desc()
	ch <- c.writeClient.receivedSamples.Desc()
	ch <- c.writeClient.writeExecutionTime.Desc()
	ch <- c.writeClient.writeRequests.Desc()
	ch <- c.queryClient.readRequests.Desc()
	ch <- c.queryClient.readExecutionTime.Desc()
}

// Collect implements prometheus.Collector.
func (c *Client) Collect(ch chan<- prometheus.Metric) {
	ch <- c.writeClient.ignoredSamples
	ch <- c.writeClient.receivedSamples
	ch <- c.writeClient.writeExecutionTime
	ch <- c.writeClient.writeRequests
	ch <- c.queryClient.readRequests
	ch <- c.queryClient.readExecutionTime
}

// Get the value of a counter
func getCounterValue(collector prometheus.Collector) int {
	channel := make(chan prometheus.Metric, 1) // 1 denotes no Vector
	collector.Collect(channel)
	metric := prometheusClientModel.Metric{}
	_ = (<-channel).Write(&metric)
	return int(*metric.Counter.Value)
}

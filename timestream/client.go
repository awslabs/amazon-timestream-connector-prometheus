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
// 1. converts the Prometheus write requests to Amazon Timestream records;
// 2. sends the records to Amazon Timestream through the write clients;
package timestream

import (
	"fmt"
	"os"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/timestreamwrite"
	"github.com/aws/aws-sdk-go/service/timestreamwrite/timestreamwriteiface"
	"github.com/go-kit/kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/prompb"
	"math"
	"strconv"
	"time"
	"timestream-prometheus-connector/errors"
)

type labelOperation string
type longMetricsOperation func(measureValueName string) (labelOperation, error)

// Store the initialization function calls to allow unit tests to mock the creation of real clients.
var initWriteClient = func(config *aws.Config, endpoint string) (timestreamwriteiface.TimestreamWriteAPI, error) {
	sess, err := session.NewSession(config)
	if err != nil {
		return nil, err
	}
	tcfg := &aws.Config{}
	if endpoint != "" {
		tcfg.Endpoint = aws.String(endpoint)
	}
	wclient := timestreamwrite.New(sess, tcfg)
	// Add the user agent version
	useragent := fmt.Sprintf("PrometheusTimestream/%s/%s", os.Getenv("AWS_LAMBDA_FUNCTION_NAME"), Version)
	wclient.Handlers.Send.PushFront(func(r *request.Request) {
		r.HTTPRequest.Header.Set("User-Agent", useragent)
	})
	return wclient, nil
}

// recordDestinationMap is a nested map that stores slices of Records based on the ingestion destination.
// Below is an example of the map structure:
// records := map[string]map[string][]*timestreamwrite.Record{
// 		"database1": map[string][]*timestreamwrite.Record{
// 				"table1":[]*timestreamwrite.Record{record1, record2},
// 				"table2":[]*timestreamwrite.Record{record3},
// 		},
// 		"database2": map[string]string{
// 				"table3":[]*timestreamwrite.Record{record4, record5},
// 				"table4":[]*timestreamwrite.Record{record6},
// 		},
// }
type recordDestinationMap map[string]map[string][]*timestreamwrite.Record

const (
	maxMeasureNameLength        int            = 60
	ignored                     labelOperation = "Ignored"
	failed                      labelOperation = "Failed"
	unmodified                  labelOperation = "Unmodified"
)

type WriteClient struct {
	client                    *Client
	config                    *aws.Config
	logger                    log.Logger
	ignoredSamples            prometheus.Counter
	receivedSamples           prometheus.Counter
	writeRequests             prometheus.Counter
	writeExecutionTime        prometheus.Histogram
	timestreamWrite           timestreamwriteiface.TimestreamWriteAPI
	failOnLongMetricLabelName bool
	failOnInvalidSample       bool
	endpoint                  string
}

type Client struct {
	writeClient   *WriteClient
	databaseLabel string
	tableLabel    string
}

// NewBaseClient creates a Timestream Client object with the ingestion destination labels.
func NewBaseClient(databaseLabel, tableLabel string) *Client {
	client := &Client{
		databaseLabel: databaseLabel,
		tableLabel:    tableLabel,
	}

	return client
}

// NewWriteClient creates a new Timestream write client with a given set of configurations.
func (c *Client) NewWriteClient(logger log.Logger, configs *aws.Config, failOnLongMetricLabelName bool, failOnInvalidSample bool, endpoint string) {
	c.writeClient = &WriteClient{
		client:                    c,
		logger:                    logger,
		config:                    configs,
		failOnLongMetricLabelName: failOnLongMetricLabelName,
		failOnInvalidSample:       failOnInvalidSample,
		endpoint:                  endpoint,
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
func (wc *WriteClient) Write(req *prompb.WriteRequest, credentials *credentials.Credentials) ([3]int64, error) {
	wc.config.Credentials = credentials
	var err error
	nilresp := [3]int64{0, 0, 0}
	if wc.endpoint != "" {
		LogInfo(wc.logger, "Ingest Endpoint: ", wc.endpoint)
	}
	wc.timestreamWrite, err = initWriteClient(wc.config, wc.endpoint)
	if err != nil {
		LogError(wc.logger, "Unable to construct a new session with the given credentials", err)
		return nilresp, err
	}

	recordMap := make(recordDestinationMap)
	recordMap, err = wc.convertToRecords(req.Timeseries, recordMap)
	if err != nil {
		LogError(wc.logger, "Unable to convert the received Prometheus write request to Timestream Records.", err)
		return nilresp, err
	}

	var sdkErr error
	var ctime int64
	var recordlen int64
	var numWrites int64
	for database, tableMap := range recordMap {
		for table, records := range tableMap {
			writeRecordsInput := &timestreamwrite.WriteRecordsInput{
				DatabaseName: aws.String(database),
				TableName:    aws.String(table),
				Records:      records,
			}
			begin := time.Now()
			_, err = wc.timestreamWrite.WriteRecords(writeRecordsInput)
			duration := time.Since(begin).Seconds()
			ctime += int64(time.Since(begin))
			recordlen += int64(len(records))
			numWrites += int64(1)
			if err != nil {
				sdkErr = wc.handleSDKErr(req, err, sdkErr)
			}
			wc.writeExecutionTime.Observe(duration)
			wc.writeRequests.Inc()
		}
	}

	resp := [3]int64{ctime, recordlen, numWrites}
	return resp, sdkErr
}

// handleSDKErr parses and logs the error from SDK (if any)
func (wc *WriteClient) handleSDKErr(req *prompb.WriteRequest, currErr error, errToReturn error) error {
	requestError, ok := currErr.(awserr.RequestFailure)
	if !ok {
		LogError(wc.logger, "Error occurred while ingesting Timestream Records.", currErr)
		return errors.NewSDKNonRequestError(currErr)
	}

	if errToReturn == nil {
		errToReturn = requestError
	}
	switch requestError.StatusCode() / 100 {
	case 4:
		LogDebug(wc.logger, "Error occurred while ingesting data due to invalid write request. Some Prometheus Samples were not ingested into Timestream, please review the write request and check the documentation for troubleshooting.", "request", req)
	case 5:
		errToReturn = requestError
		LogDebug(wc.logger, "Internal server error occurred. Samples will be retried by Prometheus", "request", req)
	}
	return errToReturn
}

// convertToRecords converts a slice of *prompb.TimeSeries to a slice of *timestreamwrite.Record
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

// processTimeSeries processes a slice of *prompb.TimeSeries to a slice of *timestreamwrite.Record
func processTimeSeries(wc *WriteClient, operationOnLongMetrics longMetricsOperation, series []*prompb.TimeSeries, recordMap recordDestinationMap) (recordDestinationMap, error) {
	var numRecords int
	for _, timeSeries := range series {
		var dimensions []*timestreamwrite.Dimension
		var err error
		var operation labelOperation
		wc.receivedSamples.Add(float64(len(timeSeries.Samples)))

		metricLabels, measureValueName := convertToMap(timeSeries.Labels)

		databaseName := metricLabels[wc.client.databaseLabel]
		tableName := metricLabels[wc.client.tableLabel]

		if len(databaseName) == 0 {
			err = errors.NewMissingDatabaseWithWriteError(wc.client.databaseLabel, timeSeries)
			return nil, err
		}

		if len(tableName) == 0 {
			err = errors.NewMissingTableWithWriteError(wc.client.tableLabel, timeSeries)
			return nil, err
		}

		delete(metricLabels, wc.client.databaseLabel)
		delete(metricLabels, wc.client.tableLabel)

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

		var records []*timestreamwrite.Record

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

		numRecords += 1
		recordMap[databaseName][tableName] = records

	}
	LogInfo(wc.logger, "Number of records: ", numRecords)
	return recordMap, nil
}

// processMetricLabels processes metricLabels to a *timestreamwrite.Record
func processMetricLabels(metricLabels map[string]string, operationOnLongMetrics longMetricsOperation) ([]*timestreamwrite.Dimension, labelOperation, error) {
	var operation labelOperation
	var dimensions []*timestreamwrite.Dimension
	var err error
	for name, value := range metricLabels {
		// Each label in the metricLabels map contains a characteristic/dimension of the metric, which maps to timestreamwrite.Dimension
		operation, err = operationOnLongMetrics(name)
		switch operation {
		case failed:
			return nil, operation, err
		case ignored:
			return nil, operation, nil
		default:
			dimensions = append(dimensions, &timestreamwrite.Dimension{
				Name:  aws.String(name),
				Value: aws.String(value),
			})
		}
	}
	return dimensions, operation, nil
}

// getOrCreateRecordMapEntry gets record map entry
func getOrCreateRecordMapEntry(recordMap recordDestinationMap, databaseName string) map[string][]*timestreamwrite.Record {
	if recordMap[databaseName] == nil {
		recordMap[databaseName] = make(map[string][]*timestreamwrite.Record)
	}
	return recordMap[databaseName]
}

// convertToMap converts the slice of Labels to a Map and retrieves the measure value name.
func convertToMap(labels []*prompb.Label) (map[string]string, string) {
	// measureValueName is the Prometheus metric name that maps to MeasureName of a timestreamwrite.Record
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
func (wc *WriteClient) appendRecords(records []*timestreamwrite.Record, timeSeries *prompb.TimeSeries, dimensions []*timestreamwrite.Dimension, measureValueName string) ([]*timestreamwrite.Record, error) {
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
		// sample.Value is the measured value of a metric which maps to the MeasureValue in timestreamwrite.Record
		timeSeriesValue := sample.Value
		operation, err := operationOnInvalidSample(timeSeriesValue)

		switch operation {
		case failed:
			return records, err
		case ignored:
			continue
		default:
		}

		records = append(records, &timestreamwrite.Record{
			Dimensions:       dimensions,
			MeasureName:      aws.String(measureValueName),
			MeasureValue:     aws.String(strconv.FormatFloat(timeSeriesValue, 'f', 6, 64)),
			MeasureValueType: aws.String(timestreamwrite.MeasureValueTypeDouble),
			Time:             aws.String(strconv.FormatInt(sample.Timestamp, 10)),
			TimeUnit:         aws.String(timestreamwrite.TimeUnitMilliseconds),
		})
	}

	return records, nil
}

// Name gets the name of the write client.
func (wc WriteClient) Name() string {
	return "Timestream write client"
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
}

// Collect implements prometheus.Collector.
func (c *Client) Collect(ch chan<- prometheus.Metric) {
	ch <- c.writeClient.ignoredSamples
	ch <- c.writeClient.receivedSamples
	ch <- c.writeClient.writeExecutionTime
	ch <- c.writeClient.writeRequests
}

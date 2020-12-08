/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

package errors

import (
	"fmt"
	"github.com/prometheus/prometheus/prompb"
	"net/http"
)

type baseConnectorError struct {
	error
	statusCode int
	errorMsg   string
	message    string
}

func (e *baseConnectorError) StatusCode() int {
	return e.statusCode
}

func (e *baseConnectorError) Error() string {
	return e.errorMsg
}

func (e *baseConnectorError) Message() string {
	return e.message
}

type MissingDestinationError struct {
	baseConnectorError
}

func NewMissingDestinationError() error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   "no database label or table label provided",
		message: "The environment variables database-label and table-label must be specified in the Lambda function. " +
			labelErrorMessage,
	}
	return &MissingDestinationError{baseConnectorError: base}
}

type ParseEnableLoggingError struct {
	baseConnectorError
}

func NewParseEnableLoggingError(enableLogging string) error {
	return &ParseEnableLoggingError{baseConnectorError: baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("error occurred while parsing enable-logging, expected true or false, but received '%s'", enableLogging),
		message: "The value specified in the enable-logging option is not one of the accepted values. " +
			acceptedValueErrorMessage,
	}}
}

type ParseMetricLabelError struct {
	baseConnectorError
}

func NewParseMetricLabelError(failOnLongMetricLabelName string) error {
	return &ParseMetricLabelError{baseConnectorError: baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("error occurred while parsing fail-on-long-label, expected true or false, but received '%s'", failOnLongMetricLabelName),
		message: "The value specified in the fail-on-long-label option is not one of the accepted values. " +
			acceptedValueErrorMessage,
	}}
}

type ParseSampleOptionError struct {
	baseConnectorError
}

func NewParseSampleOptionError(failOnInvalidSample string) error {
	return &ParseSampleOptionError{baseConnectorError: baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("error occurred while parsing fail-on-invalid-sample, expected true or false, but received '%s'", failOnInvalidSample),
		message: "The value specified in the fail-on-invalid-sample option is not one of the accepted values. " +
			acceptedValueErrorMessage,
	}}
}

type ParseRetriesError struct {
	baseConnectorError
}

func NewParseRetriesError(retries string) error {
	return &ParseRetriesError{baseConnectorError: baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("error occurred while parsing max-retries, expected an integer, but received '%s'", retries),
		message: "The value specified in the max-retries option is not one of the accepted values. " +
			acceptedValueErrorMessage,
	}}
}

type MissingHeaderError struct {
	baseConnectorError
}

func NewMissingHeaderError(readHeader, writeHeader string) error {
	return &MissingHeaderError{baseConnectorError: baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("No appropriate header found in the request. Please ensure the request header contains either %s or %s.", readHeader, writeHeader),
		message:    fmt.Sprintf("The request must contains either %s or %s in the header.", readHeader, writeHeader),
	}}
}

type MissingDatabaseWithWriteError struct {
	baseConnectorError
}

func NewMissingDatabaseWithWriteError(databaseLabel string, timeSeries *prompb.TimeSeries) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("the given database label name: %s cannot be found in the slice of Labels for the current time series %v", databaseLabel, timeSeries),
		message: "The environment variables database-label must be specified in the Prometheus time series labels. " +
			labelErrorMessage,
	}
	return &MissingDatabaseWithWriteError{baseConnectorError: base}
}

type MissingTableWithWriteError struct {
	baseConnectorError
}

func NewMissingTableWithWriteError(tableLabel string, timeSeries *prompb.TimeSeries) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("the given table label name: %s cannot be found in the slice of Labels for the current time series %v", tableLabel, timeSeries),
		message: "The environment variables table-label must be specified in the Prometheus time series labels. " +
			labelErrorMessage,
	}
	return &MissingTableWithWriteError{baseConnectorError: base}
}

type MissingDatabaseWithQueryError struct {
	baseConnectorError
}

func NewMissingDatabaseWithQueryError(databaseLabel string) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("no Timestream database is specified in the query, please provide the database in the PromQL as a label matcher {%s=\"<databaseName>\"}", databaseLabel),
		message: "The environment variables database-label must be specified in the PromQL when sending a query request. " +
			labelErrorMessage,
	}
	return &MissingDatabaseWithQueryError{baseConnectorError: base}
}

type MissingTableWithQueryError struct {
	baseConnectorError
}

func NewMissingTableWithQueryError(tableLabel string) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("no Timestream table is specified in the query, please provide the table in the PromQL as a label matcher {%s=\"<tableName>\"}", tableLabel),
		message: "The environment variables table-label must be specified when sending a query request. " +
			labelErrorMessage,
	}
	return &MissingTableWithQueryError{baseConnectorError: base}
}

type UnknownMatcherError struct {
	baseConnectorError
}

func NewUnknownMatcherError() error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   "unknown matcher in query, Prometheus only supports 4 types of matchers in the filter: =, !=, =~, !~",
		message:    "Prometheus only supports 4 types of matchers in the filter: =, !=, =~, !~, others matchers will be invalid. ",
	}
	return &UnknownMatcherError{baseConnectorError: base}
}

type LongLabelNameError struct {
	baseConnectorError
}

func NewLongLabelNameError(measureValueName string, maxMeasureNameLength int) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("metric name '%s' exceeds %d characters, the maximum length supported by Timestream", measureValueName, maxMeasureNameLength),
		message: "The metric name exceeds the maximum Timestream supported length, and the `fail-on-long-label` is set to  `true`. " +
			detailsErrorMessage,
	}
	return &LongLabelNameError{baseConnectorError: base}
}

type InvalidSampleValueError struct {
	baseConnectorError
}

func NewInvalidSampleValueError(timeSeriesValue float64) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("invalid sample value: %f", timeSeriesValue),
		message: "Timestream only accepts finite IEEE Standard 754 floating-point precision. " +
			"Non-finite sample value will fail the program with fail-on-invalid-sample-value enabled.",
	}
	return &InvalidSampleValueError{baseConnectorError: base}
}

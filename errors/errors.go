/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file contains all the error messages and their HTTP status code the Prometheus Connector may return.
package errors

import (
	"fmt"
	"github.com/prometheus/prometheus/prompb"
	"net/http"
)

type baseConnectorError struct {
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

type ParseBasicAuthHeaderError struct {
	baseConnectorError
}

func NewParseBasicAuthHeaderError() error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   "expected a valid AWS credentials, please check Prometheus configuration for basic auth",
		message:    "The request must contain a valid basic authentication header, please refer to the documentation on how to configure Prometheus.",
	}
	return &ParseBasicAuthHeaderError{baseConnectorError: base}
}

type MissingHeaderError struct {
	baseConnectorError
}

func NewMissingHeaderError(writeHeader string) error {
	return &MissingHeaderError{baseConnectorError: baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   fmt.Sprintf("No appropriate header found in the request. Please ensure the request header contains %s.", writeHeader),
		message:    fmt.Sprintf("The request must contain %s in the header.", writeHeader),
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

type SDKNonRequestError struct {
	baseConnectorError
}

func NewSDKNonRequestError(err error) error {
	base := baseConnectorError{
		statusCode: http.StatusBadRequest,
		errorMsg:   err.Error(),
		message:    err.Error(),
	}
	return &SDKNonRequestError{baseConnectorError: base}
}

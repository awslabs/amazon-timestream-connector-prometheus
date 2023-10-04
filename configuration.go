/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file contains all the standalone and AWS Lambda configuration options for Prometheus Connector, allowing main.go
// to easily reference them when retrieving and parsing the options from the command line or environment variables.
package main

import (
	awsClient "github.com/aws/aws-sdk-go/aws/client"
	"strconv"
)

type configuration struct {
	flag         string
	envFlag      string
	defaultValue string
}

var (
	enableLogConfig           = &configuration{flag: "enable-logging", envFlag: "enable_logging", defaultValue: "true"}
	regionConfig              = &configuration{flag: "region", envFlag: "region", defaultValue: "us-east-1"}
	maxRetriesConfig          = &configuration{flag: "max-retries", envFlag: "max_retries", defaultValue: strconv.Itoa(awsClient.DefaultRetryerMaxNumRetries)}
	defaultDatabaseConfig     = &configuration{flag: "default-database", envFlag: "default_database", defaultValue: ""}
	defaultTableConfig        = &configuration{flag: "default-table", envFlag: "default_table", defaultValue: ""}
	databaseLabelConfig       = &configuration{flag: "database-label", envFlag: "database_label", defaultValue: ""}
	tableLabelConfig          = &configuration{flag: "table-label", envFlag: "table_label", defaultValue: ""}
	listenAddrConfig          = &configuration{flag: "web.listen-address", envFlag: "", defaultValue: ":9201"}
	telemetryPathConfig       = &configuration{flag: "web.telemetry-path", envFlag: "", defaultValue: "/metrics"}
	failOnLabelConfig         = &configuration{flag: "fail-on-long-label", envFlag: "fail_on_long_label", defaultValue: "false"}
	failOnInvalidSampleConfig = &configuration{flag: "fail-on-invalid-sample-value", envFlag: "fail_on_invalid_sample_value", defaultValue: "false"}
	promlogLevelConfig        = &configuration{flag: "log.level", envFlag: "log_level", defaultValue: "info"}
	promlogFormatConfig       = &configuration{flag: "log.format", envFlag: "log_format", defaultValue: "logfmt"}
	certificateConfig         = &configuration{flag: "tls-certificate", envFlag: "", defaultValue: ""}
	keyConfig                 = &configuration{flag: "tls-key", envFlag: "", defaultValue: ""}
)

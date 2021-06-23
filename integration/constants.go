/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file contains the constants used in the integration test.
package integration

const (
	databaseLabel         = "timestreamDatabaseName"
	tableLabel            = "timestreamTableName"
	database              = "integrationDB"
	table                 = "integration"
	database2             = "integrationDB2"
	table2                = "integration2"
	region                = "us-east-1"
	writeMetricName       = "write_metric"
	value                 = 1.0
	numRecords            = 100
	memStoreRetentionHour = 5
)

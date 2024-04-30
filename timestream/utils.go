/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file contains helper methods for logging messages.
package timestream

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
)

// LogError logs the provided error with the given message.
func LogError(logger log.Logger, msg string, err error, keyvals ...interface{}) {
	level.Error(logger).Log(append([]interface{}{"message", msg}, keyvals...)...)
	level.Debug(logger).Log(err)
}

// LogDebug logs at DEBUG level with the given message and any additional key-value pairs.
func LogDebug(logger log.Logger, message string, keyvals ...interface{}) {
	level.Debug(logger).Log(append([]interface{}{"message", message}, keyvals...)...)
}

// LogInfo logs at INFO level with the given message and any additional key-value pairs.
func LogInfo(logger log.Logger, message string, keyvals ...interface{}) {
	level.Info(logger).Log(append([]interface{}{"message", message}, keyvals...)...)
}

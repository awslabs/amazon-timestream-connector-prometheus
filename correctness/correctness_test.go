/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This test suite validates the correctness of the Prometheus Connector using Mockmetheus
// to execute remote-read and remote-write operations. It covers various scenarios by
// executing queries against a locally hosted Connector connected to a Timestream database.
package correctness

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"
)

// Enable this flag when working with a fresh Timestream database.
var freshTSDB bool

// ingestionWaitTime is the duration to wait for data ingestion.
var ingestionWaitTime time.Duration

// Shared Mockmetheus instance
var m *Mockmetheus

func init() {
	flag.BoolVar(&freshTSDB, "freshTSDB", true, "Use a fresh Timestream DB")
	flag.DurationVar(&ingestionWaitTime, "ingestionWaitTime", 1*time.Second, "Delay to wait for data ingestion.")
}

func TestMain(main *testing.M) {
	flag.Parse()

	var err error
	m, err = NewMockmetheus("http://0.0.0.0:9201")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize Mockmetheus: %v\n", err)
		os.Exit(1)
	}

	code := main.Run()

	os.Exit(code)
}

func TestEmptyOnInit(t *testing.T) {
	ctx := context.Background()
	var query string

	if freshTSDB {
		// Test empty on initialization
		query = "prometheus_http_requests_total{}"
	} else {
		// For running against an existing Timestream DB, wait for a few
		// seconds to avoid conflicting results
		time.Sleep(3 * time.Second)
		query = "prometheus_http_requests_total{}[3s]"
	}

	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}

	if !isEmpty(resp) {
		t.Errorf("expected empty results but got non-empty")
	}
}

func TestReadMetricDNE(t *testing.T) {
	ctx := context.Background()

	query := "non_existent_metric"
	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if !isEmpty(resp) {
		t.Errorf("expected empty results for DNE metric but got non-empty")
	}
}

func TestReadLabelDNE(t *testing.T) {
	ctx := context.Background()

	metric := "non_existent_metric"
	label := "some_label_value"
	query := fmt.Sprintf(`%s{non_existent_label="%s"}`, metric, label)
	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if !isEmpty(resp) {
		t.Errorf("expected empty results for DNE metric but got non-empty")
	}
}

func TestWriteNoData(t *testing.T) {
	ctx := context.Background()

	var data []TimeSeriesData // empty
	err := m.RemoteWrite(ctx, data)
	if err != nil {
		t.Fatalf("RemoteWrite returned an error: %v", err)
	}
}

func TestWriteNoLabels(t *testing.T) {
	ctx := context.Background()

	data := []TimeSeriesData{
		{
			Labels: map[string]string{},
			Samples: []SampleData{
				{Value: 220, Timestamp: time.Now().UnixMilli()},
			},
		},
	}
	err := m.RemoteWrite(ctx, data)
	if err == nil {
		t.Fatal("expected an error (status code 400), but got nil")
	}
	if !strings.Contains(err.Error(), "status code 400") {
		t.Errorf("expected status code 400 error, got: %v", err)
	}
}

func TestWriteNoSamples(t *testing.T) {
	ctx := context.Background()

	name := "prometheus_http_requests_total"
	instance := "mockmetheus"

	data := []TimeSeriesData{
		{
			Labels: map[string]string{
				"__name__": name,
				"instance": instance,
			},
			Samples: []SampleData{}, // no samples
		},
	}
	if err := m.RemoteWrite(ctx, data); err != nil {
		t.Fatalf("RemoteWrite error: %v", err)
	}

	// Wait to ensure ingestion
	time.Sleep(ingestionWaitTime)

	// query for within the last 2 seconds, accounting for ingestion delay
	waitSeconds := int(ingestionWaitTime.Seconds()) + 2
	query := fmt.Sprintf(`%s{instance="%s"}[%ds]`, name, instance, waitSeconds)
	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if !isEmpty(resp) {
		t.Errorf("expected empty results but got non-empty")
	}
}

func TestSuccess(t *testing.T) {
	ctx := context.Background()

	name := "prometheus_http_requests_total"
	instance := "mockmetheus"
	expectedSample := 220.0
	testID := generateTestRunID()

	data := []TimeSeriesData{
		{
			Labels: map[string]string{
				"__name__": name,
				"instance": instance,
				"test_id":  testID,
			},
			Samples: []SampleData{
				{Value: expectedSample, Timestamp: time.Now().UnixMilli()},
			},
		},
	}
	if err := m.RemoteWrite(ctx, data); err != nil {
		t.Fatalf("RemoteWrite error: %v", err)
	}

	time.Sleep(ingestionWaitTime)

	waitSeconds := int(ingestionWaitTime.Seconds()) + 2
	query := fmt.Sprintf(`%s{instance="%s", test_id="%s"}[%ds]`, name, instance, testID, waitSeconds)
	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp) {
		t.Errorf("expected non-empty results but got empty")
	}

	// Validate the returned value by parsing out the time series
	v, err := getFirstSampleValue(resp)
	if err != nil {
		t.Fatalf("error getting sample value: %v", err)
	}
	if v != expectedSample {
		t.Errorf("expected sample value %.2f, got %.2f", expectedSample, v)
	}
}

func TestSuccessWriteMultipleMetrics(t *testing.T) {
	ctx := context.Background()

	name := "prometheus_http_requests_total"
	handler := "/api/v1/query"
	instance := "mockmetheus"
	job := "prometheus"
	sample1 := 300.0

	name2 := "mockmetheus_custom_metric"
	sample2 := 400.0
	testID := generateTestRunID()

	data := []TimeSeriesData{
		{
			Labels: map[string]string{
				"__name__": name,
				"handler":  handler,
				"instance": instance,
				"test_id":  testID,
				"job":      job,
			},
			Samples: []SampleData{
				{Value: sample1, Timestamp: time.Now().UnixMilli()},
			},
		},
		{
			Labels: map[string]string{
				"__name__": name2,
				"handler":  handler,
				"instance": instance,
				"test_id":  testID,
				"job":      job,
			},
			Samples: []SampleData{
				{Value: sample2, Timestamp: time.Now().UnixMilli()},
			},
		},
	}
	if err := m.RemoteWrite(ctx, data); err != nil {
		t.Fatalf("RemoteWrite error: %v", err)
	}

	time.Sleep(ingestionWaitTime)

	waitSeconds := int(ingestionWaitTime.Seconds()) + 2

	// Query for first metric
	query1 := fmt.Sprintf(`%s{instance="%s", test_id="%s"}[%ds]`, name, instance, testID, waitSeconds)
	resp1, err := m.RemoteRead(ctx, query1)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp1) {
		t.Errorf("expected non-empty results (metric1) but got empty")
	}
	v1, err := getFirstSampleValue(resp1)
	if err != nil {
		t.Fatalf("error getting sample value (metric1): %v", err)
	}
	if v1 != sample1 {
		t.Errorf("expected sample value %.2f, got %.2f (metric1)", sample1, v1)
	}

	// Query for second metric
	query2 := fmt.Sprintf(`%s{instance="%s", job="%s", test_id="%s"}[3s]`, name2, instance, job, testID)
	resp2, err := m.RemoteRead(ctx, query2)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp2) {
		t.Errorf("expected non-empty results (metric2) but got empty")
	}
	v2, err := getFirstSampleValue(resp2)
	if err != nil {
		t.Fatalf("error getting sample value (metric2): %v", err)
	}
	if v2 != sample2 {
		t.Errorf("expected sample value %.2f, got %.2f (metric2)", sample2, v2)
	}
}

func TestSuccessMultipleSamples(t *testing.T) {
	ctx := context.Background()

	name := "prometheus_http_requests_total"
	handler := "/api/v1/query"
	instance := "mockmetheus"
	job := "prometheus"
	sample1 := 300.0
	sample2 := 400.0
	testID := generateTestRunID()

	data := []TimeSeriesData{
		{
			Labels: map[string]string{
				"__name__": name,
				"handler":  handler,
				"instance": instance,
				"test_id":  testID,
				"job":      job,
			},
			Samples: []SampleData{
				{Value: sample1, Timestamp: time.Now().UnixMilli()},
				{Value: sample2, Timestamp: time.Now().UnixMilli() - 100},
			},
		},
	}
	if err := m.RemoteWrite(ctx, data); err != nil {
		t.Fatalf("RemoteWrite error: %v", err)
	}

	time.Sleep(ingestionWaitTime)

	waitSeconds := int(ingestionWaitTime.Seconds()) + 2

	// Query for first metric
	query := fmt.Sprintf(`%s{handler="%s", instance="%s", job="%s", test_id="%s"}[%ds]`,
		name, handler, instance, job, testID, waitSeconds,
	)
	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp) {
		t.Errorf("expected non-empty results but got empty")
	}

	// We expect multiple samples in ascending order by timestamp
	tsSamples, err := getSampleValues(resp)
	if err != nil {
		t.Fatalf("error parsing samples: %v", err)
	}
	if len(tsSamples) != 2 {
		t.Fatalf("expected 2 samples, got %d", len(tsSamples))
	}
	if tsSamples[0] != sample2 {
		t.Errorf("expected first sample value=%.2f, got=%.2f", sample2, tsSamples[0])
	}
	if tsSamples[1] != sample1 {
		t.Errorf("expected second sample value=%.2f, got=%.2f", sample1, tsSamples[1])
	}
}

func TestSuccessLabelMatchers(t *testing.T) {
	ctx := context.Background()

	name := "prometheus_http_requests_total"
	handler := "/api/v1/query"
	instance := "mockmetheus"
	job1 := "prometheus"
	code1 := "200"
	expected1 := 100.0

	// second job & code
	job2 := "mockmetheus"
	code2 := "400"
	expected2 := 200.0
	expected3 := 300.0 // third sample for job2

	// third code
	code3 := "404"
	expected4 := 100.0

	testID := generateTestRunID()

	data := []TimeSeriesData{
		{
			// timeseries #1
			Labels: map[string]string{
				"__name__": name,
				"handler":  handler,
				"instance": instance,
				"test_id":  testID,
				"job":      job1,
				"code":     code1,
			},
			Samples: []SampleData{
				{Value: expected1, Timestamp: time.Now().UnixMilli()},
				{Value: expected2, Timestamp: time.Now().UnixMilli() - 100},
			},
		},
		{
			// timeseries #2
			Labels: map[string]string{
				"__name__": name,
				"handler":  handler,
				"instance": instance,
				"test_id":  testID,
				"job":      job2,
				"code":     code2,
			},
			Samples: []SampleData{
				{Value: expected1, Timestamp: time.Now().UnixMilli()},
				{Value: expected2, Timestamp: time.Now().UnixMilli() - 100},
				{Value: expected3, Timestamp: time.Now().UnixMilli() - 200},
			},
		},
		{
			// timeseries #3
			Labels: map[string]string{
				"__name__": name,
				"handler":  handler,
				"instance": instance,
				"test_id":  testID,
				"job":      job1,
				"code":     code3,
			},
			Samples: []SampleData{
				{Value: expected4, Timestamp: time.Now().UnixMilli()},
			},
		},
	}
	if err := m.RemoteWrite(ctx, data); err != nil {
		t.Fatalf("RemoteWrite error: %v", err)
	}
	time.Sleep(ingestionWaitTime)

	waitSeconds := int(ingestionWaitTime.Seconds()) + 2

	// NEQ matcher: job != job1
	// => Should return only timeseries #2
	query := fmt.Sprintf(`%s{job!="%s", test_id="%s"}[%ds]`, name, job1, testID, waitSeconds)
	resp, err := m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp) {
		t.Fatalf("expected non-empty results but got empty (NEQ matcher)")
	}
	numTS, numSamples := countTimeSeriesAndSamples(resp)
	if numTS != 1 {
		t.Errorf("expected 1 timeseries for job!=%s, got %d", job1, numTS)
	}
	// timeseries #2 has 3 samples
	if numSamples != 3 {
		t.Errorf("expected 3 samples in timeseries #2, got %d", numSamples)
	}

	// NRE matcher: code!~"2.."
	// => Should return any timeseries whose code does not match 2xx
	// timeseries #1 has code=200 (excluded), #2 has code=400 (included), #3 has code=404 (included)
	query = fmt.Sprintf(`%s{code!~"2..", test_id="%s"}[%ds]`, name, testID, waitSeconds)
	resp, err = m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp) {
		t.Fatalf("expected non-empty results but got empty (NRE matcher)")
	}
	numTS, _ = countTimeSeriesAndSamples(resp)
	if numTS != 2 {
		t.Errorf("expected 2 timeseries (codes 400,404), got %d", numTS)
	}

	// NEQ + NRE matcher
	// job="{job2}" AND code!~"2.."
	// => Should return timeseries #2 only
	query = fmt.Sprintf(`%s{job="%s", code!~"2..", test_id="%s"}[%ds]`, name, job2, testID, waitSeconds+1)
	resp, err = m.RemoteRead(ctx, query)
	if err != nil {
		t.Fatalf("RemoteRead error: %v", err)
	}
	if isEmpty(resp) {
		t.Fatalf("expected non-empty results but got empty (NEQ+NRE matcher)")
	}
	numTS, numSamples = countTimeSeriesAndSamples(resp)
	if numTS != 1 {
		t.Errorf("expected 1 timeseries, got %d", numTS)
	}
	if numSamples != 3 {
		t.Errorf("expected 3 samples for that timeseries, got %d", numSamples)
	}
}

// ----------------------------------------------------------------------------
// Helper functions
// ----------------------------------------------------------------------------
// Checks if the `results` array is empty or if the first result has no `timeseries`.
func isEmpty(response map[string]interface{}) bool {
	results, ok := response["results"].([]interface{})
	if !ok || len(results) == 0 {
		return true
	}

	firstResult, ok := results[0].(map[string]interface{})
	if !ok {
		return true
	}

	timeseriesList, ok := firstResult["timeseries"].([]interface{})
	if !ok || len(timeseriesList) == 0 {
		return true
	}

	return false
}

// getFirstSampleValue tries to return the first sample value from the first timeseries.
func getFirstSampleValue(response map[string]interface{}) (float64, error) {
	results, ok := response["results"].([]interface{})
	if !ok || len(results) == 0 {
		return 0, fmt.Errorf("no results found")
	}

	firstResult, ok := results[0].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid results[0] format")
	}

	timeseriesList, ok := firstResult["timeseries"].([]interface{})
	if !ok || len(timeseriesList) == 0 {
		return 0, fmt.Errorf("no timeseries in firstResult")
	}

	firstTimeseries, ok := timeseriesList[0].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid timeseries[0] format")
	}

	samplesList, ok := firstTimeseries["samples"].([]interface{})
	if !ok || len(samplesList) == 0 {
		return 0, fmt.Errorf("no samples in firstTimeseries")
	}

	firstSample, ok := samplesList[0].(map[string]interface{})
	if !ok {
		return 0, fmt.Errorf("invalid sample format")
	}

	value, ok := firstSample["value"].(float64)
	if !ok {
		return 0, fmt.Errorf("sample value not float64")
	}

	return value, nil
}

// getSampleValues returns all sample values from the first timeseries.
func getSampleValues(response map[string]interface{}) ([]float64, error) {
	var sampleValues []float64

	results, ok := response["results"].([]interface{})
	if !ok || len(results) == 0 {
		return sampleValues, fmt.Errorf("no results found")
	}

	firstResult, ok := results[0].(map[string]interface{})
	if !ok {
		return sampleValues, fmt.Errorf("invalid results[0] format")
	}

	timeseriesList, ok := firstResult["timeseries"].([]interface{})
	if !ok || len(timeseriesList) == 0 {
		return sampleValues, fmt.Errorf("no timeseries in firstResult")
	}

	firstTimeseries, ok := timeseriesList[0].(map[string]interface{})
	if !ok {
		return sampleValues, fmt.Errorf("invalid timeseries[0] format")
	}

	samplesList, ok := firstTimeseries["samples"].([]interface{})
	if !ok || len(samplesList) == 0 {
		// Return an empty slice rather than an error, since "no samples" may be valid
		return sampleValues, nil
	}

	for _, sample := range samplesList {
		sampleMap, ok := sample.(map[string]interface{})
		if !ok {
			continue
		}

		value, ok := sampleMap["value"].(float64)
		if !ok {
			continue
		}

		sampleValues = append(sampleValues, value)
	}

	return sampleValues, nil
}

// countTimeSeriesAndSamples returns the number of timeseries and total number of samples across all timeseries.
func countTimeSeriesAndSamples(response map[string]interface{}) (int, int) {
	results, ok := response["results"].([]interface{})
	if !ok || len(results) == 0 {
		return 0, 0
	}

	firstResult, ok := results[0].(map[string]interface{})
	if !ok {
		return 0, 0
	}

	timeseriesList, ok := firstResult["timeseries"].([]interface{})
	if !ok {
		return 0, 0
	}

	numTimeSeries := len(timeseriesList)
	var totalSamples int

	for _, ts := range timeseriesList {
		timeseriesMap, ok := ts.(map[string]interface{})
		if !ok {
			continue
		}
		samplesList, ok := timeseriesMap["samples"].([]interface{})
		if !ok {
			continue
		}
		totalSamples += len(samplesList)
	}

	return numTimeSeries, totalSamples
}

func generateTestRunID() string {
	rand.Seed(time.Now().UnixNano())
	alphabet := []rune("abcde12345")
	idRunes := make([]rune, 4)
	for i := 0; i < 4; i++ {
		idRunes[i] = alphabet[rand.Intn(len(alphabet))]
	}
	return string(idRunes)
}

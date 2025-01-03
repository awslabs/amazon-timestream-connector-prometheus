/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// Mockmetheus provides methods to remotely read/write Prometheus data for
// correctness testing the Prometheus connector.
package correctness

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/golang/protobuf/jsonpb"
	"github.com/golang/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/promql/parser"
)

type Mockmetheus struct {
	username     string
	password     string
	connectorURL string
	httpClient   *http.Client
}

func NewMockmetheus(connectorURL string) (*Mockmetheus, error) {
	if connectorURL == "" {
		return nil, fmt.Errorf("connectorURL cannot be empty")
	}

	cfg, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("unable to load AWS SDK config: %w", err)
	}

	creds, err := cfg.Credentials.Retrieve(context.Background())
	if err != nil {
		return nil, fmt.Errorf("unable to retrieve AWS credentials: %w", err)
	}

	username := creds.AccessKeyID
	password := creds.SecretAccessKey

	return &Mockmetheus{
		username:     username,
		password:     password,
		connectorURL: connectorURL,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (m *Mockmetheus) RemoteRead(ctx context.Context, query string) (map[string]interface{}, error) {
	rreq, err := m.constructReadRequest(query)
	if err != nil {
		return nil, err
	}

	data, err := proto.Marshal(rreq)
	if err != nil {
		return nil, err
	}
	encoded := snappy.Encode(nil, data)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.connectorURL+"/read", bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.SetBasicAuth(m.username, m.password)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("status code %d: %s", resp.StatusCode, string(bodyBytes))
	}

	bodyBytes, err := parseResponse(resp)
	if err != nil {
		return nil, err
	}

	body, err := snappy.Decode(nil, bodyBytes)
	if err != nil {
		return nil, err
	}

	var rr prompb.ReadResponse
	if err := proto.Unmarshal(body, &rr); err != nil {
		return nil, err
	}

	marshaller := &jsonpb.Marshaler{EmitDefaults: true}
	var buf bytes.Buffer
	if err := marshaller.Marshal(&buf, &rr); err != nil {
		return nil, err
	}

	var out map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		return nil, err
	}

	return out, nil
}

func (m *Mockmetheus) RemoteWrite(ctx context.Context, seriesData []TimeSeriesData) error {
	wreq := m.constructWriteRequest(seriesData)

	b, err := proto.Marshal(wreq)
	if err != nil {
		return err
	}
	encoded := snappy.Encode(nil, b)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, m.connectorURL+"/write", bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.SetBasicAuth(m.username, m.password)

	resp, err := m.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf("Server responded with status code: %d\n", resp.StatusCode)

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status code %d: %s", resp.StatusCode, string(bodyBytes))
	}
	return nil
}

type TimeSeriesData struct {
	Labels  map[string]string
	Samples []SampleData
}

type SampleData struct {
	Value     float64
	Timestamp int64
}

func (m *Mockmetheus) constructWriteRequest(timeSeriesData []TimeSeriesData) *prompb.WriteRequest {
	var tsList []prompb.TimeSeries
	for _, row := range timeSeriesData {
		var ts prompb.TimeSeries
		for k, v := range row.Labels {
			ts.Labels = append(ts.Labels, prompb.Label{Name: k, Value: v})
		}
		for _, s := range row.Samples {
			ts.Samples = append(ts.Samples, prompb.Sample{Value: s.Value, Timestamp: s.Timestamp})
		}
		tsList = append(tsList, ts)
	}
	return &prompb.WriteRequest{Timeseries: tsList}
}

func (m *Mockmetheus) constructReadRequest(query string) (*prompb.ReadRequest, error) {
	expression, err := parser.ParseExpr(query)
	if err != nil {
		return nil, err
	}

	var (
		metric   string
		labels   []*prompb.LabelMatcher
		duration = time.Hour
	)

	switch node := expression.(type) {
	case *parser.MatrixSelector:
		v, ok := node.VectorSelector.(*parser.VectorSelector)
		if !ok {
			return nil, fmt.Errorf("invalid matrix selector in query")
		}
		if v.Name != "" {
			metric = v.Name
		} else {
			metric = "__name__"
		}
		for _, labelMatcher := range v.LabelMatchers {
			labels = append(labels, &prompb.LabelMatcher{
				Name:  labelMatcher.Name,
				Value: labelMatcher.Value,
				Type:  toPrompbMatcherType(labelMatcher.Type),
			})
		}
		duration = node.Range

	case *parser.VectorSelector:
		if node.Name != "" {
			metric = node.Name
		} else {
			metric = "__name__"
		}
		for _, labelMatcher := range node.LabelMatchers {
			labels = append(labels, &prompb.LabelMatcher{
				Name:  labelMatcher.Name,
				Value: labelMatcher.Value,
				Type:  toPrompbMatcherType(labelMatcher.Type),
			})
		}

	default:
		return nil, fmt.Errorf("unsupported query expression type %T", expression)
	}

	nowMS := time.Now().UnixMilli()
	startMS := nowMS - int64(duration.Milliseconds())

	prompbQuery := &prompb.Query{
		StartTimestampMs: startMS,
		EndTimestampMs:   nowMS,
	}

	if metric != "" && metric != "__name__" {
		prompbQuery.Matchers = append(prompbQuery.Matchers, &prompb.LabelMatcher{
			Type:  prompb.LabelMatcher_EQ,
			Name:  "__name__",
			Value: metric,
		})
	}
	prompbQuery.Matchers = append(prompbQuery.Matchers, labels...)

	return &prompb.ReadRequest{
		Queries:               []*prompb.Query{prompbQuery},
		AcceptedResponseTypes: []prompb.ReadRequest_ResponseType{prompb.ReadRequest_SAMPLES},
	}, nil
}

// toPrompbMatcherType converts PromQL match types to Prometheus prompb label matcher types.
func toPrompbMatcherType(matchType labels.MatchType) prompb.LabelMatcher_Type {
	switch matchType {
	case labels.MatchEqual:
		return prompb.LabelMatcher_EQ
	case labels.MatchNotEqual:
		return prompb.LabelMatcher_NEQ
	case labels.MatchRegexp:
		return prompb.LabelMatcher_RE
	case labels.MatchNotRegexp:
		return prompb.LabelMatcher_NRE
	}
	return prompb.LabelMatcher_EQ
}

func parseResponse(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	b := new(bytes.Buffer)
	if _, err := io.Copy(b, resp.Body); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

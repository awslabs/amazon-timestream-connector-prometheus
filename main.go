/*
Copyright Amazon.com, Inc. or its affiliates. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License"). You may not use this file except in compliance with
the License. A copy of the License is located at

http://www.apache.org/licenses/LICENSE-2.0

or in the "license" file accompanying this file. This file is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR
CONDITIONS OF ANY KIND, either express or implied. See the License for the specific language governing permissions
and limitations under the License.
*/

// This file creates a local server when running from precompiled binaries or a Docker container, which will listen for
// Prometheus remote read and write requests. When running on AWS Lambda, the lambdaHandler function will listen for
// Prometheus remote read and write request sent to Amazon API Gateway.
package main

import (
	"encoding/base64"
	"fmt"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/go-kit/kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/prometheus/prompb"
	"gopkg.in/alecthomas/kingpin.v2"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"timestream-prometheus-connector/errors"
	"timestream-prometheus-connector/timestream"
)

const (
	readHeader            = "x-prometheus-remote-read-version"
	writeHeader           = "x-prometheus-remote-write-version"
	basicAuthHeader       = "authorization"
	writeClientMaxRetries = 10
)

var (
	// Store the initialization function calls and client retrieval calls to allow unit tests to mock the creation of real clients.
	createWriteClient = func(timestreamClient *timestream.Client, logger log.Logger, configs *aws.Config, failOnLongMetricLabelName bool, failOnInvalidSample bool, endpoint string) {
		timestreamClient.NewWriteClient(logger, configs, failOnLongMetricLabelName, failOnInvalidSample, endpoint)
	}
	createQueryClient = func(timestreamClient *timestream.Client, logger log.Logger, configs *aws.Config, maxRetries int, endpoint string) {
		configs.MaxRetries = aws.Int(maxRetries)
		timestreamClient.NewQueryClient(logger, configs, endpoint)
	}
	getWriteClient = func(timestreamClient *timestream.Client) writer { return timestreamClient.WriteClient() }
	getQueryClient = func(timestreamClient *timestream.Client) reader { return timestreamClient.QueryClient() }
	halt           = os.Exit
)

type writer interface {
	Write(req *prompb.WriteRequest, credentials *credentials.Credentials) ([3]int64, error)
	Name() string
}

type reader interface {
	Read(req *prompb.ReadRequest, credentials *credentials.Credentials) (*prompb.ReadResponse, error)
	Name() string
}

type clientConfig struct {
	region string
}

type connectionConfig struct {
	clientConfig              *clientConfig
	databaseLabel             string
	enableLogging             bool
	failOnLongMetricLabelName bool
	failOnInvalidSample       bool
	listenAddr                string
	promlogConfig             promlog.Config
	tableLabel                string
	telemetryPath             string
	maxRetries                int
	certificate               string
	key                       string
	ingestEndpoint            string
	queryEndpoint             string
}

func main() {
	if len(os.Getenv("LAMBDA_TASK_ROOT")) != 0 && len(os.Getenv("AWS_EXECUTION_ENV")) != 0 {
		// Start the AWS Lambda lambdaHandler if the connector is executing in an AWS Lambda environment.
		lambda.Start(lambdaHandler)
	} else {
		var writers []writer
		var readers []reader

		cfg := parseFlags()

		http.Handle(cfg.telemetryPath, promhttp.Handler())

		logger := cfg.createLogger()
		awsQueryConfigs := cfg.buildAWSConfig()
		awsWriteConfigs := cfg.buildAWSConfig()

		timestreamClient := timestream.NewBaseClient(cfg.databaseLabel, cfg.tableLabel)

		awsQueryConfigs.MaxRetries = aws.Int(cfg.maxRetries)

		timestreamClient.NewQueryClient(logger, awsQueryConfigs, cfg.queryEndpoint)

		awsWriteConfigs.MaxRetries = aws.Int(writeClientMaxRetries)
		timestreamClient.NewWriteClient(logger, awsWriteConfigs, cfg.failOnLongMetricLabelName, cfg.failOnInvalidSample, cfg.ingestEndpoint)

		timestream.LogInfo(logger, "Query endpoint: ", cfg.queryEndpoint)
		timestream.LogInfo(logger, "Ingest endpoint: ", cfg.ingestEndpoint)
		timestream.LogInfo(logger, "Successfully created Timestream clients to handle read and write requests from Prometheus.")

		// Register TimestreamClient to Prometheus for it to scrape metrics
		prometheus.MustRegister(timestreamClient)

		writers = append(writers, timestreamClient.WriteClient())
		readers = append(readers, timestreamClient.QueryClient())

		if err := serve(logger, cfg.listenAddr, writers, readers, cfg.certificate, cfg.key); err != nil {
			timestream.LogError(logger, "Error occurred while listening for requests.", err)
			os.Exit(1)
		}
	}
}

// lambdaHandler receives Prometheus read or write requests sent by API Gateway.
func lambdaHandler(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if len(os.Getenv(databaseLabelConfig.envFlag)) == 0 || len(os.Getenv(tableLabelConfig.envFlag)) == 0 {
		return createErrorResponse(errors.NewMissingDestinationError().(*errors.MissingDestinationError).Message())
	}

	cfg, err := parseEnvironmentVariables()
	if err != nil {
		return createErrorResponse(err.Error())
	}

	logger := cfg.createLogger()

	awsCredentials, ok := parseBasicAuth(req.Headers[basicAuthHeader])
	if !ok {
		return createErrorResponse(errors.NewParseBasicAuthHeaderError().(*errors.ParseBasicAuthHeaderError).Message())
	}

	awsConfigs := cfg.buildAWSConfig()
	timestreamClient := timestream.NewBaseClient(cfg.databaseLabel, cfg.tableLabel)

	requestBody, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		return createErrorResponse("Error occurred while decoding the API Gateway request body: " + err.Error())
	}

	reqBuf, err := snappy.Decode(nil, requestBody)
	if err != nil {
		return createErrorResponse("Error occurred while reading the write request sent by Prometheus: " + err.Error())
	}

	if len(req.Headers[writeHeader]) != 0 {
		return handleWriteRequest(reqBuf, timestreamClient, awsConfigs, cfg, logger, awsCredentials)
	} else if len(req.Headers[readHeader]) != 0 {
		return handleReadRequest(reqBuf, timestreamClient, awsConfigs, cfg, logger, awsCredentials)
	}

	return createErrorResponse(errors.NewMissingHeaderError(readHeader, writeHeader).(*errors.MissingHeaderError).Message())
}

// handleWriteRequest handles a Prometheus write request.
func handleWriteRequest(reqBuf []byte, timestreamClient *timestream.Client, awsConfigs *aws.Config, cfg *connectionConfig, logger log.Logger, credentials *credentials.Credentials) (events.APIGatewayProxyResponse, error) {
	var writeRequest prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &writeRequest); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Error occurred while unmarshalling the decoded write request from Prometheus.",
		}, nil
	}

	createWriteClient(timestreamClient, logger, awsConfigs, cfg.failOnLongMetricLabelName, cfg.failOnInvalidSample, cfg.ingestEndpoint)

	timestream.LogInfo(logger, "Ingest endpoint: ")
	timestream.LogInfo(logger, "Successfully created a Timestream write client to handle write requests from Prometheus.")

	if resp, err := getWriteClient(timestreamClient).Write(&writeRequest, credentials); err != nil {
		errorCode := http.StatusBadRequest

		if requestError, ok := err.(awserr.RequestFailure); ok {
			errorCode = requestError.StatusCode()
		}

		return events.APIGatewayProxyResponse{
			StatusCode: errorCode,
			Body:       err.Error(),
		}, nil
	} else {
		timestream.LogInfo(logger, "Ingest time: ", resp[0])
		timestream.LogInfo(logger, "Number of records : ", resp[1])
		timestream.LogInfo(logger, "Number of write calls : ", resp[2])
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
	}, nil
}

// handleReadRequest handles a Prometheus read request.
func handleReadRequest(reqBuf []byte, timestreamClient *timestream.Client, awsConfigs *aws.Config, cfg *connectionConfig, logger log.Logger, credentials *credentials.Credentials) (events.APIGatewayProxyResponse, error) {
	var readRequest prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &readRequest); err != nil {
		timestream.LogError(logger, "Error occurred while unmarshalling the decoded read request from Prometheus.", err)
		return createErrorResponse(err.Error())
	}

	createQueryClient(timestreamClient, logger, awsConfigs, cfg.maxRetries, cfg.queryEndpoint)

	timestream.LogInfo(logger, "Query endpoint: ", cfg.queryEndpoint)
	timestream.LogInfo(logger, "Successfully created a Timestream query client to handle write requests from Prometheus.")

	response, err := getQueryClient(timestreamClient).Read(&readRequest, credentials)
	if err != nil {
		timestream.LogError(logger, "Error occurred while reading the data back from Timestream.", err)
		if requestError, ok := err.(awserr.RequestFailure); ok {
			return events.APIGatewayProxyResponse{
				StatusCode: requestError.StatusCode(),
				Body:       err.Error(),
			}, nil
		}

		return createErrorResponse(err.Error())
	}

	data, err := proto.Marshal(response)
	if err != nil {
		timestream.LogError(logger, "Error occurred while marshalling the Prometheus ReadResponse.", err)
		return createErrorResponse(err.Error())
	}

	snappyEncodeData := snappy.Encode(nil, data)
	base64EncodeData := make([]byte, base64.StdEncoding.EncodedLen(len(snappyEncodeData)))
	base64.StdEncoding.Encode(base64EncodeData, snappyEncodeData)

	return events.APIGatewayProxyResponse{
		StatusCode:      http.StatusOK,
		IsBase64Encoded: true,
		Headers: map[string]string{
			"Content-Type":     "application/x-protobuf",
			"Content-Encoding": "snappy",
		},
		Body: string(base64EncodeData),
	}, nil
}

// parseBasicAuth parses the encoded HTTP Basic Authentication Header.
func parseBasicAuth(encoded string) (awsCredentials *credentials.Credentials, ok bool) {
	auth := strings.SplitN(encoded, " ", 2)
	if len(auth) != 2 || auth[0] != "Basic" {
		return nil, false
	}

	credentialsBytes, err := base64.StdEncoding.DecodeString(auth[1])
	if err != nil {
		return nil, false
	}
	credentialsSlice := strings.SplitN(string(credentialsBytes), ":", 2)
	if len(credentialsSlice) != 2 {
		return nil, false
	}
	return credentials.NewStaticCredentials(credentialsSlice[0], credentialsSlice[1], ""), true
}

// createLogger creates a new logger for the clients.
func (cfg *connectionConfig) createLogger() (logger log.Logger) {
	if cfg.enableLogging {
		logger = promlog.New(&cfg.promlogConfig)
	} else {
		logger = log.NewNopLogger()
	}

	timestream.LogInfo(logger, "timestream-prometheus-connector", "version", timestream.Version, "go version", timestream.GoVersion)
	return logger
}

// parseBoolFromStrings parses the boolean configuration options from the strings in connectionConfig.
func (cfg *connectionConfig) parseBoolFromStrings(enableLogging, failOnLongMetricLabelName, failOnInvalidSample string) error {
	var err error

	cfg.enableLogging, err = strconv.ParseBool(enableLogging)
	if err != nil {
		timestreamError := errors.NewParseEnableLoggingError(enableLogging)
		fmt.Println(timestreamError.Error())
		return timestreamError
	}

	cfg.failOnLongMetricLabelName, err = strconv.ParseBool(failOnLongMetricLabelName)
	if err != nil {
		timestreamError := errors.NewParseMetricLabelError(failOnLongMetricLabelName)
		fmt.Println(timestreamError.Error())
		return timestreamError
	}

	cfg.failOnInvalidSample, err = strconv.ParseBool(failOnInvalidSample)
	if err != nil {
		timestreamError := errors.NewParseSampleOptionError(failOnInvalidSample)
		fmt.Println(timestreamError.Error())
		return timestreamError
	}

	return nil
}

// getOrDefault returns the value if the key exists as an environment variable; returns the default value otherwise.
func getOrDefault(key *configuration) string {
	if value, exists := os.LookupEnv(key.envFlag); exists {
		return value
	}

	return key.defaultValue
}

// parseEnvironmentVariables parses the connector configuration options from the AWS Lambda function's environment variables.
func parseEnvironmentVariables() (*connectionConfig, error) {
	cfg := &connectionConfig{
		clientConfig:  &clientConfig{},
		promlogConfig: promlog.Config{},
	}

	cfg.clientConfig.region = getOrDefault(regionConfig)
	cfg.databaseLabel = getOrDefault(databaseLabelConfig)
	cfg.tableLabel = getOrDefault(tableLabelConfig)

	var err error
	err = cfg.parseBoolFromStrings(getOrDefault(enableLogConfig), getOrDefault(failOnLabelConfig), getOrDefault(failOnInvalidSampleConfig))
	if err != nil {
		return nil, err
	}

	retries := getOrDefault(maxRetriesConfig)
	cfg.maxRetries, err = strconv.Atoi(retries)
	if err != nil {
		return nil, errors.NewParseRetriesError(retries)
	}

	cfg.ingestEndpoint = getOrDefault(ingestEndpointConfig)
	cfg.queryEndpoint = getOrDefault(queryEndpointConfig)

	cfg.promlogConfig = promlog.Config{Level: &promlog.AllowedLevel{}, Format: &promlog.AllowedFormat{}}
	cfg.promlogConfig.Level.Set(getOrDefault(promlogLevelConfig))
	cfg.promlogConfig.Format.Set(getOrDefault(promlogFormatConfig))

	return cfg, nil
}

// parseFlags parses command line flags and return a connectionConfig pointer.
func parseFlags() *connectionConfig {
	a := kingpin.New(filepath.Base(os.Args[0]), "Remote storage adapter")
	a.HelpFlag.Short('h')

	cfg := &connectionConfig{
		clientConfig:  &clientConfig{},
		promlogConfig: promlog.Config{},
	}

	var enableLogging string
	var failOnLongMetricLabelName string
	var failOnInvalidSample string

	a.Flag(enableLogConfig.flag, "Enables or disables logging in the connector. Default to 'true'.").Default(enableLogConfig.defaultValue).StringVar(&enableLogging)
	a.Flag(regionConfig.flag, "The signing region for the Timestream service. Default to 'us-east-1'.").Default(regionConfig.defaultValue).StringVar(&cfg.clientConfig.region)
	a.Flag(maxRetriesConfig.flag, "The maximum number of times the read request will be retried for failures. Default to 3.").Default(maxRetriesConfig.defaultValue).IntVar(&cfg.maxRetries)
	a.Flag(databaseLabelConfig.flag, "The Prometheus label containing the database name for data ingestion.").Required().StringVar(&cfg.databaseLabel)
	a.Flag(tableLabelConfig.flag, "The Prometheus label containing the table name for data ingestion.").Required().StringVar(&cfg.tableLabel)
	a.Flag(listenAddrConfig.flag, "Address to listen on for web endpoints.").Default(listenAddrConfig.defaultValue).StringVar(&cfg.listenAddr)
	a.Flag(telemetryPathConfig.flag, "Address to listen on for web endpoints.").Default(telemetryPathConfig.defaultValue).StringVar(&cfg.telemetryPath)
	a.Flag(failOnLabelConfig.flag, "Enables or disables the option to halt the program immediately when a Prometheus Label name exceeds 256 bytes. Default to 'false'.").
		Default(failOnLabelConfig.defaultValue).StringVar(&failOnLongMetricLabelName)
	a.Flag(failOnInvalidSampleConfig.flag, "Enables or disables the option to halt the program immediately when a Sample contains a non-finite float value. Default to 'false'.").
		Default(failOnInvalidSampleConfig.defaultValue).StringVar(&failOnInvalidSample)
	a.Flag(certificateConfig.flag, "TLS server certificate file.").Default(certificateConfig.defaultValue).StringVar(&cfg.certificate)
	a.Flag(keyConfig.flag, "TLS server private key file.").Default(keyConfig.defaultValue).StringVar(&cfg.key)
	a.Flag(ingestEndpointConfig.flag, "The ingestion endpoint for private link.").Default("").StringVar(&cfg.ingestEndpoint)
	a.Flag(queryEndpointConfig.flag, "The query endpoint for private link.").Default("").StringVar(&cfg.queryEndpoint)

	flag.AddFlags(a, &cfg.promlogConfig)

	if _, err := a.Parse(os.Args[1:]); err != nil {
		kingpin.Errorf("error occurred while parsing command line flags: '%s'", err)
		os.Exit(1)
	}

	if err := cfg.parseBoolFromStrings(enableLogging, failOnLongMetricLabelName, failOnInvalidSample); err != nil {
		os.Exit(1)
	}

	return cfg
}

// buildAWSConfig builds a aws.Config and return the pointer of the config.
func (cfg *connectionConfig) buildAWSConfig() *aws.Config {
	clientConfig := cfg.clientConfig
	awsConfig := &aws.Config{
		Region: aws.String(clientConfig.region),
	}
	return awsConfig
}

// serve listens for requests and remote writes and reads to Timestream.
func serve(logger log.Logger, address string, writers []writer, readers []reader, certificate string, key string) error {
	http.HandleFunc("/write", createWriteHandler(logger, writers))
	http.HandleFunc("/read", createReadHandler(logger, readers))

	server := http.Server{
		Addr: address,
	}

	if certificate == "" || key == "" {
		return server.ListenAndServe()
	} else {
		return server.ListenAndServeTLS(certificate, key)
	}
}

// createWriteHandler creates a handler func(ResponseWriter, *Request) to handle Prometheus write requests.
func createWriteHandler(logger log.Logger, writers []writer) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		awsCredentials, authOk := parseBasicAuth(r.Header.Get(basicAuthHeader))
		if !authOk {
			err := errors.NewParseBasicAuthHeaderError()
			timestream.LogError(logger, "Error occurred while parsing the basic authentication header.", err)
			http.Error(w, err.(*errors.ParseBasicAuthHeaderError).Message(), http.StatusBadRequest)
			return
		}

		compressed, err := ioutil.ReadAll(r.Body)
		if err != nil {
			timestream.LogError(logger, "Error occurred while reading the write request sent by Prometheus.", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			timestream.LogError(logger, "Error occurred while decoding the write request from Prometheus.", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.WriteRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			timestream.LogError(logger, "Error occurred while unmarshalling the decoded write request from Prometheus.", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if resp, err := writers[0].Write(&req, awsCredentials); err != nil {
			switch err := err.(type) {
			case awserr.RequestFailure:
				requestError := err.(awserr.RequestFailure)
				http.Error(w, err.Error(), requestError.StatusCode())
			case *errors.SDKNonRequestError:
				http.Error(w, err.Error(), http.StatusBadRequest)
			case *errors.MissingDatabaseWithWriteError:
				http.Error(w, err.Error(), http.StatusBadRequest)
			case *errors.MissingTableWithWriteError:
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				// Others will halt the program.
				halt(1)
			}
		} else {
			timestream.LogInfo(logger, "Ingest time: ", resp[0])
			timestream.LogInfo(logger, "Number of records : ", resp[1])
			timestream.LogInfo(logger, "Number of write calls : ", resp[2])
		}
	}
}

// createReadHandler creates a handler func(ResponseWriter, *Request) to handle Prometheus read requests.
func createReadHandler(logger log.Logger, readers []reader) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		awsCredentials, authOk := parseBasicAuth(r.Header.Get(basicAuthHeader))
		if !authOk {
			err := errors.NewParseBasicAuthHeaderError()
			timestream.LogError(logger, "Error occurred while parsing the basic authentication header.", err)
			http.Error(w, err.(*errors.ParseBasicAuthHeaderError).Message(), http.StatusBadRequest)
			return
		}

		compressed, err := ioutil.ReadAll(r.Body)

		if err != nil {
			timestream.LogError(logger, "Error occurred while reading the read request sent by Prometheus.", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		reqBuf, err := snappy.Decode(nil, compressed)
		if err != nil {
			timestream.LogError(logger, "Error occurred while decoding the read request from Prometheus.", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		var req prompb.ReadRequest
		if err := proto.Unmarshal(reqBuf, &req); err != nil {
			timestream.LogError(logger, "Error occurred while unmarshalling the decoded read request from Prometheus.", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		response, err := readers[0].Read(&req, awsCredentials)
		if err != nil {
			timestream.LogError(logger, "Error occurred while reading the data back from Timestream.", err)
			if requestError, ok := err.(awserr.RequestFailure); ok {
				http.Error(w, err.Error(), requestError.StatusCode())
				return
			}

			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		data, err := proto.Marshal(response)
		if err != nil {
			timestream.LogError(logger, "Error occurred while marshalling the Prometheus ReadResponse.", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/x-protobuf")
		w.Header().Set("Content-Encoding", "snappy")

		if _, err := w.Write(snappy.Encode(nil, data)); err != nil {
			timestream.LogError(logger, "Error occurred while writing the encoded ReadResponse to the connection as part of an HTTP reply.", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
}

// createErrorResponse creates an events.APIGatewayProxyResponse with a 400 Status Code and the given error message.
func createErrorResponse(msg string) (events.APIGatewayProxyResponse, error) {
	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusBadRequest,
		Body:       msg,
	}, nil
}

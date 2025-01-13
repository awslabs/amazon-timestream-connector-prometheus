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
	"context"
	"encoding/base64"
	goErrors "errors"
	"fmt"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/retry"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	wtypes "github.com/aws/aws-sdk-go-v2/service/timestreamwrite/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"timestream-prometheus-connector/errors"
	"timestream-prometheus-connector/timestream"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/snappy"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/promlog/flag"
	"github.com/prometheus/prometheus/prompb"
)

const (
	readHeader      = "x-prometheus-remote-read-version"
	writeHeader     = "x-prometheus-remote-write-version"
	basicAuthHeader = "authorization"
)

var (
	// Store the initialization function calls and client retrieval calls to allow unit tests to mock the creation of real clients.
	createWriteClient = func(timestreamClient *timestream.Client, logger log.Logger, cfg aws.Config, failOnLongMetricLabelName bool, failOnInvalidSample bool) {
		timestreamClient.NewWriteClient(logger, cfg, failOnLongMetricLabelName, failOnInvalidSample)
	}
	createQueryClient = func(timestreamClient *timestream.Client, logger log.Logger, cfg aws.Config) {
		timestreamClient.NewQueryClient(logger, cfg)
	}

	getWriteClient = func(timestreamClient *timestream.Client) writer {
		return timestreamClient.WriteClient()
	}
	getQueryClient = func(timestreamClient *timestream.Client) reader {
		return timestreamClient.QueryClient()
	}
	halt = os.Exit
)

type writer interface {
	Write(ctx context.Context, req *prompb.WriteRequest, credentialsProvider aws.CredentialsProvider) error
	Name() string
}

type reader interface {
	Read(ctx context.Context, req *prompb.ReadRequest, credentialsProvider aws.CredentialsProvider) (*prompb.ReadResponse, error)
	Name() string
}

type clientConfig struct {
	region string
}

type connectionConfig struct {
	clientConfig              *clientConfig
	defaultDatabase           string
	defaultTable              string
	enableLogging             bool
	enableSigV4Auth           bool
	failOnLongMetricLabelName bool
	failOnInvalidSample       bool
	listenAddr                string
	promlogConfig             promlog.Config
	telemetryPath             string
	maxReadRetries            int
	maxWriteRetries           int
	certificate               string
	key                       string
}

func main() {
	if len(os.Getenv("LAMBDA_TASK_ROOT")) != 0 {
		// Start the AWS Lambda lambdaHandler if the connector is executing in an AWS Lambda environment.
		lambda.Start(lambdaHandler)
	} else {
		var writers []writer
		var readers []reader

		cfg := parseFlags()

		http.Handle(cfg.telemetryPath, promhttp.Handler())

		logger := cfg.createLogger()

		ctx := context.Background()
		awsQueryConfigs, err := cfg.buildAWSConfig(ctx, cfg.maxReadRetries)
		if err != nil {
			timestream.LogError(logger, "Failed to build AWS configuration for query", err)
			os.Exit(1)
		}

		awsWriteConfigs, err := cfg.buildAWSConfig(ctx, cfg.maxWriteRetries)
		if err != nil {
			timestream.LogError(logger, "Failed to build AWS configuration for write", err)
			os.Exit(1)
		}

		timestreamClient := timestream.NewBaseClient(cfg.defaultDatabase, cfg.defaultTable)
		timestreamClient.NewQueryClient(logger, awsQueryConfigs)
		timestreamClient.NewWriteClient(logger, awsWriteConfigs, cfg.failOnLongMetricLabelName, cfg.failOnInvalidSample)

		timestream.LogInfo(logger, fmt.Sprintf("Timestream connection is initialized (Database: %s, Table: %s, Region: %s)", cfg.defaultDatabase, cfg.defaultTable, cfg.clientConfig.region))
		// Register TimestreamClient to Prometheus for it to scrape metrics
		prometheus.MustRegister(timestreamClient)

		writers = append(writers, timestreamClient.WriteClient())
		readers = append(readers, timestreamClient.QueryClient())

		timestream.LogInfo(logger, "The Prometheus Connector is now ready to begin serving ingestion and query requests.")
		if err := serve(logger, cfg.listenAddr, writers, readers, cfg.certificate, cfg.key); err != nil {
			timestream.LogError(logger, "Error occurred while listening for requests.", err)
			os.Exit(1)
		}
	}
}

// lambdaHandler receives Prometheus read or write requests sent by API Gateway.
func lambdaHandler(req events.APIGatewayProxyRequest) (events.APIGatewayProxyResponse, error) {
	if len(os.Getenv(defaultDatabaseConfig.envFlag)) == 0 || len(os.Getenv(defaultTableConfig.envFlag)) == 0 {
		return createErrorResponse(errors.NewMissingDestinationError().(*errors.MissingDestinationError).Message())
	}

	cfg, err := parseEnvironmentVariables()
	if err != nil {
		return createErrorResponse(err.Error())
	}

	logger := cfg.createLogger()

	ctx := context.Background()
	var awsCredentials aws.CredentialsProvider
	var ok bool

	// If SigV4 authentication has been enabled, such as when write requests originate
	// from the OpenTelemetry collector, credentials will be taken from the local environment.
	// Otherwise, basic auth is used for AWS credentials
	if cfg.enableSigV4Auth {
		awsConfig, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			return createErrorResponse("Error loading AWS config: " + err.Error())
		}
		awsCredentials = awsConfig.Credentials
	} else {
		awsCredentials, ok = parseBasicAuth(req.Headers[basicAuthHeader])
		if !ok {
			return createErrorResponse(errors.NewParseBasicAuthHeaderError().(*errors.ParseBasicAuthHeaderError).Message())
		}
	}
	awsQueryConfigs, err := cfg.buildAWSConfig(ctx, cfg.maxReadRetries)
	if err != nil {
		timestream.LogError(logger, "Failed to build AWS configuration for query", err)
		os.Exit(1)
	}
	awsWriteConfigs, err := cfg.buildAWSConfig(ctx, cfg.maxWriteRetries)
	if err != nil {
		timestream.LogError(logger, "Failed to build AWS configuration for write", err)
		os.Exit(1)
	}

	timestreamClient := timestream.NewBaseClient(cfg.defaultDatabase, cfg.defaultTable)

	requestBody, err := base64.StdEncoding.DecodeString(req.Body)
	if err != nil {
		return createErrorResponse("Error occurred while decoding the API Gateway request body: " + err.Error())
	}

	reqBuf, err := snappy.Decode(nil, requestBody)
	if err != nil {
		return createErrorResponse("Error occurred while reading the write request sent by Prometheus: " + err.Error())
	}

	if len(req.Headers[writeHeader]) != 0 {
		return handleWriteRequest(reqBuf, timestreamClient, awsWriteConfigs, cfg, logger, awsCredentials)
	} else if len(req.Headers[readHeader]) != 0 {
		return handleReadRequest(reqBuf, timestreamClient, awsQueryConfigs, cfg, logger, awsCredentials)
	}

	return createErrorResponse(errors.NewMissingHeaderError(readHeader, writeHeader).(*errors.MissingHeaderError).Message())
}

// handleWriteRequest handles a Prometheus write request.
func handleWriteRequest(reqBuf []byte, timestreamClient *timestream.Client, awsConfigs aws.Config, cfg *connectionConfig, logger log.Logger, credentialsProvider aws.CredentialsProvider) (events.APIGatewayProxyResponse, error) {
	var writeRequest prompb.WriteRequest
	if err := proto.Unmarshal(reqBuf, &writeRequest); err != nil {
		return events.APIGatewayProxyResponse{
			StatusCode: http.StatusBadRequest,
			Body:       "Error occurred while unmarshalling the decoded write request from Prometheus.",
		}, nil
	}

	createWriteClient(timestreamClient, logger, awsConfigs, cfg.failOnLongMetricLabelName, cfg.failOnInvalidSample)

	timestream.LogInfo(logger, fmt.Sprintf("Timestream write connection is initialized (Database: %s, Table: %s, Region: %s)", cfg.defaultDatabase, cfg.defaultTable, cfg.clientConfig.region))
	if err := getWriteClient(timestreamClient).Write(context.Background(), &writeRequest, credentialsProvider); err != nil {
		errorCode := http.StatusBadRequest
		return events.APIGatewayProxyResponse{
			StatusCode: errorCode,
			Body:       err.Error(),
		}, nil
	}

	return events.APIGatewayProxyResponse{
		StatusCode: http.StatusOK,
	}, nil
}

// handleReadRequest handles a Prometheus read request.
func handleReadRequest(reqBuf []byte, timestreamClient *timestream.Client, awsConfigs aws.Config, cfg *connectionConfig, logger log.Logger, credentialsProvider aws.CredentialsProvider) (events.APIGatewayProxyResponse, error) {
	var readRequest prompb.ReadRequest
	if err := proto.Unmarshal(reqBuf, &readRequest); err != nil {
		timestream.LogError(logger, "Error occurred while unmarshalling the decoded read request from Prometheus.", err)
		return createErrorResponse(err.Error())
	}

	createQueryClient(timestreamClient, logger, awsConfigs)

	timestream.LogInfo(logger, fmt.Sprintf("Timestream query connection is initialized (Database: %s, Table: %s, Region: %s)", cfg.defaultDatabase, cfg.defaultTable, cfg.clientConfig.region))

	response, err := getQueryClient(timestreamClient).Read(context.Background(), &readRequest, credentialsProvider)
	if err != nil {
		timestream.LogError(logger, "Error occurred while reading the data back from Timestream.", err)
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
func parseBasicAuth(encoded string) (aws.CredentialsProvider, bool) {
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
	staticCredentials := aws.NewCredentialsCache(
		credentials.StaticCredentialsProvider{
			Value: aws.Credentials{
				AccessKeyID:     credentialsSlice[0],
				SecretAccessKey: credentialsSlice[1],
				Source:          "BasicAuthHeader",
			},
		},
	)
	return staticCredentials, true
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
func (cfg *connectionConfig) parseBoolFromStrings(enableLogging, failOnLongMetricLabelName, failOnInvalidSample, enableSigV4Auth string) error {
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

	cfg.enableSigV4Auth, err = strconv.ParseBool(enableSigV4Auth)
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
	cfg.defaultDatabase = getOrDefault(defaultDatabaseConfig)
	cfg.defaultTable = getOrDefault(defaultTableConfig)

	var err error
	err = cfg.parseBoolFromStrings(getOrDefault(enableLogConfig), getOrDefault(failOnLabelConfig), getOrDefault(failOnInvalidSampleConfig), getOrDefault(enableSigV4AuthConfig))
	if err != nil {
		return nil, err
	}

	readRetries := getOrDefault(maxReadRetriesConfig)
	cfg.maxReadRetries, err = strconv.Atoi(readRetries)
	if err != nil {
		return nil, errors.NewParseRetriesError(readRetries, "read")
	}

	writeRetries := getOrDefault(maxWriteRetriesConfig)
	cfg.maxWriteRetries, err = strconv.Atoi(writeRetries)
	if err != nil {
		return nil, errors.NewParseRetriesError(writeRetries, "write")
	}

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
	var enableSigV4Auth string
	var failOnLongMetricLabelName string
	var failOnInvalidSample string

	a.Flag(enableLogConfig.flag, "Enables or disables logging in the connector. Default to 'true'.").Default(enableLogConfig.defaultValue).StringVar(&enableLogging)
	a.Flag(regionConfig.flag, "The signing region for the Timestream service. Default to 'us-east-1'.").Default(regionConfig.defaultValue).StringVar(&cfg.clientConfig.region)
	a.Flag(maxReadRetriesConfig.flag, "The maximum number of times the read request will be retried for failures. Default to 3.").Default(maxReadRetriesConfig.defaultValue).IntVar(&cfg.maxReadRetries)
	a.Flag(maxWriteRetriesConfig.flag, "The maximum number of times the write request will be retried for failures. Default to 10.").Default(maxWriteRetriesConfig.defaultValue).IntVar(&cfg.maxWriteRetries)
	a.Flag(defaultDatabaseConfig.flag, "The Prometheus label containing the database name for data ingestion.").Default(defaultDatabaseConfig.defaultValue).StringVar(&cfg.defaultDatabase)
	a.Flag(defaultTableConfig.flag, "The Prometheus label containing the table name for data ingestion.").Default(defaultTableConfig.defaultValue).StringVar(&cfg.defaultTable)
	a.Flag(listenAddrConfig.flag, "Address to listen on for web endpoints.").Default(listenAddrConfig.defaultValue).StringVar(&cfg.listenAddr)
	a.Flag(telemetryPathConfig.flag, "Address to listen on for web endpoints.").Default(telemetryPathConfig.defaultValue).StringVar(&cfg.telemetryPath)
	a.Flag(failOnLabelConfig.flag, "Enables or disables the option to halt the program immediately when a Prometheus Label name exceeds 256 bytes. Default to 'false'.").
		Default(failOnLabelConfig.defaultValue).StringVar(&failOnLongMetricLabelName)
	a.Flag(failOnInvalidSampleConfig.flag, "Enables or disables the option to halt the program immediately when a Sample contains a non-finite float value. Default to 'false'.").
		Default(failOnInvalidSampleConfig.defaultValue).StringVar(&failOnInvalidSample)
	a.Flag(certificateConfig.flag, "TLS server certificate file.").Default(certificateConfig.defaultValue).StringVar(&cfg.certificate)
	a.Flag(keyConfig.flag, "TLS server private key file.").Default(keyConfig.defaultValue).StringVar(&cfg.key)
	a.Flag(enableSigV4AuthConfig.flag, "Whether to enable SigV4 authentication with the API Gateway. Default to 'false'.").Default(enableSigV4AuthConfig.defaultValue).StringVar(&enableSigV4Auth)

	flag.AddFlags(a, &cfg.promlogConfig)

	if _, err := a.Parse(os.Args[1:]); err != nil {
		kingpin.Errorf("error occurred while parsing command line flags: '%s'", err)
		os.Exit(1)
	}

	if err := cfg.parseBoolFromStrings(enableLogging, failOnLongMetricLabelName, failOnInvalidSample, enableSigV4Auth); err != nil {
		os.Exit(1)
	}

	if cfg.defaultDatabase == "" {
		kingpin.Errorf("The default database value must be set through the flag --default-database")
		os.Exit(1)
	}

	if cfg.defaultTable == "" {
		kingpin.Errorf("The default table value must be set through the flag --default-table")
		os.Exit(1)
	}

	return cfg
}

// buildAWSConfig builds a aws.Config and return the pointer of the config.
func (cfg *connectionConfig) buildAWSConfig(ctx context.Context, maxRetries int) (aws.Config, error) {
	awsConfig, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.clientConfig.region),
		config.WithRetryer(func() aws.Retryer {
			return retry.NewStandard(func(o *retry.StandardOptions) {
				o.MaxAttempts = maxRetries
			})
		}),
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to build AWS config: %w", err)
	}
	return awsConfig, nil
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

		compressed, err := io.ReadAll(r.Body)
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
		if err := writers[0].Write(context.Background(), &req, awsCredentials); err != nil {
			switch err := err.(type) {
			case *smithyhttp.ResponseError:
				http.Error(w, err.Error(), http.StatusBadRequest)
			case *wtypes.RejectedRecordsException:
				http.Error(w, err.Error(), http.StatusUnprocessableEntity)
			case *smithy.OperationError:
				var apiError *smithy.GenericAPIError
				if goErrors.As(err, &apiError) {
					http.Error(w, apiError.ErrorMessage(), getHTTPStatusFromSmithyError(apiError))
					return
				}
				http.Error(w, "An unknown service error occurred", http.StatusInternalServerError)
			case *errors.SDKNonRequestError:
				http.Error(w, err.Error(), http.StatusBadRequest)
			case *errors.MissingDatabaseWithWriteError:
				http.Error(w, err.Error(), http.StatusNotFound)
			case *errors.MissingTableWithWriteError:
				http.Error(w, err.Error(), http.StatusNotFound)
			default:
				halt(1)
			}
		}

	}
}

func getHTTPStatusFromSmithyError(err *smithy.GenericAPIError) int {
	switch err.ErrorCode() {
	case "ThrottlingException":
		return http.StatusTooManyRequests
	case "ResourceNotFoundException":
		return http.StatusNotFound
	case "AccessDeniedException":
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
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

		compressed, err := io.ReadAll(r.Body)
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
		response, err := readers[0].Read(context.Background(), &req, awsCredentials)
		if err != nil {
			timestream.LogError(logger, "Error occurred while reading the data back from Timestream.", err)
			var rejectedRecordsErr *wtypes.RejectedRecordsException
			if goErrors.As(err, &rejectedRecordsErr) {
				http.Error(w, err.Error(), http.StatusBadRequest)
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

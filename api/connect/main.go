package main

import (
	"os"
	"fmt"
	"log"
	"time"
	"errors"
	"context"
	"strings"
	"strconv"
	"net/http"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

type MessageResponse struct {
	Data []MessageData `json:"data"`
}

type MessageData struct {
	Id      int    `dynamodbav:"id"`
	Data    string `dynamodbav:"data"`
	Created int    `dynamodbav:"created"`
	Color   string `dynamodbav:"color"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type Connection struct {
	ConnectionId string `dynamodbav:"connectionId"`
	Created      int    `dynamodbav:"created"`
	Color        string `dynamodbav:"color"`
}

type Response events.APIGatewayProxyResponse

var apiClient *apigatewaymanagementapi.Client
var dynamodbClient *dynamodb.Client

const layout string = "20060102150405.000"

func HandleRequest(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) (Response, error) {
	var err error
	var jsonBytes []byte
	connectionCount, err := getConnectionCount(ctx)
	limitCount, _ := strconv.Atoi(os.Getenv("LIMIT_CONNECTION_COUNT"))
	if err == nil && int(connectionCount) < limitCount {
		err = putConnection(ctx, request.RequestContext.ConnectionID)
	} else if int(connectionCount) >= limitCount {
		err = errors.New("too many connections")
	}
	log.Print(request.RequestContext.Identity.SourceIP)
	if err != nil {
		log.Print(err)
		jsonBytes, _ = json.Marshal(ErrorResponse{Message: fmt.Sprint(err)})
		return Response{
			StatusCode: http.StatusInternalServerError,
			Body: string(jsonBytes),
		}, nil
	}
	responseBody := ""
	if len(jsonBytes) > 0 {
		responseBody = string(jsonBytes)
	}
	return Response {
		StatusCode: http.StatusOK,
		Body: responseBody,
	}, nil
}

func scan(ctx context.Context, tableName string)(*dynamodb.ScanOutput, error)  {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(getConfig(ctx))
	}
	params := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	}
	return dynamodbClient.Scan(ctx, params)
}

func put(ctx context.Context, tableName string, av map[string]types.AttributeValue) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(getConfig(ctx))
	}
	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}
	_, err := dynamodbClient.PutItem(ctx, input)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func getConnectionCount(ctx context.Context)(int32, error)  {
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"))
	if err != nil {
		return int32(0), err
	}
	return result.ScannedCount, nil
}

func putConnection(ctx context.Context, connectionId string) error {
	t := time.Now()
	t_, _ := strconv.Atoi(strings.Replace(t.Format(layout), ".", "", 1))
	c := strconv.FormatInt(int64(t_), 16)
	item := Connection {
		ConnectionId: connectionId,
		Created:      t_,
		Color:        "00" + c[(len(c) - 4):],
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		log.Print(err)
		return err
	}
	err = put(ctx, os.Getenv("CONNECTION_TABLE_NAME"), av)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func getConfig(ctx context.Context) aws.Config {
	var err error
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(os.Getenv("REGION")))
	if err != nil {
		log.Print(err)
	}
	return cfg
}

func main() {
	lambda.Start(HandleRequest)
}

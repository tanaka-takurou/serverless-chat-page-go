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
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/dynamodbattribute"
)

type MessageResponse struct {
	Data []MessageData `json:"data"`
}

type MessageData struct {
	Id      int    `json:"id"`
	Data    string `json:"data"`
	Created int    `json:"created"`
	Color   string `json:"color"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

type Connection struct {
	ConnectionId string `json:"connectionId"`
	Created      int    `json:"created"`
	Color        string `json:"color"`
}

type Response events.APIGatewayProxyResponse

var cfg aws.Config
var apiClient *apigatewaymanagementapi.Client
var dynamodbClient *dynamodb.Client

const layout string = "20060102150405.000"

func HandleRequest(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) (Response, error) {
	var err error
	var jsonBytes []byte
	connectionCount, err := getConnectionCount(ctx)
	limitCount, _ := strconv.Atoi(os.Getenv("LIMIT_CONNECTION_COUNT"))
	if err == nil && int(*connectionCount) < limitCount {
		err = putConnection(ctx, request.RequestContext.ConnectionID)
	} else if int(*connectionCount) >= limitCount {
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

func scan(ctx context.Context, tableName string, filt expression.ConditionBuilder)(*dynamodb.ScanResponse, error)  {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.New(cfg)
	}
	expr, err := expression.NewBuilder().WithFilter(filt).Build()
	if err != nil {
		return nil, err
	}
	params := &dynamodb.ScanInput{
		ExpressionAttributeNames:  expr.Names(),
		ExpressionAttributeValues: expr.Values(),
		FilterExpression:          expr.Filter(),
		ProjectionExpression:      expr.Projection(),
		TableName:                 aws.String(tableName),
	}
	req := dynamodbClient.ScanRequest(params)
	return req.Send(ctx)
}

func put(ctx context.Context, tableName string, av map[string]dynamodb.AttributeValue) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.New(cfg)
	}
	input := &dynamodb.PutItemInput{
		Item:      av,
		TableName: aws.String(tableName),
	}
	req := dynamodbClient.PutItemRequest(input)
	_, err := req.Send(ctx)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func getConnectionCount(ctx context.Context)(*int64, error)  {
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"), expression.NotEqual(expression.Name("status"), expression.Value(-1)))
	if err != nil {
		return nil, err
	}
	return result.ScanOutput.ScannedCount, nil
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
	av, err := dynamodbattribute.MarshalMap(item)
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

func init() {
	var err error
	cfg, err = external.LoadDefaultAWSConfig()
	cfg.Region = os.Getenv("REGION")
	if err != nil {
		log.Print(err)
	}
}

func main() {
	lambda.Start(HandleRequest)
}

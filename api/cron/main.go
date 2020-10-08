package main

import (
	"os"
	"log"
	"time"
	"strings"
	"strconv"
	"context"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/dynamodbattribute"
)

type ErrorResponse struct {
	Message  string `json:"message"`
}

type Connection struct {
	ConnectionId string `json:"connectionId"`
	Created      int    `json:"created"`
	Color        string `json:"color"`
}

var cfg aws.Config
var apiClient *apigatewaymanagementapi.Client
var dynamodbClient *dynamodb.Client

const layout string = "20060102150405.000"

func HandleRequest(ctx context.Context, event events.CloudWatchEvent) error {
	err := checkConnections(ctx)
	if err != nil {
		return err
	}
	return nil
}

func scan(ctx context.Context, tableName string)(*dynamodb.ScanResponse, error)  {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.New(cfg)
	}
	params := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	}
	req := dynamodbClient.ScanRequest(params)
	return req.Send(ctx)
}

func delete(ctx context.Context, tableName string, key map[string]dynamodb.AttributeValue) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.New(cfg)
	}
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: key,
	}

	req := dynamodbClient.DeleteItemRequest(input)
	_, err := req.Send(ctx)
	return err
}

func checkConnections(ctx context.Context) error {
	t := time.Now()
	t_, _ := strconv.Atoi(strings.Replace(t.Format(layout), ".", "", 1))
	old := t_ - 60*60*2
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"))
	if err != nil {
		log.Print(err)
		return err
	}
	for _, i := range result.ScanOutput.Items {
		item := Connection{}
		err := dynamodbattribute.UnmarshalMap(i, &item)
		if err != nil {
			log.Print(err)
		} else {
			// Delete old Connections
			if item.Created > old {
				continue
			}
			key := map[string]dynamodb.AttributeValue{
				"connectionId": {
					S: aws.String(item.ConnectionId),
				},
			}
			err := delete(ctx, os.Getenv("CONNECTION_TABLE_NAME"), key)
			if err != nil {
				log.Print(err)
			}
		}
	}
	if err != nil {
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

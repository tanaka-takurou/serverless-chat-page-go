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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

type ErrorResponse struct {
	Message  string `json:"message"`
}

type Connection struct {
	ConnectionId string `dynamodbav:"connectionId"`
	Created      int    `dynamodbav:"created"`
	Color        string `dynamodbav:"color"`
}

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

func scan(ctx context.Context, tableName string)(*dynamodb.ScanOutput, error)  {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(getConfig(ctx))
	}
	params := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	}
	return dynamodbClient.Scan(ctx, params)
}

func delete(ctx context.Context, tableName string, key map[string]types.AttributeValue) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(getConfig(ctx))
	}
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: key,
	}

	_, err := dynamodbClient.DeleteItem(ctx, input)
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
	for _, i := range result.Items {
		item := Connection{}
		err := attributevalue.UnmarshalMap(i, &item)
		if err != nil {
			log.Print(err)
		} else {
			// Delete old Connections
			if item.Created > old {
				continue
			}
			item := struct {Token string `dynamodbav:"connectionId"`}{item.ConnectionId}
			key, err := attributevalue.MarshalMap(item)
			if err != nil {
				log.Print(err)
			} else {
				err = delete(ctx, os.Getenv("CONNECTION_TABLE_NAME"), key)
				if err != nil {
					log.Print(err)
				}
			}
		}
	}
	if err != nil {
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

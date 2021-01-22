package main

import (
	"os"
	"fmt"
	"log"
	"context"
	"net/http"
	"encoding/json"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
)

type ErrorResponse struct {
	Message  string `json:"message"`
}

type Connection struct {
	ConnectionId string `dynamodbav:"connectionId"`
	Created      int    `dynamodbav:"created"`
	Color        string `dynamodbav:"color"`
}

type Response events.APIGatewayProxyResponse

var apiClient *apigatewaymanagementapi.Client
var dynamodbClient *dynamodb.Client

func HandleRequest(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) (Response, error) {
	err := deleteConnection(ctx, request.RequestContext.ConnectionID)
	log.Print(request.RequestContext.Identity.SourceIP)
	if err != nil {
		jsonBytes, _ := json.Marshal(ErrorResponse{Message: fmt.Sprint(err)})
		return Response{
			StatusCode: http.StatusInternalServerError,
			Body: string(jsonBytes),
		}, nil
	}
	return Response {
		StatusCode: http.StatusOK,
		Body: "",
	}, nil
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

func deleteConnection(ctx context.Context, connectionId string) error {
	item := struct {Token string `dynamodbav:"connectionId"`}{connectionId}
	key, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}
	err = delete(ctx, os.Getenv("CONNECTION_TABLE_NAME"), key)
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

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
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
)

type ErrorResponse struct {
	Message  string `json:"message"`
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

func deleteConnection(ctx context.Context, connectionId string) error {
	key := map[string]dynamodb.AttributeValue{
		"connectionId": {
			S: aws.String(connectionId),
		},
	}
	err := delete(ctx, os.Getenv("CONNECTION_TABLE_NAME"), key)
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

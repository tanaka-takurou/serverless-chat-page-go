package main

import (
	"os"
	"fmt"
	"log"
	"html"
	"time"
	"bytes"
	"errors"
	"strconv"
	"strings"
	"context"
	"net/url"
	"net/http"
	"encoding/json"
	"path/filepath"
	"encoding/base64"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/expression"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/dynamodbattribute"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/s3manager"
)

type MessageData struct {
	Id           int    `json:"id"`
	Data         string `json:"data"`
	Created      int    `json:"created"`
	Color        string `json:"color"`
}

type ErrorResponse struct {
	Message  string `json:"message"`
}

type Connection struct {
	ConnectionId string `json:"connectionId"`
	Created      int    `json:"created"`
	Color        string `json:"color"`
}

type PostData struct {
	Image string `data:"image"`
	Text  string `data:"text"`
}

type PublishData struct {
	Data    string `json:"data"`
	Color   string `json:"color"`
}

type Response events.APIGatewayProxyResponse

var cfg aws.Config
var apigatewayClient *apigatewaymanagementapi.Client
var dynamodbClient *dynamodb.Client

const layout string = "20060102150405.000"

func HandleRequest(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) (Response, error) {
	var err error
	err = sendMessage(ctx, request)
	log.Print(request.RequestContext.Identity.SourceIP)
	if err != nil {
		log.Print(err)
		var jsonBytes []byte
		jsonBytes, _ = json.Marshal(ErrorResponse{Message: fmt.Sprint(err)})
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

func get(ctx context.Context, tableName string, key map[string]dynamodb.AttributeValue, att string)(*dynamodb.GetItemResponse, error) {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.New(cfg)
	}
	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: key,
		AttributesToGet: []string{
			att,
		},
		ConsistentRead: aws.Bool(true),
		ReturnConsumedCapacity: dynamodb.ReturnConsumedCapacityNone,
	}
	req := dynamodbClient.GetItemRequest(input)
	return req.Send(ctx)
}

func update(ctx context.Context, tableName string, an map[string]string, av map[string]dynamodb.AttributeValue, key map[string]dynamodb.AttributeValue, updateExpression string) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.New(cfg)
	}
	input := &dynamodb.UpdateItemInput{
		ExpressionAttributeNames: an,
		ExpressionAttributeValues: av,
		TableName: aws.String(tableName),
		Key: key,
		ReturnValues:     dynamodb.ReturnValueUpdatedNew,
		UpdateExpression: aws.String(updateExpression),
	}

	req := dynamodbClient.UpdateItemRequest(input)
	_, err := req.Send(ctx)
	return err
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

func getMessageCount(ctx context.Context)(*int64, error) {
	result, err := scan(ctx, os.Getenv("MESSAGE_TABLE_NAME"), expression.NotEqual(expression.Name("status"), expression.Value(-1)))
	if err != nil {
		return nil, err
	}
	return result.ScanOutput.ScannedCount, nil
}

func getOldestMessage(ctx context.Context)(MessageData, error) {
	var messageData MessageData
	result, err := scan(ctx, os.Getenv("MESSAGE_TABLE_NAME"), expression.NotEqual(expression.Name("id"), expression.Value(-1)))
	if err != nil {
		log.Println(err)
		return messageData, err
	}
	for _, i := range result.ScanOutput.Items {
		item := MessageData{}
		err = dynamodbattribute.UnmarshalMap(i, &item)
		if err != nil {
			log.Println(err)
		} else if messageData.Created < 1 || messageData.Created > item.Created {
			messageData = item
		}
	}
	return messageData, nil
}

func putMessage(ctx context.Context, connectionId string, message string, color string, lastId int) error {
	t := time.Now()
	t_, _ := strconv.Atoi(strings.Replace(t.Format(layout), ".", "", 1))
	item := MessageData {
		Id: lastId + 1,
		Data: message,
		Created: t_,
		Color: color,
	}
	av, err := dynamodbattribute.MarshalMap(item)
	if err != nil {
		log.Print(err)
		return err
	}
	err = put(ctx, os.Getenv("MESSAGE_TABLE_NAME"), av)
	if err != nil {
		log.Print(err)
		return err
	}
	return nil
}

func updateMessage(ctx context.Context, connectionId string, message string, color string) error {
	t := time.Now()
	oldestMessage, err := getOldestMessage(ctx)
	if err != nil {
		return err
	}
	an := map[string]string{
		"#d": "data",
		"#c": "created",
		"#i": "connectionId",
		"#l": "color",
	}
	av := map[string]dynamodb.AttributeValue{
		":newData": {
			S: aws.String(message),
		},
		":newCreated": {
			N: aws.String(strings.Replace(t.Format(layout), ".", "", 1)),
		},
		":newConnectionId": {
			S: aws.String(connectionId),
		},
		":newColor": {
			S: aws.String(color),
		},
	}
	key := map[string]dynamodb.AttributeValue{
		"id": {
			N: aws.String(strconv.Itoa(oldestMessage.Id)),
		},
	}
	updateExpression := "set #d = :newData, #c = :newCreated, #i = :newConnectionId, #l = :newColor"
	err = update(ctx, os.Getenv("MESSAGE_TABLE_NAME"), an, av, key, updateExpression)
	if err != nil {
		return err
	}
	return nil
}

func saveMessage(ctx context.Context, connectionId string, message string, color string) error {
	messageCount, err := getMessageCount(ctx)
	if err != nil {
		return err
	}
	limitCount, _ := strconv.Atoi(os.Getenv("LIMIT_MESSAGE_COUNT"))
	if int(*messageCount) < limitCount {
		putMessage(ctx, connectionId, message, color, int(*messageCount))
	} else {
		updateMessage(ctx, connectionId, message, color)
	}
	if err != nil {
		return err
	}
	return nil
}

func getColorFromConnectionID(ctx context.Context, connectionId string)( string, error) {
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"), expression.Name("connectionId").Equal(expression.Value(connectionId)))
	if err != nil {
		return "", err
	}
	item := Connection{}
	err = dynamodbattribute.UnmarshalMap(result.ScanOutput.Items[0], &item)
	if err != nil {
		return "", err
	}
	return item.Color, nil
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

func uploadImage(filename string, filedata string)(string, error) {
	t := time.Now()
	b64data := filedata[strings.IndexByte(filedata, ',')+1:]
	data, err := base64.StdEncoding.DecodeString(b64data)
	if err != nil {
		log.Print(err)
		return "", err
	}
	extension := filepath.Ext(filename)
	var contentType string

	switch extension {
	case ".jpg":
		contentType = "image/jpeg"
	case ".jpeg":
		contentType = "image/jpeg"
	case ".gif":
		contentType = "image/gif"
	case ".png":
		contentType = "image/png"
	default:
		return "", errors.New("this extension is invalid")
	}
	filename_ := string([]rune(filename)[:(len(filename) - len(extension))]) + strings.Replace(t.Format(layout), ".", "", 1) + extension
	uploader := s3manager.NewUploader(cfg)
	_, err = uploader.Upload(&s3manager.UploadInput{
		ACL: s3.ObjectCannedACLPublicRead,
		Bucket: aws.String(os.Getenv("BUCKET_NAME")),
		Key: aws.String(filename_),
		Body: bytes.NewReader(data),
		ContentType: aws.String(contentType),
	})
	if err != nil {
		log.Print(err)
		return "", err
	}
	return "https://" + os.Getenv("BUCKET_NAME") + ".s3-" + os.Getenv("REGION") + ".amazonaws.com/" + filename_, nil
}

func sendMessage(ctx context.Context, request events.APIGatewayWebsocketProxyRequest) error {
	if apigatewayClient == nil {
		cp := cfg.Copy()
		cp.EndpointResolver = aws.EndpointResolverFunc(func(service, region string) (aws.Endpoint, error) {
			if service != "execute-api" {
				return cfg.EndpointResolver.ResolveEndpoint(service, region)
			}

			var endpoint url.URL
			endpoint.Path = request.RequestContext.Stage
			endpoint.Host = request.RequestContext.DomainName
			endpoint.Scheme = "https"
			return aws.Endpoint{
				SigningRegion: region,
				URL:           endpoint.String(),
			}, nil
		})
		apigatewayClient = apigatewaymanagementapi.New(cp)
	}
	var d PostData
	var err error
	err = json.Unmarshal([]byte(request.Body), &d)
	if err != nil {
		log.Print(err)
		return err
	}

	var message string
	isText := true
	if len(d.Image) > 0 {
		message, err = uploadImage(d.Text, d.Image)
		isText = false
		if err != nil {
			log.Print(err)
			return err
		}
	} else {
		message = html.EscapeString(d.Text)
	}
	color, err := getColorFromConnectionID(ctx, request.RequestContext.ConnectionID)
	if err != nil {
		log.Print(err)
		return err
	}

	err = saveMessage(ctx, request.RequestContext.ConnectionID, message, color)
	if err != nil {
		log.Print(err)
		return err
	}
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"), expression.NotEqual(expression.Name("status"), expression.Value(-1)))
	if err != nil {
		log.Print(err)
		return err
	}

	var lostConnectionIdList []string
	var jsonBytes []byte
	jsonBytes, _ = json.Marshal(PublishData{
		Data: message,
		Color: color,
	})
	if err != nil {
		log.Print(err)
		return err
	}
	// Post to ConnectionRequest
	for _, i := range result.ScanOutput.Items {
		item := Connection{}
		err = dynamodbattribute.UnmarshalMap(i, &item)
		if err != nil {
			log.Println(err)
		} else {
			if isText && item.ConnectionId == request.RequestContext.ConnectionID  {
				continue
			}
			connectionId := item.ConnectionId
			connectionRequest := apigatewayClient.PostToConnectionRequest(&apigatewaymanagementapi.PostToConnectionInput{
				Data:         jsonBytes,
				ConnectionId: &connectionId,
			})
			_, err := connectionRequest.Send(ctx)
			if err != nil {
				lostConnectionIdList = append(lostConnectionIdList, connectionId)
			}
		}
	}
	// Delete lost-ConnectionId form dynamodb
	for _, i := range lostConnectionIdList {
		_ = deleteConnection(ctx, i)
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

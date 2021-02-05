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
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	dynamodbtypes "github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	s3manager "github.com/aws/aws-sdk-go-v2/feature/s3/manager"
)

type MessageData struct {
	Id           int    `dynamodbav:"id"`
	Data         string `dynamodbav:"data"`
	Created      int    `dynamodbav:"created"`
	Color        string `dynamodbav:"color"`
}

type ErrorResponse struct {
	Message  string `json:"message"`
}

type Connection struct {
	ConnectionId string `dynamodbav:"connectionId"`
	Created      int    `dynamodbav:"created"`
	Color        string `dynamodbav:"color"`
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
	initConfig(ctx)
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

func scan(ctx context.Context, tableName string)(*dynamodb.ScanOutput, error)  {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(cfg)
	}
	params := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	}
	return dynamodbClient.Scan(ctx, params)
}

func put(ctx context.Context, tableName string, av map[string]dynamodbtypes.AttributeValue) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(cfg)
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

func get(ctx context.Context, tableName string, key map[string]dynamodbtypes.AttributeValue, att string)(*dynamodb.GetItemOutput, error) {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(cfg)
	}
	input := &dynamodb.GetItemInput{
		TableName: aws.String(tableName),
		Key: key,
		AttributesToGet: []string{
			att,
		},
		ConsistentRead: aws.Bool(true),
		ReturnConsumedCapacity: dynamodbtypes.ReturnConsumedCapacityNone,
	}
	return dynamodbClient.GetItem(ctx, input)
}

func update(ctx context.Context, tableName string, an map[string]string, av map[string]dynamodbtypes.AttributeValue, key map[string]dynamodbtypes.AttributeValue, updateExpression string) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(cfg)
	}
	input := &dynamodb.UpdateItemInput{
		ExpressionAttributeNames: an,
		ExpressionAttributeValues: av,
		TableName: aws.String(tableName),
		Key: key,
		ReturnValues:     dynamodbtypes.ReturnValueUpdatedNew,
		UpdateExpression: aws.String(updateExpression),
	}

	_, err := dynamodbClient.UpdateItem(ctx, input)
	return err
}

func delete(ctx context.Context, tableName string, key map[string]dynamodbtypes.AttributeValue) error {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(cfg)
	}
	input := &dynamodb.DeleteItemInput{
		TableName: aws.String(tableName),
		Key: key,
	}

	_, err := dynamodbClient.DeleteItem(ctx, input)
	return err
}

func getMessageCount(ctx context.Context)(int32, error) {
	result, err := scan(ctx, os.Getenv("MESSAGE_TABLE_NAME"))
	if err != nil {
		return int32(0), err
	}
	return result.ScannedCount, nil
}

func getOldestMessage(ctx context.Context)(MessageData, error) {
	var messageData MessageData
	result, err := scan(ctx, os.Getenv("MESSAGE_TABLE_NAME"))
	if err != nil {
		log.Println(err)
		return messageData, err
	}
	for _, i := range result.Items {
		item := MessageData{}
		err = attributevalue.UnmarshalMap(i, &item)
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
	av, err := attributevalue.MarshalMap(item)
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
	created, err := strconv.Atoi(strings.Replace(t.Format(layout), ".", "", 1))
	if err != nil {
		return err
	}
	item := struct {
		NewData         string `dynamodbav:":newData"`
		NewCreated      int    `dynamodbav:":newCreated"`
		NewConnectionId string `dynamodbav:":newConnectionId"`
		NewColor        string `dynamodbav:":newColor"`
	}{
		NewData:         message,
		NewCreated:      created,
		NewConnectionId: connectionId,
		NewColor:        color,
	}
	av, err := attributevalue.MarshalMap(item)
	if err != nil {
		return err
	}
	item_ := struct {Id int `dynamodbav:"id"`}{oldestMessage.Id}
	key, err := attributevalue.MarshalMap(item_)
	if err != nil {
		return err
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
	if int(messageCount) < limitCount {
		putMessage(ctx, connectionId, message, color, int(messageCount))
	} else {
		updateMessage(ctx, connectionId, message, color)
	}
	if err != nil {
		return err
	}
	return nil
}

func getColorFromConnectionID(ctx context.Context, connectionId string)( string, error) {
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"))
	if err != nil {
		return "", err
	}
	color := ""
	for _, i := range result.Items {
		item := Connection{}
		err = attributevalue.UnmarshalMap(i, &item)
		if err != nil {
			log.Println(err)
		} else if item.ConnectionId == connectionId {
			color = item.Color
			break
		}
	}
	return color, nil
}

func deleteConnection(ctx context.Context, connectionId string) error {
	item := struct {ConnectionId string `dynamodbav:"connectionId"`}{connectionId}
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

func uploadImage(ctx context.Context, filename string, filedata string)(string, error) {
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
	uploader := s3manager.NewUploader(s3.NewFromConfig(cfg))
	_, err = uploader.Upload(ctx, &s3.PutObjectInput{
		ACL: s3types.ObjectCannedACLPublicRead,
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
		var endpoint url.URL
		endpoint.Scheme = "https"
		endpoint.Path = request.RequestContext.Stage
		endpoint.Host = request.RequestContext.DomainName
		endpointResolver := apigatewaymanagementapi.EndpointResolverFromURL(endpoint.String())
		apigatewayClient = apigatewaymanagementapi.NewFromConfig(cfg, apigatewaymanagementapi.WithEndpointResolver(endpointResolver))
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
		message, err = uploadImage(ctx, d.Text, d.Image)
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
	result, err := scan(ctx, os.Getenv("CONNECTION_TABLE_NAME"))
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
	for _, i := range result.Items {
		item := Connection{}
		err = attributevalue.UnmarshalMap(i, &item)
		if err != nil {
			log.Println(err)
		} else {
			if isText && item.ConnectionId == request.RequestContext.ConnectionID  {
				continue
			}
			connectionId := item.ConnectionId
			_, err := apigatewayClient.PostToConnection(ctx, &apigatewaymanagementapi.PostToConnectionInput{
				Data:         jsonBytes,
				ConnectionId: &connectionId,
			})
			if err != nil {
				log.Println(err)
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

func initConfig(ctx context.Context) {
	var err error
	cfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(os.Getenv("REGION")))
	if err != nil {
		log.Print(err)
	}
}

func main() {
	lambda.Start(HandleRequest)
}

package main

import (
	"io"
	"os"
	"log"
	"sort"
	"bytes"
	"context"
	"strconv"
	"strings"
	"net/http"
	"html/template"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/aws/external"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/dynamodbattribute"
)

type TemplateData struct {
	Title   string
	Url     string
	Max     int
	Bucket  string
	LogList []LogData
}

type MessageData struct {
	Id           int    `json:"id"`
	Data         string `json:"data"`
	Created      int    `json:"created"`
	Color        string `json:"color"`
}

type LogData struct {
	Text     string `json:"text"`
	ImageUrl string `json:"imageurl"`
	Color    string `json:"color"`
}

type Response events.APIGatewayProxyResponse

var cfg aws.Config
var dynamodbClient *dynamodb.Client

const title string = "Simple Chat"

func init() {
	var err error
	cfg, err = external.LoadDefaultAWSConfig()
	cfg.Region = os.Getenv("REGION")
	if err != nil {
		log.Print(err)
	}
}

func main() {
	lambda.Start(handler)
}

func handler(ctx context.Context, request events.APIGatewayV2HTTPRequest) (Response, error) {
	var dat TemplateData
	fnc := template.FuncMap{
		"safehtml": func(text string) template.HTML { return template.HTML(text) },
	}
	buf := new(bytes.Buffer)
	fw := io.Writer(buf)
	tmp := template.Must(template.New("tmp").Funcs(fnc).ParseFiles("templates/index.html", "templates/view.html", "templates/header.html"))
	dat.Title = title
	dat.Url = os.Getenv("WEBSOCKET_URL")
	dat.Max, _ = strconv.Atoi(os.Getenv("LIMIT_MESSAGE_COUNT"))
	dat.Bucket = os.Getenv("BUCKET_NAME")
	messageList, err := sacnMessageList(ctx)
	if err != nil {
		log.Fatal(err)
	} else {
		dat.LogList = getLogList(messageList)
	}
	if err = tmp.ExecuteTemplate(fw, "base", dat); err != nil {
		log.Fatal(err)
	}
	res := Response{
		StatusCode:      http.StatusOK,
		IsBase64Encoded: false,
		Body:            string(buf.Bytes()),
		Headers: map[string]string{
			"Content-Type": "text/html",
		},
	}
	return res, nil
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

func sacnMessageList(ctx context.Context)([]MessageData, error)  {
	result, err := scan(ctx, os.Getenv("MESSAGE_TABLE_NAME"))
	if err != nil {
		log.Print(err)
		return nil, err
	}
	var messageList []MessageData
	for _, i := range result.ScanOutput.Items {
		item := MessageData{}
		err := dynamodbattribute.UnmarshalMap(i, &item)
		if err != nil {
			log.Print(err)
		} else {
			messageList = append(messageList, item)
		}
	}
	sort.Slice(messageList, func(i, j int) bool { return messageList[i].Created < messageList[j].Created })
	return messageList, nil
}

func getLogList(messageList []MessageData) []LogData {
	var logList []LogData
	for _, i := range messageList {
		text := ""
		imageUrl := ""
		if strings.HasPrefix(i.Data, "https://" + os.Getenv("BUCKET_NAME")) {
			imageUrl = i.Data
		} else {
			text = i.Data
		}
		logList = append(logList, LogData{
			Text: text,
			ImageUrl: imageUrl,
			Color: i.Color,
		})
	}
	return logList
}

package main

import (
	"io"
	"os"
	"log"
	"sort"
	"bytes"
	"embed"
	"context"
	"strconv"
	"strings"
	"net/http"
	"html/template"
	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
)

type TemplateData struct {
	Title   string
	Url     string
	Max     int
	Bucket  string
	LogList []LogData
}

type MessageData struct {
	Id           int    `dynamodbav:"id"`
	Data         string `dynamodbav:"data"`
	Created      int    `dynamodbav:"created"`
	Color        string `dynamodbav:"color"`
}

type LogData struct {
	Text     string `json:"text"`
	ImageUrl string `json:"imageurl"`
	Color    string `json:"color"`
}

type Response events.APIGatewayProxyResponse

//go:embed templates
var templateFS embed.FS
var dynamodbClient *dynamodb.Client

const title string = "Simple Chat"

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
	tmp := template.Must(template.New("tmp").Funcs(fnc).ParseFS(templateFS, "templates/index.html", "templates/view.html", "templates/header.html"))
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

func scan(ctx context.Context, tableName string)(*dynamodb.ScanOutput, error)  {
	if dynamodbClient == nil {
		dynamodbClient = dynamodb.NewFromConfig(getConfig(ctx))
	}
	params := &dynamodb.ScanInput{
		TableName: aws.String(tableName),
	}
	return dynamodbClient.Scan(ctx, params)
}

func sacnMessageList(ctx context.Context)([]MessageData, error)  {
	result, err := scan(ctx, os.Getenv("MESSAGE_TABLE_NAME"))
	if err != nil {
		log.Print(err)
		return nil, err
	}
	var messageList []MessageData
	for _, i := range result.Items {
		item := MessageData{}
		err := attributevalue.UnmarshalMap(i, &item)
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

func getConfig(ctx context.Context) aws.Config {
	var err error
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(os.Getenv("REGION")))
	if err != nil {
		log.Print(err)
	}
	return cfg
}

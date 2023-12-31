module github.com/tanaka-takurou/serverless-chat-page-go

go 1.21

require (
	github.com/aws/aws-lambda-go latest
	github.com/aws/aws-sdk-go-v2 latest
	github.com/aws/aws-sdk-go-v2/config latest
	github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue latest
	github.com/aws/aws-sdk-go-v2/feature/s3/manager latest
	github.com/aws/aws-sdk-go-v2/service/apigatewaymanagementapi latest
	github.com/aws/aws-sdk-go-v2/service/dynamodb latest
	github.com/aws/aws-sdk-go-v2/service/s3 latest
)

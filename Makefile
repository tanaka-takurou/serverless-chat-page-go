root	:=		$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

.PHONY: clean build deploy

clean:
	rm -rfv bin
	$(MAKE) -C "${root}/api/connect" clean
	$(MAKE) -C "${root}/api/disconnect" clean
	$(MAKE) -C "${root}/api/send" clean
	$(MAKE) -C "${root}/api/cron" clean

build:
	mkdir -p bin
	scripts/create_template.sh
	GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/bootstrap
	$(MAKE) -C "${root}/api/connect" build
	$(MAKE) -C "${root}/api/disconnect" build
	$(MAKE) -C "${root}/api/send" build
	$(MAKE) -C "${root}/api/cron" build

deploy:
	sam package --output-template-file "${root}"/packaged.yml --s3-bucket "${bucket}"
	sam deploy --stack-name "${stack}" --capabilities CAPABILITY_IAM --template-file "${root}/packaged.yml"

publish:
	sam package --output-template-file "${root}"/packaged.yml --s3-bucket "${bucket}"
	sam publish --template "${root}/packaged.yml" --region "${AWS_DEFAULT_REGION}"

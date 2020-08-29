#!/bin/bash
echo 'Creating template...'
`dirname $0`/create_template.sh

echo 'Creating function.zip...'
`dirname $0`/create_function.sh

echo 'Updating Lambda-Function...'
cd `dirname $0`/../
aws lambda update-function-code \
	--profile default \
	--function-name ServerlessChatFrontFunction \
	--zip-file fileb://`pwd`/function.zip \
	--cli-connect-timeout 6000 \
	--publish

.PHONY: build clean deploy deploy-dev deploy-prod test vet lint
STAGE ?= dev
REGION = eu-central-1
STACK = fairytale-$(STAGE)

build:
	sam build

build-WebhookFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o bootstrap ./cmd/webhook
	cp bootstrap $(ARTIFACTS_DIR)/bootstrap

build-BotWorkerFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o bootstrap ./cmd/botworker
	cp bootstrap $(ARTIFACTS_DIR)/bootstrap

build-OutboundFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o bootstrap ./cmd/outbound
	cp bootstrap $(ARTIFACTS_DIR)/bootstrap

build-SheetsSyncFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o bootstrap ./cmd/sheetssync
	cp bootstrap $(ARTIFACTS_DIR)/bootstrap

build-MigrateFunction:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -tags lambda.norpc -o bootstrap ./cmd/migrate
	cp bootstrap $(ARTIFACTS_DIR)/bootstrap

clean:
	rm -rf .aws-sam bootstrap

deploy: build
	sam deploy --guided

deploy-dev: build
	sam deploy --no-confirm-changeset --no-fail-on-empty-changeset \
		--resolve-s3 --capabilities CAPABILITY_IAM \
		--stack-name fairytale-dev --region $(REGION) \
		--parameter-overrides Stage=dev

deploy-prod: build
	sam deploy --no-confirm-changeset --no-fail-on-empty-changeset \
		--resolve-s3 --capabilities CAPABILITY_IAM \
		--stack-name fairytale-prod --region $(REGION) \
		--parameter-overrides Stage=prod

test:
	go test ./...

vet:
	go vet ./...

lint: vet
	go build ./...

migrate-dev:
	aws lambda invoke --function-name fairytale-migrate-dev --region $(REGION) /dev/stdout

migrate-prod:
	aws lambda invoke --function-name fairytale-migrate-prod --region $(REGION) /dev/stdout

logs-webhook:
	aws logs tail /aws/lambda/fairytale-webhook-$(STAGE) --follow --region $(REGION)

logs-botworker:
	aws logs tail /aws/lambda/fairytale-botworker-$(STAGE) --follow --region $(REGION)

logs-outbound:
	aws logs tail /aws/lambda/fairytale-outbound-$(STAGE) --follow --region $(REGION)

logs-sheetssync:
	aws logs tail /aws/lambda/fairytale-sheetssync-$(STAGE) --follow --region $(REGION)

dlq-check:
	@echo "=== BotTasks DLQ ==="
	@aws sqs get-queue-attributes \
		--queue-url $$(aws sqs get-queue-url --queue-name fairytale-bottasks-dlq-$(STAGE) --query QueueUrl --output text --region $(REGION)) \
		--attribute-names ApproximateNumberOfMessages --region $(REGION)
	@echo "=== Outbound DLQ ==="
	@aws sqs get-queue-attributes \
		--queue-url $$(aws sqs get-queue-url --queue-name fairytale-outbound-dlq-$(STAGE) --query QueueUrl --output text --region $(REGION)) \
		--attribute-names ApproximateNumberOfMessages --region $(REGION)

set-webhook-dev:
	@WEBHOOK_URL=$$(aws cloudformation describe-stacks --stack-name fairytale-dev \
		--query 'Stacks[0].Outputs[?OutputKey==`WebhookURL`].OutputValue' \
		--output text --region $(REGION)) && \
	TOKEN=$$(aws ssm get-parameter --name /fairytale/dev/telegram-bot-token \
		--with-decryption --query Parameter.Value --output text --region $(REGION)) && \
	SECRET=$$(aws ssm get-parameter --name /fairytale/dev/telegram-webhook-secret \
		--with-decryption --query Parameter.Value --output text --region $(REGION)) && \
	echo "Setting webhook: $$WEBHOOK_URL" && \
	curl -s "https://api.telegram.org/bot$$TOKEN/setWebhook" \
		-d "url=$$WEBHOOK_URL" -d "secret_token=$$SECRET"

set-webhook-prod:
	@WEBHOOK_URL=$$(aws cloudformation describe-stacks --stack-name fairytale-prod \
		--query 'Stacks[0].Outputs[?OutputKey==`WebhookURL`].OutputValue' \
		--output text --region $(REGION)) && \
	TOKEN=$$(aws ssm get-parameter --name /fairytale/prod/telegram-bot-token \
		--with-decryption --query Parameter.Value --output text --region $(REGION)) && \
	SECRET=$$(aws ssm get-parameter --name /fairytale/prod/telegram-webhook-secret \
		--with-decryption --query Parameter.Value --output text --region $(REGION)) && \
	echo "Setting webhook: $$WEBHOOK_URL" && \
	curl -s "https://api.telegram.org/bot$$TOKEN/setWebhook" \
		-d "url=$$WEBHOOK_URL" -d "secret_token=$$SECRET"

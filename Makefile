.PHONY: build run test lint migrate-up migrate-down docker-up docker-down swagger mock

build:
	go build -o bin/notification-service ./cmd/api

run:
	go run ./cmd/api

test:
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

lint:
	golangci-lint run ./...

migrate-up:
	migrate -path migrations -database "$(DATABASE_DSN)" up

migrate-down:
	migrate -path migrations -database "$(DATABASE_DSN)" down

docker-up:
	docker-compose up --build

docker-down:
	docker-compose down -v

swagger:
	swag init -g cmd/api/main.go -o docs

mock:
	mockery --all --dir internal/domain --output internal/mocks

tidy:
	go mod tidy

.DEFAULT_GOAL := build

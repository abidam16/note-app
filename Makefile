APP_NAME=note-app

.PHONY: run test tidy migrate-up migrate-down

run:
	go run ./cmd/api

test:
	go test ./...

tidy:
	go mod tidy

migrate-up:
	go run ./cmd/migrate -direction up

migrate-down:
	go run ./cmd/migrate -direction down -steps 1
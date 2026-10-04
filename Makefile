.PHONY: test fmt api worker agent watchdog db-up db-down

test:
	go test ./...

fmt:
	gofmt -w cmd internal

api:
	go run ./cmd/api

worker:
	go run ./cmd/worker

agent:
	go run ./cmd/agent

watchdog:
	go run ./cmd/watchdog

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

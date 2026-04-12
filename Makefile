.PHONY: run build test test-integration lint tidy up down seed status

run:
	go run ./cmd/server/main.go

build:
	CGO_ENABLED=0 go build -ldflags="-s -w" -o bin/canopy ./cmd/server/main.go

test:
	go test ./... -v -race -count=1 -timeout=60s

test-integration:
	MONGO_URI=mongodb://localhost:27017 \
	go test ./internal/integration/... -v -race -count=1 -timeout=120s

test-all: test test-integration

lint:
	golangci-lint run ./...

tidy:
	go mod tidy

up:
	docker compose up -d

down:
	docker compose down

seed:
	@bash scripts/seed.sh

logs:
	docker compose logs -f canopy

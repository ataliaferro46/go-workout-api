.PHONY: run demo build test cover vet fmt tidy docker clean

# Run the API server on :8080 (override with ADDR=:9000 make run).
run:
	go run ./cmd/api

# Print a sample generated plan to the terminal.
demo:
	go run ./cmd/demo

# Compile both binaries into ./bin.
build:
	go build -o bin/api ./cmd/api
	go build -o bin/demo ./cmd/demo

# Full test suite with the race detector.
test:
	go test -race ./...

# Tests plus a coverage summary.
cover:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

docker:
	docker build -t go-workout-api .

clean:
	rm -rf bin coverage.out

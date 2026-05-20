.PHONY: run build test cover vet fmt tidy docker clean

# Run the API locally on :8080 (override with ADDR=:9000 make run).
run:
	go run ./cmd/api

# Compile all packages and produce a binary in ./bin.
build:
	go build -o bin/api ./cmd/api

# Run the full test suite with the race detector enabled.
test:
	go test -race ./...

# Run tests and print a coverage summary.
cover:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

# Static analysis.
vet:
	go vet ./...

# Format all Go files in place.
fmt:
	gofmt -w .

# Verify the module graph (no-op here since there are no deps).
tidy:
	go mod tidy

# Build the container image.
docker:
	docker build -t go-workout-api .

clean:
	rm -rf bin coverage.out

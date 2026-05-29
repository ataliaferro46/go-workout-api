.PHONY: run demo build test cover vet fmt tidy docker clean \
        db-up db-down db-psql test-integration

# Local Postgres URL matching docker-compose.yml.
DATABASE_URL ?= postgres://workout:workout@localhost:5433/workout?sslmode=disable
TEST_DATABASE_URL ?= postgres://workout:workout@localhost:5433/workout_test?sslmode=disable

# Run the API server on :8080. With DATABASE_URL unset it uses in-memory
# storage; export DATABASE_URL=... or use `make run-pg` for Postgres.
run:
	go run ./cmd/api

# Run the API against the local Postgres in docker-compose.yml.
run-pg:
	DATABASE_URL=$(DATABASE_URL) go run ./cmd/api

# Print a sample generated plan to the terminal.
demo:
	go run ./cmd/demo

# Compile both binaries into ./bin.
build:
	go build -o bin/api ./cmd/api
	go build -o bin/demo ./cmd/demo

# Unit tests with the race detector. Excludes integration tests.
test:
	go test -race ./...

# Integration tests against a real Postgres. Requires `make db-up` first;
# creates the workout_test database if it does not exist.
test-integration:
	@docker exec workout-postgres psql -U workout -d workout \
		-tAc "SELECT 1 FROM pg_database WHERE datname='workout_test'" | grep -q 1 || \
		docker exec workout-postgres createdb -U workout workout_test
	TEST_DATABASE_URL=$(TEST_DATABASE_URL) go test -race -tags=integration ./internal/workout/...

# Tests plus a coverage summary.
cover:
	go test -race -coverprofile=coverage.out ./... && go tool cover -func=coverage.out

vet:
	go vet ./...

fmt:
	gofmt -w .

tidy:
	go mod tidy

# --- local Postgres -------------------------------------------------------

db-up:
	docker compose up -d postgres

db-down:
	docker compose down

db-psql:
	docker exec -it workout-postgres psql -U workout -d workout

docker:
	docker build -t go-workout-api .

clean:
	rm -rf bin coverage.out

.PHONY: run debug build test vet tidy fmt check db-up db-down migrate-up migrate-down migrate-create

# Local Postgres (with pgvector) for development.
db-up:
	docker compose up -d postgres

db-down:
	docker compose down

# Migration CLI targets (the app also auto-migrates on startup).
# Requires ASKDESK_DATABASE_URL and the `migrate` CLI.
migrate-up:
	migrate -path internal/store/migrations -database "$(ASKDESK_DATABASE_URL)" up

migrate-down:
	migrate -path internal/store/migrations -database "$(ASKDESK_DATABASE_URL)" down 1

# Create a new migration: make migrate-create name=add_something
migrate-create:
	migrate create -ext sql -dir internal/store/migrations -seq $(name)


# Run the service (info level — debug logs hidden).
run:
	go run ./cmd/askdesk

# Run with debug logging enabled (debug logs appear).
debug:
	ASKDESK_LOG_LEVEL=debug go run ./cmd/askdesk

# Compile the binary to bin/askdesk.
build:
	go build -o bin/askdesk ./cmd/askdesk

# Run all tests.
test:
	go test ./...

# Static analysis.
vet:
	go vet ./...

# Sync module dependencies.
tidy:
	go mod tidy

# Format all Go code.
fmt:
	go fmt ./...

# Pre-commit gate: vet and tests.
check: vet test

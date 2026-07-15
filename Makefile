.PHONY: run debug build test vet tidy fmt check

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

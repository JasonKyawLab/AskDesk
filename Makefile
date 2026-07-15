.PHONY: run build test vet tidy fmt check

# Run the service locally.
run:
	go run ./cmd/askdesk

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

# Pre-commit gate: format check, vet, and tests.
check: vet test

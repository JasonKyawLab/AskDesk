.PHONY: run worker debug build test vet tidy fmt check db-up db-down migrate-up migrate-down migrate-create seed set-webhook delete-webhook docker-build load-faqs

# Seed the first business + admin. Requires ASKDESK_DATABASE_URL and psql.
# Usage: make seed BUSINESS_NAME=minipos ADMIN_TG_ID=123456789
seed:
	psql "$(ASKDESK_DATABASE_URL)" -v ON_ERROR_STOP=1 \
	  -c "INSERT INTO businesses (id, name, api_key) OVERRIDING SYSTEM VALUE VALUES (1, '$(BUSINESS_NAME)', gen_random_uuid()) ON CONFLICT (id) DO NOTHING;" \
	  -c "INSERT INTO admins (business_id, channel, external_id, name) VALUES (1, 'telegram', '$(ADMIN_TG_ID)', 'owner') ON CONFLICT DO NOTHING;"

# Point Telegram at your webhook. Requires ASKDESK_TELEGRAM_BOT_TOKEN,
# ASKDESK_PUBLIC_URL, ASKDESK_TELEGRAM_WEBHOOK_SECRET.
set-webhook:
	curl -fsS "https://api.telegram.org/bot$(ASKDESK_TELEGRAM_BOT_TOKEN)/setWebhook" \
	  --data-urlencode "url=$(ASKDESK_PUBLIC_URL)/webhook/telegram" \
	  --data-urlencode "secret_token=$(ASKDESK_TELEGRAM_WEBHOOK_SECRET)"; echo

delete-webhook:
	curl -fsS "https://api.telegram.org/bot$(ASKDESK_TELEGRAM_BOT_TOKEN)/deleteWebhook"; echo

# Bulk-load FAQs from a JSON file. Usage: make load-faqs file=minipos_faqs.json
# Requires ASKDESK_DATABASE_URL, ASKDESK_GEMINI_API_KEY, ASKDESK_BUSINESS_ID.
load-faqs:
	go run ./cmd/loadfaqs -file $(file) $(if $(reset),-reset,)

# Build the container image locally.
docker-build:
	docker build -t askdesk:local .


# Local Postgres (pgvector) and Redis for development.
db-up:
	docker compose up -d postgres redis

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


# Run the web tier (info level — debug logs hidden).
run:
	go run ./cmd/askdesk

# Run the worker tier.
worker:
	go run ./cmd/worker

# Run the web tier with debug logging enabled (debug logs appear).
debug:
	ASKDESK_LOG_LEVEL=debug go run ./cmd/askdesk

# Compile both binaries to bin/.
build:
	go build -o bin/askdesk ./cmd/askdesk
	go build -o bin/worker ./cmd/worker

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

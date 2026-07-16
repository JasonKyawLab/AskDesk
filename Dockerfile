# --- build stage ---
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache dependencies first.
COPY go.mod go.sum ./
RUN go mod download

# Build both binaries. Migrations are embedded in the binary, so nothing else
# needs to ship. CGO off + static build so it runs on distroless.
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/askdesk ./cmd/askdesk
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/worker ./cmd/worker

# --- runtime stage ---
# distroless static + nonroot: minimal attack surface, no shell, non-root user.
FROM gcr.io/distroless/static:nonroot
COPY --from=build /out/askdesk /askdesk
COPY --from=build /out/worker /worker
USER nonroot:nonroot

# The all-in-one/web binary. For the worker tier, override with: ["/worker"].
ENTRYPOINT ["/askdesk"]

# Build stage
FROM golang:1.23-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /opspilot-api ./cmd/server && \
    CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /opspilot-worker ./cmd/worker

# Runtime stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates wget

WORKDIR /app

COPY --from=builder /opspilot-api /app/opspilot-api
COPY --from=builder /opspilot-worker /app/opspilot-worker

EXPOSE 8080

USER nobody

# Default to API; worker overrides entrypoint in docker-compose
ENTRYPOINT ["/app/opspilot-api"]

# Multi-stage build for Telegram LLM Bot

# Stage 1: Builder
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags='-w -s -extldflags "-static"' \
    -o telegram-llm-bot \
    ./cmd/bot

# Stage 2: Final minimal image
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 botuser && \
    adduser -D -u 1000 -G botuser botuser

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/telegram-llm-bot .

# Copy timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Set ownership
RUN chown -R botuser:botuser /app

# Switch to non-root user
USER botuser

# Health check (optional, can be removed if not needed)
# The bot doesn't expose HTTP endpoint, so this is a placeholder
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD pgrep telegram-llm-bot || exit 1

# Run the application
CMD ["./telegram-llm-bot"]

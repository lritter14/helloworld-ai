# Build stage
FROM golang:1.25.3-alpine AS builder

WORKDIR /build

# Install build dependencies (including CGO requirements for SQLite)
RUN apk add --no-cache git make gcc musl-dev sqlite-dev

# Enable CGO for SQLite support
ENV CGO_ENABLED=1

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build binary
RUN make build-api

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for HTTPS requests and SQLite runtime libraries
RUN apk --no-cache add ca-certificates sqlite-libs

# Copy binary from builder
COPY --from=builder /build/bin/helloworld-ai-api /app/helloworld-ai-api

# Create data directory for SQLite
RUN mkdir -p /app/data

# Expose API port
EXPOSE 9000

# Run the binary
CMD ["/app/helloworld-ai-api"]


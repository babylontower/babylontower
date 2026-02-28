# Babylon Tower - Docker Image for Testing
# Multi-stage build for minimal runtime image

FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

WORKDIR /build

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN make build

# Runtime stage
FROM alpine:3.19

# Install runtime dependencies
RUN apk add --no-cache ca-certificates

# Create non-root user
RUN addgroup -g 1000 babylon && \
    adduser -D -u 1000 -G babylon babylon

WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/bin/messenger /app/messenger

# Create data directory
RUN mkdir -p /home/babylon/.babylontower && \
    chown -R babylon:babylon /home/babylon

USER babylon

# Set environment variables
ENV HOME=/home/babylon
ENV BABYLON_DATA_DIR=/home/babylon/.babylontower

# Expose IPFS ports (for peer communication)
# TCP: 4001, WebSocket: 4002
EXPOSE 4001 4002

ENTRYPOINT ["/app/messenger"]

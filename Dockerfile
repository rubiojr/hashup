# Builder stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory for the build
WORKDIR /build

# Copy go.mod and go.sum files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# Using CGO_ENABLED=0 for static linking and setting GOARCH=amd64 for compatibility
RUN CGO_ENABLED=1 \
    go build -ldflags="-s -w" -o hashup .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata sqlite

# Create necessary directories
RUN mkdir -p /etc/hashup /data/hashup

# Add non-root user for running the service
RUN adduser -D -h /home/hashup hashup
RUN chown -R hashup:hashup /data/hashup

# Set working directory
WORKDIR /app

# Copy the compiled binary from the builder stage
COPY --from=builder /build/hashup /app/hashup
RUN chmod +x /app/hashup

# Copy default configurations
COPY configs/nats.conf /etc/hashup/nats.conf
COPY configs/hashup.toml /etc/hashup/config.toml

# Expose NATS ports
EXPOSE 4222 8222

# Volume for configuration and data
VOLUME ["/etc/hashup", "/data/hashup"]

# Switch to non-root user
USER hashup

ENTRYPOINT ["/app/hashup"]
# Run hashup nats server
CMD ["nats", "--config", "/etc/hashup/nats.conf", "--data-dir", "/data/hashup"]

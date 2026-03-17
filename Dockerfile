# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-libs && \
    rm -rf /var/cache/apk/*

WORKDIR /src

# Copy go mod files and download (cached layer)
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source code
COPY . .

# Build with minimal flags and no cache to save disk space
RUN CGO_ENABLED=1 GOOS=linux \
    GOCACHE=/tmp/go-cache \
    GOMODCACHE=/go/pkg/mod \
    go build -ldflags="-s -w" -a -installsuffix cgo -o signifer . && \
    rm -rf /tmp/go-cache /root/.cache

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache libcap ca-certificates sqlite-libs && \
    rm -rf /var/cache/apk/*

WORKDIR /

# Copy binary from builder
COPY --from=builder /src/signifer .

# Grant CAP_NET_RAW capability for ICMP pings
RUN setcap cap_net_raw=+ep /signifer

# Create directory for database
RUN mkdir -p /data

# Expose web port
EXPOSE 9090

# Run the application
CMD ["/signifer"]

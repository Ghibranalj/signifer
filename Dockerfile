# Build stage
FROM golang:1.25-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-libs

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o signifer .

# Runtime stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache libcap ca-certificates sqlite-libs

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/signifer .

# Grant CAP_NET_RAW capability for ICMP pings
RUN setcap cap_net_raw=+ep /app/signifer

# Create directory for database
RUN mkdir -p /data

# Expose web port
EXPOSE 9090

# Run the application
CMD ["/app/signifer"]

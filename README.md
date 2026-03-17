# Signifer

<img src="images/signifer.png" alt="Signifer" height="400">

> A *signifer* was a standard bearer of the Roman legions. He carried a *signum* (military standard) for a cohort or century.

Signifer is a network device monitoring application that tracks device availability via ICMP pings and sends Discord webhook notifications when status changes occur.

## Features

- **ICMP Monitoring**: Ping devices (IPv4/IPv6) with configurable intervals
- **State Tracking**: Detects device status changes (online ↔ offline)
- **Discord Notifications**: Rich embeds with color coding, latency, and failure reasons
- **Web UI**: Device management interface powered by Templ and Tailwind CSS
- **SQLite Database**: Lightweight data persistence
- **Basic Authentication**: Secure web interface access

## Quick Start

### Docker (Recommended)

```bash
# Create config.yaml from example
cp config.yaml.example config.yaml
# Edit config.yaml and add your Discord webhook URL

docker-compose up -d
```

Access at http://localhost with credentials `admin`/`admin`.

### From Binary

```bash
# Build
./build.sh

# Create config.yaml
cat > config.yaml << EOF
discord:
  webhook_url: "YOUR_DISCORD_WEBHOOK_URL"
EOF

# Run
./signifer
```

## Configuration

Create a `config.yaml` file:

```yaml
# Web server port (default: 9090)
port: 9090

# Discord webhook URL (REQUIRED)
discord:
  webhook_url: "https://discord.com/api/webhooks/YOUR_WEBHOOK_ID/YOUR_WEBHOOK_TOKEN"

# Basic authentication (default: admin/admin)
auth:
  user: "admin"
  password: "secure_password"

# Ping configuration
ping:
  interval_seconds: 30  # How often to ping devices (default: 30)
```

### Using Docker

The `config.yaml` file is mounted into the container at `/app/config.yaml`. Create this file before running:

```bash
# Create config.yaml on host
cat > config.yaml << EOF
discord:
  webhook_url: "YOUR_DISCORD_WEBHOOK_URL"
EOF

# Run with docker-compose
docker-compose up -d
```

## Discord Notifications

Signifer sends notifications when:

- A device goes **offline** (red embed)
  - Includes hostname, previous state, and reason (DNS resolution error, timeout, etc.)

- A device comes **back online** (green embed)
  - Includes hostname, previous state, current state, and latency

### Example Notification

**Device Offline:**
```
┌─────────────────────────────────────────┐
│ Device Offline                          │
│ Database Server is no longer responding │
├─────────────────────────────────────────┤
│ Hostname:       db01.example.com        │
│ Previous State: Online                  │
│ Current State:  Offline                 │
│ Reason:         DNS resolution error    │
└─────────────────────────────────────────┘
```

## Building from Source

### Prerequisites

- Go 1.25+
- GCC (for SQLite)

### Build

```bash
go build -o signifer .
sudo setcap cap_net_raw=+ep ./signifer  # Grant ICMP capability
```

### Development

```bash
# Generate templ code
templ generate

# Generate SQL code
sqlc generate

# Run
go run .
```

## License

MIT

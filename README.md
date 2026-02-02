# RFC-2217 NAT Proxy

TCP proxy for COM port emulation over RFC-2217 protocol, designed for IoT devices behind NAT.

**[Документация на русском языке](README_RU.md)**

## License

MIT License - see [LICENSE](LICENSE) for details. Attribution required when using this project.

## Architecture

```
┌─────────────┐         ┌───────────────────────┐         ┌─────────────┐
│ IoT Device  │────────▶│    RFC-2217 Proxy     │◀────────│   Client    │
│ (behind NAT)│  :2217  │                       │  :2217  │ (PC/Server) │
│ ESP8266/32  │         │  ┌─────────────────┐  │         │             │
└─────────────┘         │  │ Device Registry │  │         └─────────────┘
                        │  │ Session Manager │  │
                        │  │ HTTP API :8080  │  │
                        │  └─────────────────┘  │
                        └───────────────────────┘
```

## Ports

| Port | Purpose |
|------|---------|
| 2217 | Device and client connections (unified port) |
| 8080 | HTTP API and health checks |

## Quick Start

```bash
# Build and run locally
make run

# Or using Docker
make docker-build
docker run -p 2217:2217 -p 8080:8080 registry.jad.ru/proxy-rfc2217:latest
```

## Configuration

Environment variables:

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | 2217 | Port for connections (devices and clients) |
| `API_PORT` | 8080 | Port for HTTP API and web interface |
| `AUTH_TOKEN` | (empty) | Device authentication token |
| `WEB_USER` | admin | Web interface login (Basic Auth) |
| `WEB_PASS` | admin | Web interface password (Basic Auth) |
| `KEEPALIVE` | 30 | TCP keepalive interval in seconds |
| `INIT_TIMEOUT` | 5 | Timeout for AT command on connection in seconds |

## Protocol

AT commands are used. Devices and clients connect to the same port and are distinguished by their first AT command.

### Device Registration

A device connects to the port and sends a registration AT command:

```
AT+REG=<token>\r\n
```

The `<token>` format depends on the `AUTH_TOKEN` setting:

- **Without authentication** (`AUTH_TOKEN` not set): `<token>` = DEVICE_ID
  ```
  AT+REG=DEVICE_001\r\n
  ```

- **With authentication** (`AUTH_TOKEN` set): `<token>` = `AUTH_TOKEN+DEVICE_ID`
  ```
  AT+REG=secret123+DEVICE_001\r\n
  ```

**Response:**
```
OK\r\n
```

or on error:
```
ERROR\r\n
```

After registration, the connection is maintained via TCP keepalive (no additional commands required from the device).

### Client Connection

A client connects to the same port and sends:

```
AT+CONNECT=<token>\r\n
```

The `<token>` format is the same as for device registration:

- **Without authentication** (`AUTH_TOKEN` not set): `<token>` = DEVICE_ID
  ```
  AT+CONNECT=DEVICE_001\r\n
  ```

- **With authentication** (`AUTH_TOKEN` set): `<token>` = `AUTH_TOKEN+DEVICE_ID`
  ```
  AT+CONNECT=secret123+DEVICE_001\r\n
  ```

**Response:**
```
OK\r\n
```

or on error:
```
ERROR\r\n
```

After receiving `OK`, the connection enters transparent data transfer mode (RFC-2217 bridge).

## HTTP API

```
GET /                  # Web interface (Basic Auth)
GET /healthz           # Liveness probe
GET /readyz            # Readiness probe
GET /api/v1/devices    # List connected devices
GET /api/v1/sessions   # List active sessions
GET /api/v1/stats      # Statistics
```

The web interface is available at `http://localhost:8080/` and is protected by Basic Auth (default admin:admin).

### Response Examples

**GET /api/v1/devices:**
```json
{
  "count": 1,
  "devices": [
    {
      "id": "DEVICE_001",
      "registered_at": "2024-01-15T10:30:00Z",
      "in_session": false,
      "remote_addr": "192.168.1.100:54321"
    }
  ]
}
```

**GET /api/v1/sessions:**
```json
{
  "count": 1,
  "sessions": [
    {
      "id": "sess_1705312200_1",
      "device_id": "DEVICE_001",
      "client_addr": "10.0.0.5:12345",
      "device_addr": "192.168.1.100:54321",
      "started_at": "2024-01-15T10:30:00Z",
      "duration_secs": 120.5,
      "bytes_in": 1024,
      "bytes_out": 2048
    }
  ]
}
```

## Testing

### Device Emulation (using netcat)

```bash
# Connect to proxy as a device
nc localhost 2217

# Without AUTH_TOKEN:
AT+REG=DEVICE_001
# Response: OK

# With AUTH_TOKEN=secret:
AT+REG=secret+DEVICE_001
# Response: OK

# Connection remains open (TCP keepalive)
```

### Client Connection

```bash
# Connect as a client
nc localhost 2217

# Without AUTH_TOKEN:
AT+CONNECT=DEVICE_001
# Response: OK

# With AUTH_TOKEN=secret:
AT+CONNECT=secret+DEVICE_001
# Response: OK (then transparent channel to device)
```

### Device Test Script

```bash
#!/bin/bash
# test_device.sh - Simple device emulator

HOST=${1:-localhost}
PORT=${2:-2217}
DEVICE_ID=${3:-DEVICE_001}
AUTH_TOKEN=${4:-}  # Optional

if [ -n "$AUTH_TOKEN" ]; then
  TOKEN="${AUTH_TOKEN}+${DEVICE_ID}"
else
  TOKEN="${DEVICE_ID}"
fi

{
  echo -e "AT+REG=${TOKEN}\r"
  cat  # Keep connection open
} | nc $HOST $PORT
```

### Client Test Script

```bash
#!/bin/bash
# test_client.sh - Test client

HOST=${1:-localhost}
PORT=${2:-2217}
DEVICE_ID=${3:-DEVICE_001}
AUTH_TOKEN=${4:-}  # Optional

if [ -n "$AUTH_TOKEN" ]; then
  TOKEN="${AUTH_TOKEN}+${DEVICE_ID}"
else
  TOKEN="${DEVICE_ID}"
fi

{
  echo -e "AT+CONNECT=${TOKEN}\r"
  cat  # Transparent stdin transfer
} | nc $HOST $PORT
```

## Kubernetes Deployment

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/deployment.yaml
```

### Creating Secrets

```bash
# Generate random tokens
AUTH_TOKEN=$(openssl rand -hex 16)
WEB_PASS=$(openssl rand -hex 16)

# Create secret
kubectl -n waterius create secret generic proxy-rfc2217-secrets \
  --from-literal=auth-token=$AUTH_TOKEN \
  --from-literal=web-user=admin \
  --from-literal=web-pass=$WEB_PASS \
  --dry-run=client -o yaml | kubectl apply -f -

# View generated values
echo "AUTH_TOKEN: $AUTH_TOKEN"
echo "WEB_PASS: $WEB_PASS"
```

## Building

```bash
make build          # Build for Linux amd64
make build-local    # Build for current platform
make docker-build   # Build Docker image
make docker-push    # Push to registry
make release        # Full pipeline
make test           # Run tests
make lint           # Format and check
```

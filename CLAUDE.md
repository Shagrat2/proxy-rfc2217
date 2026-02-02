# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

MeterRS485 is a universal counter/meter reading system via RS485 channels. It consists of:
- **Proxy component** (`proxy/`): Go HTTP proxy forwarding requests to `iz.waterius.ru`
- **RFC-2217 NAT Proxy** (this repo): Go TCP proxy for RFC-2217 COM emulation with NAT traversal
- **Embedded firmware** (`software/`): C++/Arduino firmware for ESP8266/ESP32

## Build Commands

### RFC-2217 NAT Proxy (this repo)

```bash
make build          # Build Go binary (linux/amd64, static)
make build-local    # Build for current platform
make run            # Run locally
make docker-build   # Build Docker image
make docker-push    # Push to registry.jad.ru
make release        # Full pipeline: build → docker-build → docker-push
make test           # Run tests
```

**Ports:**
| Port | Description |
|------|-------------|
| 2217 | Device and client connections (AT commands) |
| 8080 | HTTP API & health checks |

### Go Proxy (`proxy/` directory)

```bash
make build          # Build Go binary (linux/amd64, static)
make docker-build   # Build Docker image
make docker-push    # Push to registry.jad.ru
make release        # Full pipeline: build → docker-build → docker-push
make clean          # Remove binary
```

### Embedded Firmware (`software/` directory)

```bash
pio run -e rs485_hw2_0      # Build for ESP32-C3 RS485 HW v2.0
pio run -e rs485_hw1_2      # Build for ESP8266 RS485 HW v1.2
pio upload -e rs485_hw2_0   # Upload firmware
pio test -e native          # Run unit tests (Unity framework)
```

Key build environments: `rs485_hw0_2`, `rs485_hw1_2`, `rs485_hw2_0`, `rs485_hw1_2_gsm`, `opto_hw2_0`, `rs232_hw2_0`, `mbus_hw2_0`

## Architecture

### RFC-2217 NAT Proxy (this repo)

Entry point: `cmd/proxy/main.go`

TCP proxy for IoT devices behind NAT using AT commands:
- Devices connect to proxy on port 2217 with `AT+REG=<token>\r\n`
- Clients connect to proxy on port 2217 with `AT+CONNECT=<token>\r\n`
- Proxy bridges RFC-2217 traffic bidirectionally

**AT Command Protocol:**
```
Device registration:  AT+REG=<token>\r\n     → OK\r\n or ERROR\r\n
Client connection:    AT+CONNECT=<token>\r\n → OK\r\n or ERROR\r\n
```

Token format: `DEVICE_ID` (no auth) or `AUTH_TOKEN+DEVICE_ID` (with auth).
Connection is maintained via TCP keepalive (no application-level heartbeat needed).

**Key components:**
- `internal/connection/` - Unified server handling both devices and clients
- `internal/device/` - Device registry
- `internal/session/` - Session management and data bridging
- `internal/api/` - HTTP API (`/healthz`, `/api/v1/devices`, etc.)

### Proxy (Go)

Entry point: `proxy/app/main.go`

HTTP proxy on port 8080 that:
- Forwards requests to `iz.waterius.ru` with request/response logging
- Validates `Waterius-Token` header (returns 401 if missing)
- Provides Kubernetes health probes (`/healthz`, `/readyz`)
- Uses `InsecureSkipVerify` for SSL (intentional for transparent proxying)

### Embedded Firmware (C++)

Entry point: `software/src/main.cpp`

Layered architecture:
```
Application (main.cpp, settings)
    ↓
Services (srvMeter, srvWeb, srvWIFI, sendMQTT, srvTcp)
    ↓
Protocols (ProtObj → Mercury, Pulsar, DLMS, Modbus, etc.)
    ↓
Channels (RS485, RS232, MBus, Opto)
    ↓
Hardware (ESP8266/ESP32, MAX485, UART)
```

**Key Services:**
- `srvMeter` - Meter polling and protocol detection
- `srvWeb` - AsyncWebServer with REST API and WebSocket
- `srvTcp` - TCP server (port 502) for RAW/RFC2217/Modbus RTU modes
- `sendMQTT` - MQTT with Home Assistant autodiscovery
- `sendWaterius` - Cloud integration with HMAC-SHA256

**Operating Modes:** `M_METER` (auto-polling), `M_RAW` (passthrough), `M_RFC2217` (COM emulation), `M_TCP_RTU` (Modbus over TCP)

**Adding new protocols:** Inherit from `ProtObj` base class in `software/src/prot/`

## Key Files

- `README.md` - RFC-2217 NAT Proxy documentation
- `cmd/proxy/main.go` - Proxy entry point
- `internal/connection/` - Unified connection server (devices and clients)
- `internal/device/` - Device registry
- `internal/session/` - Session management
- `k8s/` - Kubernetes deployment manifests
- `software/ARCHITECTURE.md` - Detailed firmware architecture documentation
- `proxy/PLAN.md` - Proxy design specification
- `proxy/k8s/` - Kubernetes deployment manifests
- `platformio.ini` - Firmware build configuration
- `secrets.ini.template` - Template for credentials (copy to `secrets.ini`)

## Debugging

VSCode configurations available in `.vscode/` directories:
- Go debugger: `proxy/.vscode/launch.json`
- Build tasks: `proxy/.vscode/tasks.json`

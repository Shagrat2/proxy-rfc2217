# Connection Sequence Diagram

## Client Connection Flow

```
┌────────┐          ┌─────────┐          ┌────────┐
│ Client │          │  Proxy  │          │ Device │
└───┬────┘          └────┬────┘          └───┬────┘
    │                    │                   │
    │  TCP Connect       │                   │
    │───────────────────>│                   │
    │                    │                   │
    │  [RFC2217 presets] │  (optional, before AT command)
    │  55 AA 55 ...      │  (USR-VCOM 8 bytes)
    │  or                │
    │  FF FA 2C ...      │  (RFC2217 telnet)
    │───────────────────>│                   │
    │                    │                   │
    │  AT+CONNECT=token  │                   │
    │───────────────────>│                   │
    │                    │                   │
    │  [RFC2217 presets] │  (optional, after AT command - BUFFERED)
    │  FF FA 2C ...      │
    │───────────────────>│                   │
    │                    │                   │
    │                    │  (validate token) │
    │                    │  (find device)    │
    │                    │  (create session) │
    │                    │                   │
    │  OK\r\n            │                   │
    │<───────────────────│                   │
    │                    │                   │
    │                    │  RFC2217 presets  │
    │                    │  FF FA 2C ...     │
    │                    │──────────────────>│
    │                    │                   │
    │  ATDT\r\n          │                   │
    │───────────────────>│──────────────────>│
    │                    │                   │
    │                    │  OK\r\n (or data) │
    │<───────────────────│<──────────────────│
    │                    │                   │
    │  ═══════ Bidirectional Bridge ═══════  │
    │<──────────────────>│<─────────────────>│
    │                    │                   │
```

## Device Registration Flow

```
┌────────┐          ┌─────────┐
│ Device │          │  Proxy  │
└───┬────┘          └────┬────┘
    │                    │
    │  TCP Connect       │
    │───────────────────>│
    │                    │
    │  AT+REG=token      │
    │───────────────────>│
    │                    │
    │                    │  (validate token)
    │                    │  (register device)
    │                    │
    │  OK\r\n            │
    │<───────────────────│
    │                    │
    │  ATDT (optional)   │  (1s timeout)
    │───────────────────>│
    │                    │
    │  OK\r\n            │
    │<───────────────────│
    │                    │
    │  ══ Wait for client or disconnect ══
    │                    │
```

## Timeouts

| Parameter | Default | Env Variable | Description |
|-----------|---------|--------------|-------------|
| InitTimeout | 5s | `INIT_TIMEOUT` | Timeout for initial AT command |
| ATDT wait | 1s | (hardcoded) | Wait for optional ATDT after registration |
| KeepAlive | 30s | `KEEPALIVE` | TCP keepalive period |

## Protocol Detection

### Before AT Command (skipped bytes)

```
┌─────────────────────────────────────────────────────────────┐
│  Incoming data before "AT" detected                         │
├─────────────────────────────────────────────────────────────┤
│  55 AA 55 ... (8 bytes)  →  USR-VCOM Baud Rate Sync         │
│  FF FA 2C ... (variable) →  RFC2217 COM-PORT-OPTION         │
│  Other                   →  Logged and passed through       │
└─────────────────────────────────────────────────────────────┘
```

### After AT Command (buffered bytes)

```
┌─────────────────────────────────────────────────────────────┐
│  Data in buffer after AT+CONNECT\r\n                        │
├─────────────────────────────────────────────────────────────┤
│  55 AA 55 ... (8 bytes)  →  USR-VCOM → Convert to RFC2217   │
│  FF FA 2C ... (variable) →  RFC2217 → Parse and forward     │
│  Other                   →  Forward as-is to device         │
└─────────────────────────────────────────────────────────────┘
```

## RFC2217 Commands (COM-PORT-OPTION)

| Command | Code | Data | Example |
|---------|------|------|---------|
| SET-BAUDRATE | 0x01 | 4 bytes BE | `FF FA 2C 01 00 00 25 80 FF F0` = 9600 |
| SET-DATASIZE | 0x02 | 1 byte | `FF FA 2C 02 08 FF F0` = 8 bits |
| SET-PARITY | 0x03 | 1 byte | `FF FA 2C 03 00 FF F0` = NONE |
| SET-STOPSIZE | 0x04 | 1 byte | `FF FA 2C 04 01 FF F0` = 1 stop |
| SET-CONTROL | 0x05 | 1 byte | `FF FA 2C 05 01 FF F0` = flow control |

### Parity Values
- 0 = NONE
- 1 = ODD
- 2 = EVEN
- 3 = MARK
- 4 = SPACE

## USR-VCOM Baud Rate Sync (8 bytes)

```
55 AA 55 [baud 3B] [param] [checksum]
         ├─ Big-endian 24-bit
                     ├─ Bit 0-1: data bits (00=5, 01=6, 10=7, 11=8)
                     ├─ Bit 2: stop bits (0=1, 1=2)
                     ├─ Bit 3: parity enable
                     └─ Bit 4-5: parity type (00=ODD, 01=EVEN, 10=Mark, 11=Space)
```

## Log Messages Sequence

### Successful Client Connection

```
[conn] new connection from 10.42.0.1:12345
[protocol] RFC2217 skipped 38 bytes: fffa2c...     (if before AT)
[conn] 10.42.0.1:12345: received command: AT+CONNECT param: "device-id"
[client] 10.42.0.1:12345: received 5 port presets:  (if presets found)
[client] 10.42.0.1:12345:   - SET-BAUDRATE: 9600
[client] 10.42.0.1:12345:   - SET-DATASIZE: 8 bits
[client] 10.42.0.1:12345:   - SET-PARITY: NONE
[client] 10.42.0.1:12345:   - SET-STOPSIZE: 1
[client] 10.42.0.1:12345:   - SET-CONTROL: 1
[client] 10.42.0.1:12345: requesting session with device device-id
[session] started: id=sess_xxx device=device-id
[client] 10.42.0.1:12345: created session sess_xxx with device device-id
[rfc2217] forwarding 38 bytes to device: fffa2c...
[client] 10.42.0.1:12345: received 5 buffered port presets:  (if after AT)
[bridge] sess_xxx client->device: 5 bytes
[bridge] sess_xxx device->client: 4 bytes
...
[client] 10.42.0.1:12345: session sess_xxx ended
```

### Successful Device Registration

```
[conn] new connection from 192.168.1.100:54321
[conn] 192.168.1.100:54321: received command: AT+REG param: "device-id"
[device] 192.168.1.100:54321: registered device device-id
[device] device-id: received ATDT
[device] device-id: context cancelled
```

## Error Cases

| Error | Log Message | Response |
|-------|-------------|----------|
| Timeout | `init timeout (no data received in 5s)` | `ERROR\r\n` |
| Unknown command | `unexpected command: XXX` | `ERROR\r\n` |
| Empty token | `empty token` | `ERROR\r\n` |
| Device not found | `device XXX not found` | `ERROR\r\n` |
| Device busy | `device XXX is busy` | `ERROR\r\n` |
| Invalid auth | `invalid auth token` | `ERROR\r\n` |

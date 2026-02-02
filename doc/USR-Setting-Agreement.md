# USR Setting Agreement Protocol

Протокол конфигурации устройств серии 410s, N5xx, E2, ED2.

> См. также: [Все протоколы USR](USR-Protocols.md)

## Overview

- **Транспорт:** UDP broadcast
- **Destination IP:** 255.255.255.255
- **Port:** 1901

## Packet Format

```
FF [len] [cmd] [MAC 6 bytes] [username 6 bytes] [password 6 bytes] [params...] [checksum]
```

| Field | Size | Description |
|-------|------|-------------|
| Header | 1 | `FF` |
| Length | 1 | Длина пакета от cmd до checksum |
| Command | 1 | Код команды |
| MAC | 6 | MAC-адрес устройства |
| Username | 6 | 5 символов + `00` padding |
| Password | 6 | 5 символов + `00` padding |
| Params | var | Параметры (зависят от команды) |
| Checksum | 1 | Sum check |

## Commands

| Command | Code | Description |
|---------|------|-------------|
| Search | `FF 01 01 02` | Поиск устройств в сети |
| Reset | `FF xx 02` | Перезагрузка |
| Read Config | `FF xx 03` | Чтение конфигурации |
| Store Settings | `FF xx 04` | Сохранение в flash |
| Basic Settings | `FF xx 05` | Основные настройки |
| Serial Port 0 | `FF xx 06` | Настройки порта 0 |
| Serial Port 1 | `FF xx 07` | Настройки порта 1 |
| Serial Port 2 | `FF xx 08` | Настройки порта 2 |
| USR Cloud | `FF xx 10` | Настройки облака |

## Checksum Algorithm

**Request:** Сумма всех байт от **length** до последнего параметра (без checksum), младший байт:

```
checksum = (len + cmd + MAC[0..5] + user[0..5] + pass[0..5] + params[...]) & 0xFF
```

**Response:** Используется алгоритм **вычитания** — начиная с 0x00, последовательно вычитаем каждый байт.

## Responses

### Response Format

```
FF [len] [cmd] [status] [data...] [checksum]
```

### Status Codes

| Status | Hex | Meaning |
|--------|-----|---------|
| `K` | `4B` | Success (OK) |
| `E` | `45` | Error |
| `P` | `50` | Password wrong |

### Command Responses

| Command | Success Response | Error Response |
|---------|------------------|----------------|
| Search | `FF 24 01 ...` (36 bytes) | — |
| Reset | `FF 01 02 4B` | `FF 01 02 45` |
| Read Config | 180 bytes (params) | `FF 01 03 45` |
| Store Settings | `FF 01 04 4B` | `FF 01 04 45` |
| Restart | `FF 01 05 4B` | `FF 01 05 45` |

### Search Response Format (36 bytes)

```
FF 24 01 [status] [IP 4 bytes] [MAC 6 bytes] [firmware] [device_name...] [checksum]
```

Example:
```
FF 24 01 00 C0 A8 01 6B D8 B0 4C C0 0D 65 ...
         ^status ^-- IP ---^ ^--- MAC ---^
```
- IP: `C0 A8 01 6B` = 192.168.1.107
- MAC: `D8 B0 4C C0 0D 65`

## Example

```
FF 56 05 AC CF 23 66 66 67 61 64 6D 69 6E 00 61 64 6D 69 6E 00 ...
   ^len ^cmd ^---- MAC ----^ ^-- username --^ ^-- password --^
```

- Length: `56` (86 bytes)
- Command: `05` (Basic Settings)
- MAC: `AC CF 23 66 66 67`
- Username: `admin\0` (`61 64 6D 69 6E 00`)
- Password: `admin\0` (`61 64 6D 69 6E 00`)

## Serial Port Parameters

**Отличаются от Baud Rate Sync протокола!**

| Parameter | Size | Values |
|-----------|------|--------|
| Baud Rate | 4 | Little-endian 32-bit |
| Data Bits | 1 | `0x05`, `0x06`, `0x07`, `0x08` |
| Parity | 1 | `1`=none, `2`=odd, `3`=even, `4`=mark, `5`=space |
| Stop Bits | 1 | `0x01` или `0x02` |
| Flow Control | 1 | `0x01`=none, `0x03`=hardware |

### Baud Rate Examples

| Baud Rate | Hex (32-bit LE) |
|-----------|-----------------|
| 9600 | `00 25 80 00` |
| 115200 | `00 C2 01 00` |

## Supported Devices

- USR-TCP232-410s
- USR-N510 / N520 / N540
- USR-TCP232-E2
- USR-TCP232-ED2

## References

- [Setting Agreement Document](https://usermanual.wiki/m/d5f892ba52ffedc22bbb1ca72d64ac3eb6078c75eae35e263cb67b8d4ff6ae0e)
- [USR-N540 User Manual](https://www.pusr.com/download/M4/USR-N540-User-Manual_V1.1.0.01.pdf)
- [USR-TCP232-E2 User Manual](https://www.pusr.com/download/User%20Manual/USR-TCP232-E2-user-mannual-V1.1.3.pdf)

## Implementation Status

- [x] Формат пакета определён
- [x] Команды определены
- [x] Параметры серийного порта определены
- [ ] Требуется проверка на реальном устройстве

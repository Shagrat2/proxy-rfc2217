# USR M0/T24 Series Configuration Protocol

Протокол конфигурации устройств серии M0 (TCP232-302/304/306) и T24.

> См. также: [Все протоколы USR](USR-Protocols.md)

## Overview

**Similar T24 Setting Protocol:**
- Команда: 40 байт
- Ответ: 35 байт
- Транспорт: UDP broadcast или serial config mode

## Commands

| Command | Header | Description |
|---------|--------|-------------|
| Read Basic | `55 BD` | Чтение основных параметров |
| Write Basic | `55 BF` | Запись основных параметров (65 байт + checksum) |
| Read Port0 | `55 BC` | Чтение параметров порта 0 |
| Write Port0 | `55 BA` | Запись параметров порта 0 |
| Read Port1 | `55 C3` | Чтение параметров порта 1 |
| Write Port1 | `55 C1` | Запись параметров порта 1 |
| Read Port2 | `55 C4` | Чтение параметров порта 2 (только -500) |
| Write Port2 | `55 C2` | Запись параметров порта 2 |
| Reset | `55 B1 5A` | Перезагрузка → ответ `BA 4B` |

## Write Basic Command Format (`55 BF`)

```
55 BF [params 63 bytes] [checksum]
```

Общий размер: 65 байт + checksum

## Serial Config Mode

После удержания кнопки **Reload** при включении:
1. Модуль переходит в режим конфигурации через serial
2. Serial параметры: **9600 8N1**
3. Модуль отправляет символ `U` для индикации готовности

## Checksum

Sum check — сумма всех байт (кроме checksum), младший байт.

## Responses

### Response Format

Ответ на команду: **35 байт** (Similar T24 protocol)

### Status Codes

| Status | Hex | Meaning |
|--------|-----|---------|
| `K` | `4B` | Success (OK) |
| `P` | `50` | Password wrong |
| `E` | `45` | Error |

### Command Responses

| Command | Success | Error |
|---------|---------|-------|
| Reset (`55 B1 5A`) | `BA 4B` | — |
| Read Basic (`55 BD`) | 35 bytes data | `FF 01 05 50` (wrong pass) |
| Write Basic (`55 BF`) | `FF 01 06 4B` | `FF 01 05 50` |
| Read Port (`55 BC`) | Port params | `FF 01 05 50` |
| Write Port (`55 BA`) | `FF 01 06 4B` | `FF 01 05 50` |

## Supported Devices

- USR-TCP232-302
- USR-TCP232-304
- USR-TCP232-306
- USR-TCP232-T24 series

## References

- [USR-TCP232-302 Manual](https://www.manualslib.com/manual/1197222/Usr-Iot-Usr-Tcp232-302.html)
- [USR-TCP232-304 Manual](https://usriot.ru/download/M0/USR-TCP232-304-User%20Manual-V1.1.pdf)
- [PUSR Specification](https://www.pusr.com/uploads/20230706/USR-TCP232-302-304-306-spec-V1.0.0-20230706112316.pdf)

## Implementation Status

- [x] Команды определены
- [ ] Формат параметров требует уточнения
- [ ] Checksum алгоритм требует проверки

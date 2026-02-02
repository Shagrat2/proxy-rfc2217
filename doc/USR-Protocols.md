# USR Protocols Overview

Обзор протоколов устройств USR IOT (PUSR).

## Протоколы

| Протокол | Header | Назначение | Устройства |
|----------|--------|------------|------------|
| [Baud Rate Sync](USR-VCOM.md) | `55 AA 55` | Inline-синхронизация COM порта | USR-VCOM software |
| [M0/T24 Config](USR-M0-Config.md) | `55 BD/BF` | Конфигурация устройства | TCP232-302/304/306 |
| [Setting Agreement](USR-Setting-Agreement.md) | `FF xx` | Конфигурация по UDP | 410s, N5xx, E2, ED2 |

## Сравнение

| Аспект | Baud Rate Sync | M0 Config | Setting Agreement |
|--------|----------------|-----------|-------------------|
| **Header** | `55 AA 55` | `55 BD/BF/BC/BA` | `FF` |
| **Размер** | 8 байт | 40-65 байт | Variable |
| **Транспорт** | TCP inline | UDP/Serial | UDP port 1901 |
| **Аутентификация** | Нет | Опционально | MAC + user/pass |
| **Baud Rate** | 24-bit BE | 32-bit LE | 32-bit LE |
| **Persistence** | Временно | Постоянно | Постоянно |
| **Response** | ❌ Нет | ✅ 35 байт | ✅ 4-180 байт |

## Response Codes

Общие коды статуса для M0 Config и Setting Agreement:

| Status | Hex | Meaning |
|--------|-----|---------|
| `K` | `4B` | Success (OK) |
| `E` | `45` | Error |
| `P` | `50` | Password wrong |

**Baud Rate Sync** — inline-протокол без подтверждения (fire-and-forget).

## Baud Rate Encoding

| Протокол | Format | 9600 | 115200 |
|----------|--------|------|--------|
| Baud Rate Sync | 24-bit BE | `00 25 80` | `01 C2 00` |
| M0 Config | 32-bit LE | `80 25 00 00` | `00 C2 01 00` |
| Setting Agreement | 32-bit LE | `80 25 00 00` | `00 C2 01 00` |

## Serial Parameters Encoding

### Baud Rate Sync (битовое поле)

```
Bit 1-0: Data bits (00=5, 01=6, 10=7, 11=8)
Bit 2:   Stop bits (0=1, 1=2)
Bit 3:   Parity enable
Bit 5-4: Parity type (00=ODD, 01=EVEN, 10=Mark, 11=Space)
```

### Setting Agreement (отдельные байты)

| Parameter | Values |
|-----------|--------|
| Data Bits | `0x05`-`0x08` |
| Parity | `1`=none, `2`=odd, `3`=even, `4`=mark, `5`=space |
| Stop Bits | `0x01`, `0x02` |

## References

- [Baud Rate Synchronization Function Manual](https://www.pusr.com/news/684.html)
- [USR-TCP232-304 User Manual](https://usriot.ru/download/M0/USR-TCP232-304-User%20Manual-V1.1.pdf)
- [USR-N540 User Manual](https://www.pusr.com/download/M4/USR-N540-User-Manual_V1.1.0.01.pdf)
- [PUSR Specification](https://www.pusr.com/uploads/20230706/USR-TCP232-302-304-306-spec-V1.0.0-20230706112316.pdf)

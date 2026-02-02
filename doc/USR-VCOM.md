# USR-VCOM Baud Rate Synchronization Protocol

Пакеты отправляемые USR-VCOM перед AT командой — **Baud Rate Synchronization Protocol**.

> См. также: [Все протоколы USR](USR-Protocols.md)

## Packet Format (8 bytes)

```
55 AA 55 [baud_hi] [baud_mid] [baud_lo] [param] [checksum]
```

| Offset | Size | Field | Description |
|--------|------|-------|-------------|
| 0 | 3 | Header | `55 AA 55` - сигнатура (снижает ложные срабатывания) |
| 3 | 3 | Baud Rate | Big-endian 24-bit, минимум 600 |
| 6 | 1 | Parameter | Data bits, parity, stop bits |
| 7 | 1 | Checksum | Сумма байтов 3-6 (без переноса) |

## Parameter Byte Structure

```
Bit 7-6: Reserved (00)
Bit 5-4: Parity type (when bit 3 = 1)
         00 = ODD
         01 = EVEN
         10 = Mark
         11 = Space (Clear)
Bit 3:   Parity enable (0 = disabled, 1 = enabled)
Bit 2:   Stop bits (0 = 1 bit, 1 = 2 bits)
Bit 1-0: Data bits (00=5, 01=6, 10=7, 11=8)
```

### Parameter Byte Examples

| Mode | Binary | Hex | Breakdown |
|------|--------|-----|-----------|
| 8N1 | `00000011` | `03` | data=8(11), stop=1(0), parity=off(0) |
| 8N2 | `00000111` | `07` | data=8(11), stop=2(1), parity=off(0) |
| 8E1 | `00011011` | `1B` | data=8(11), stop=1(0), parity=on(1), type=EVEN(01) |
| 8O1 | `00001011` | `0B` | data=8(11), stop=1(0), parity=on(1), type=ODD(00) |
| 7E1 | `00011010` | `1A` | data=7(10), stop=1(0), parity=on(1), type=EVEN(01) |

## Checksum Algorithm

**Сумма байтов 3-6 (baud rate + parameter), младший байт:**

```
checksum = (baud_hi + baud_mid + baud_lo + param) & 0xFF
```

## Samples (verified ✓)

| HEX Packet | Baud | Mode | Param | Checksum | Verification |
|------------|------|------|-------|----------|--------------|
| `55 AA 55 00 09 60 03 6C` | 2400 | 8N1 | `03` | `00+09+60+03=6C` ✓ |
| `55 AA 55 00 25 80 03 A8` | 9600 | 8N1 | `03` | `00+25+80+03=A8` ✓ |
| `55 AA 55 00 25 80 1B C0` | 9600 | 8E1 | `1B` | `00+25+80+1B=C0` ✓ |
| `55 AA 55 00 01 2C 1B 48` | 300 | 8E1 | `1B` | `00+01+2C+1B=48` ✓ |
| `55 AA 55 00 01 2C 03 30` | 300 | 8N1 | `03` | `00+01+2C+03=30` ✓ |
| `55 AA 55 01 C2 00 03 C6` | 115200 | 8N1 | `03` | `01+C2+00+03=C6` ✓ |

## Baud Rate Values

| Baud Rate | Hex (24-bit BE) |
|-----------|-----------------|
| 300 | `00 01 2C` |
| 600 | `00 02 58` |
| 1200 | `00 04 B0` |
| 2400 | `00 09 60` |
| 4800 | `00 12 C0` |
| 9600 | `00 25 80` |
| 19200 | `00 4B 00` |
| 38400 | `00 96 00` |
| 57600 | `00 E1 00` |
| 115200 | `01 C2 00` |
| 230400 | `03 84 00` |

## Поведение

- Изменения вступают в силу **немедленно**
- После перезагрузки модуля параметры **возвращаются к исходным**
- Протокол используется для **синхронизации** между виртуальным COM-портом и физическим устройством
- **Без подтверждения (ACK)** — устройство не отправляет ответ

## Response

**Нет ответа.** Это inline-протокол — данные отправляются в потоке перед AT командой, устройство применяет настройки без подтверждения.

## References

- [Baud Rate Synchronization Function Manual](https://www.pusr.com/news/684.html) — **основной источник**
- [USR-TCP232-302 User Manual (Archive.org)](https://archive.org/stream/manualzilla-id-5920103/5920103_djvu.txt) — полный мануал
- [U.S. Converters USC520 Manual - Similar RFC2217](https://www.manualslib.com/manual/1311911/U-S-Converters-Usc520.html?page=31)
- [USR-GPRS232-730 Manual - Similar RFC2217](https://www.manualslib.com/manual/1249898/Usr-Iot-Usr-Gprs232-730.html?page=16)
- [StackOverflow: RFC2217 encode/decode](https://stackoverflow.com/questions/71202858/synchronous-baud-rate-rfc2217-encode-decode)
- [USR-N520 Manual (PDF)](https://www.sarcitalia.it/file_upload/prodotti//USR-N520-Manual-EN-V1.0.4.pdf)

## Implementation Status

- [x] Протокол полностью декодирован
- [x] Checksum алгоритм подтверждён
- [x] Parameter byte структура определена
- [ ] Добавить поддержку парсинга в proxy

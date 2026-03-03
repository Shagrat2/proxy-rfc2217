# Changelog

## [Unreleased]

### Added — GSM-модем эмуляция

Поддержка подключения клиентов через стандартные модемные AT-команды (GSM-CSD режим).
Позволяет опросному ПО, которое умеет работать только через GSM-модем,
подключаться к устройствам через RFC-2217 прокси.

**Новый файл:** `internal/connection/modem.go`
- `ModemState` — состояние эмуляции модема (verbose/echo режим)
- Ответы в двух форматах: текстовый (`OK`, `CONNECT 9600`, `NO CARRIER`, `ERROR`) и числовой (`0`, `1`, `3`, `4`)
- Обработка стандартных AT-команд: `AT`, `ATZ`, `ATE0/1`, `ATV0/1`, `ATH`, `ATI`, `AT+CGMI`, `AT+CPIN?`, `AT+CSQ`, `ATS`, `AT&`, `AT+`

**Изменён:** `internal/connection/protocol.go`
- Добавлена константа `CmdModem` для generic модемных команд
- `parseATCommand()`: добавлено распознавание `ATD<номер>` (без суффикса T/P) как dial-команды
- `parseATCommand()`: любая строка с префиксом `AT` теперь распознаётся как `CmdModem` (вместо ошибки "unknown command")

**Изменён:** `internal/connection/handler.go`
- Модемная эмуляция активируется автоматически при получении первой generic AT-команды (ATZ, ATE0, ATV0 и т.д.)
- `ATD<номер>` / `ATDT<номер>` / `ATDP<номер>` в режиме эмуляции обрабатывается как `AT+CONNECT=<номер>`
- Успешное подключение → `CONNECT 9600` (или `1` в числовом режиме)
- Устройство не найдено / занято → `NO CARRIER` (или `3`)
- Завершение сессии → `NO CARRIER`
- Сигнатура `handleClient()` расширена параметром `modem *ModemState`

### Fixed — зависшие клиентские сессии после отключения клиента

**Изменён:** `internal/connection/handler.go`
- TCP keepalive не был настроен на клиентском соединении (только на устройствах)
- При потере связи с клиентом bridge не мог обнаружить мёртвое соединение — сессия висела до дефолтного TCP keepalive ОС (~2 часа на Linux)
- Добавлен `SetTCPKeepalive(idle=30s, interval=10s, count=3)` в `handleClient()` — мёртвое соединение обнаруживается за ~60 секунд

### Fixed — RFC2217 настройки порта терялись при GSM-модем подключении

**Изменён:** `internal/connection/handler.go`
- RFC2217 данные (скорость, биты данных, чётность, стоп-биты), полученные перед модемными AT-командами (ATV0, ATE0 и т.д.), не передавались на устройство при `ATD<номер>` dial
- Добавлена переменная `rfc2217Presets` для накопления RFC2217 данных из `cmd.Skipped` модемных команд
- При modem dial (`ATD<номер>`) сохранённые RFC2217 presets передаются в `handleClient()` → форвардятся на устройство

### Fixed — неправильное отображение parity в RFC2217 логах

**Изменён:** `internal/connection/rfc2217.go`
- `String()`: parity выводился с 0-based индексом (0=NONE, 1=ODD), а RFC2217 использует 1-based (1=NONE, 2=ODD)
- Значение 1 отображалось как "ODD" вместо "NONE"

### Fixed — USR-VCOM → RFC2217 конвертация parity

**Изменён:** `internal/connection/usrvcom.go`
- `ToRFC2217Commands()`: parity передавался без смещения — USR-VCOM 0=None отправлялся как RFC2217 0 (query), а не 1 (NONE)
- Добавлено `+1` при конвертации: USR-VCOM 0/1/2/3/4 → RFC2217 1/2/3/4/5

### Fixed — обработка +++ escape sequence

**Изменён:** `internal/connection/protocol.go`
- `ReadATCommandWithPresets()`: вместо полного таймаута (60с) для каждого чтения строки используется короткий per-line deadline (1с)
- Данные без CR/LF (например `+++` escape sequence модема) теперь обрабатываются за 1с, а не вызывают ERROR по таймауту
- Общий таймаут ожидания AT-команды (InitTimeout / PostConnectTimeout) сохранён — проверяется в цикле

### Fixed — совместимость ReadATCommand (timeout=0)

**Изменён:** `internal/connection/protocol.go`
- Per-line deadline (1с) теперь применяется **только** при timeout>0
- `ReadATCommand()` (timeout=0) больше не перезаписывает deadline, установленный вызывающим кодом
- Исправлена регрессия: `handleDevice()` ожидание ATDT после регистрации снова использует 60с deadline

### Fixed — modem-формат ошибок в handleClient

**Изменён:** `internal/connection/handler.go`
- Ранние ошибки в `handleClient()` (пустой токен, неверный формат, неверный auth token) теперь используют `NO CARRIER` в modem-режиме вместо plain `ERROR`
- Добавлена вспомогательная функция `writeError()` для единообразной отправки ошибок

### Обратная совместимость

- `AT+REG=<token>` — без изменений
- `AT+CONNECT=<token>` — без изменений (modem=nil, стандартные ответы OK/ERROR)
- `ATDT/ATDP` без параметра — без изменений (OK + ожидание следующей команды)
- `ATDT/ATDP` с параметром **без** предшествующих модемных команд — без изменений (OK + ожидание), НЕ трактуется как dial
- `ATDT/ATDP` с параметром **после** модемных команд (ATZ, ATE0 и т.д.) — **новое поведение**: трактуется как dial (CONNECT)

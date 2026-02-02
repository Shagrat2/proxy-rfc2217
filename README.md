# RFC-2217 NAT Proxy

TCP-прокси для эмуляции COM-порта по протоколу RFC-2217 через сеть, предназначенный для IoT-устройств за NAT.

## Архитектура

```
┌─────────────┐         ┌───────────────────────┐         ┌─────────────┐
│ IoT-устр-во │────────▶│    RFC-2217 Proxy     │◀────────│   Клиент    │
│ (за NAT)    │  :2217  │                       │  :2217  │ (ПК/Сервер) │
│ ESP8266/32  │         │  ┌─────────────────┐  │         │             │
└─────────────┘         │  │ Device Registry │  │         └─────────────┘
                        │  │ Session Manager │  │
                        │  │ HTTP API :8080  │  │
                        │  └─────────────────┘  │
                        └───────────────────────┘
```

## Порты

| Порт | Назначение |
|------|------------|
| 2217 | Подключения устройств и клиентов (единый порт) |
| 8080 | HTTP API и health-проверки |

## Быстрый старт

```bash
# Сборка и запуск локально
make run

# Или через Docker
make docker-build
docker run -p 2217:2217 -p 8080:8080 registry.jad.ru/proxy-rfc2217:latest
```

## Конфигурация

Переменные окружения:

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `PORT` | 2217 | Порт для подключений (устройства и клиенты) |
| `API_PORT` | 8080 | Порт для HTTP API и веб-интерфейса |
| `AUTH_TOKEN` | (пусто) | Токен аутентификации устройств |
| `WEB_USER` | admin | Логин для веб-интерфейса (Basic Auth) |
| `WEB_PASS` | admin | Пароль для веб-интерфейса (Basic Auth) |
| `KEEPALIVE` | 30 | TCP keepalive интервал в секундах |
| `INIT_TIMEOUT` | 5 | Таймаут ожидания AT-команды при подключении в секундах |

## Протокол

Используются AT-команды. Устройства и клиенты подключаются к одному порту, различаются по первой AT-команде.

### Регистрация устройства

Устройство подключается к порту и отправляет AT-команду регистрации:

```
AT+REG=<token>\r\n
```

Формат `<token>` зависит от настройки `AUTH_TOKEN`:

- **Без авторизации** (`AUTH_TOKEN` не задан): `<token>` = DEVICE_ID
  ```
  AT+REG=DEVICE_001\r\n
  ```

- **С авторизацией** (`AUTH_TOKEN` задан): `<token>` = `AUTH_TOKEN+DEVICE_ID`
  ```
  AT+REG=secret123+DEVICE_001\r\n
  ```

**Ответ:**
```
OK\r\n
```

или при ошибке:
```
ERROR\r\n
```

После регистрации соединение поддерживается через TCP keepalive (не требует дополнительных команд от устройства).

### Подключение клиента

Клиент подключается к тому же порту и отправляет:

```
AT+CONNECT=<token>\r\n
```

Формат `<token>` аналогичен регистрации устройства:

- **Без авторизации** (`AUTH_TOKEN` не задан): `<token>` = DEVICE_ID
  ```
  AT+CONNECT=DEVICE_001\r\n
  ```

- **С авторизацией** (`AUTH_TOKEN` задан): `<token>` = `AUTH_TOKEN+DEVICE_ID`
  ```
  AT+CONNECT=secret123+DEVICE_001\r\n
  ```

**Ответ:**
```
OK\r\n
```

или при ошибке:
```
ERROR\r\n
```

После получения `OK` соединение переходит в режим прозрачной передачи данных (RFC-2217 bridge).

## HTTP API

```
GET /                  # Веб-интерфейс (Basic Auth)
GET /healthz           # Проверка liveness
GET /readyz            # Проверка readiness
GET /api/v1/devices    # Список подключённых устройств
GET /api/v1/sessions   # Список активных сессий
GET /api/v1/stats      # Статистика
```

Веб-интерфейс доступен по адресу `http://localhost:8080/` и защищён Basic Auth (по умолчанию admin:admin).

### Примеры ответов

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

## Тестирование

### Эмуляция устройства (через netcat)

```bash
# Подключение к прокси как устройство
nc localhost 2217

# Без AUTH_TOKEN:
AT+REG=DEVICE_001
# Ответ: OK

# С AUTH_TOKEN=secret:
AT+REG=secret+DEVICE_001
# Ответ: OK

# Соединение остаётся открытым (TCP keepalive)
```

### Подключение клиента

```bash
# Подключение как клиент
nc localhost 2217

# Без AUTH_TOKEN:
AT+CONNECT=DEVICE_001
# Ответ: OK

# С AUTH_TOKEN=secret:
AT+CONNECT=secret+DEVICE_001
# Ответ: OK (и далее прозрачный канал к устройству)
```

### Тестовый скрипт устройства

```bash
#!/bin/bash
# test_device.sh - Простой эмулятор устройства

HOST=${1:-localhost}
PORT=${2:-2217}
DEVICE_ID=${3:-DEVICE_001}
AUTH_TOKEN=${4:-}  # Опционально

if [ -n "$AUTH_TOKEN" ]; then
  TOKEN="${AUTH_TOKEN}+${DEVICE_ID}"
else
  TOKEN="${DEVICE_ID}"
fi

{
  echo -e "AT+REG=${TOKEN}\r"
  cat  # Держим соединение открытым
} | nc $HOST $PORT
```

### Тестовый скрипт клиента

```bash
#!/bin/bash
# test_client.sh - Тестовый клиент

HOST=${1:-localhost}
PORT=${2:-2217}
DEVICE_ID=${3:-DEVICE_001}
AUTH_TOKEN=${4:-}  # Опционально

if [ -n "$AUTH_TOKEN" ]; then
  TOKEN="${AUTH_TOKEN}+${DEVICE_ID}"
else
  TOKEN="${DEVICE_ID}"
fi

{
  echo -e "AT+CONNECT=${TOKEN}\r"
  cat  # Прозрачная передача stdin
} | nc $HOST $PORT
```

## Развёртывание в Kubernetes

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/deployment.yaml
```

### Создание секретов

```bash
# Генерация случайных токенов
AUTH_TOKEN=$(openssl rand -hex 16)
WEB_PASS=$(openssl rand -hex 16)

# Создание секрета
kubectl -n waterius create secret generic proxy-rfc2217-secrets \
  --from-literal=auth-token=$AUTH_TOKEN \
  --from-literal=web-user=admin \
  --from-literal=web-pass=$WEB_PASS \
  --dry-run=client -o yaml | kubectl apply -f -

# Посмотреть сгенерированные значения
echo "AUTH_TOKEN: $AUTH_TOKEN"
echo "WEB_PASS: $WEB_PASS"
```

## Сборка

```bash
make build          # Сборка для Linux amd64
make build-local    # Сборка для текущей платформы
make docker-build   # Сборка Docker-образа
make docker-push    # Отправка в registry
make release        # Полный цикл
make test           # Запуск тестов
make lint           # Форматирование и проверка
```

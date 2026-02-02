# MikroTik + Ingress Nginx Setup

Настройка проброса порта 2217 с сохранением реального IP клиента через Ingress Nginx PROXY Protocol.

## Архитектура

```
Клиент → MikroTik (dst-nat + masquerade) → Ingress Nginx:2217 → RFC2217 Proxy
                                           (proxy_protocol)      (PROXY_PROTOCOL=true)
```

## Развёртывание K8s

### 1. RFC2217 Proxy

```bash
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/service.yaml
kubectl apply -f k8s/deployment.yaml
```

### 2. Ingress Nginx TCP ConfigMap

```bash
kubectl apply -f k8s/ingress-tcp.yaml
```

### 3. Включить TCP services в Ingress Nginx Controller

Добавить аргумент `--tcp-services-configmap`:

```bash
kubectl patch deployment ingress-nginx-controller -n ingress-nginx \
    --type='json' \
    -p='[{"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--tcp-services-configmap=$(POD_NAMESPACE)/tcp-services"}]'
```

### 4. Добавить порт в Ingress Nginx Controller

Отредактировать Deployment `ingress-nginx-controller`:

```bash
kubectl edit deployment ingress-nginx-controller -n ingress-nginx
```

Добавить в `spec.template.spec.containers[0].ports`:

```yaml
- containerPort: 2217
  name: rfc2217
  protocol: TCP
```

### 5. Добавить порт в Service Ingress Nginx

```bash
kubectl edit service ingress-nginx-controller -n ingress-nginx
```

Добавить в `spec.ports`:

```yaml
- name: rfc2217
  port: 2217
  targetPort: 2217
  protocol: TCP
```

## MikroTik NAT

K8s Node IP: `10.9.3.8`
Ingress NodePort для 2217: `32623`

```bash
# DST-NAT на Ingress Nginx NodePort
/ip firewall nat add chain=dstnat protocol=tcp dst-port=2217 \
    action=dst-nat to-addresses=10.9.3.8 to-ports=32623 \
    comment="RFC2217 proxy via Ingress"

# Masquerade
/ip firewall nat add chain=srcnat protocol=tcp \
    dst-address=10.9.3.8 dst-port=32623 \
    action=masquerade \
    comment="RFC2217 proxy - masquerade"
```

## Проверка

1. Проверить TCP ConfigMap:
```bash
kubectl get configmap tcp-services -n ingress-nginx -o yaml
```

2. Проверить что порт открыт на Ingress:
```bash
kubectl get svc ingress-nginx-controller -n ingress-nginx
```

3. Проверить логи RFC2217 Proxy - должен показывать реальный IP:
```bash
kubectl logs -n waterius -l app=proxy-rfc2217
```

```
[conn] 1.2.3.4:12345: received command: AT+REG param: "device-123"
```

## Troubleshooting

### Клиент не подключается

1. Проверить NAT правила: `/ip firewall nat print stats`
2. Проверить что порт 2217 есть в Service ingress-nginx-controller
3. Проверить логи Ingress: `kubectl logs -n ingress-nginx -l app.kubernetes.io/name=ingress-nginx`

### IP показывает 10.9.3.8 (K8s node)

1. Проверить что `::PROXY` в tcp-services ConfigMap
2. Проверить что `PROXY_PROTOCOL=true` в deployment RFC2217 Proxy
3. Перезапустить ingress-nginx-controller после изменения tcp-services

### Ошибка "connection refused"

1. Проверить что RFC2217 Proxy запущен: `kubectl get pods -n waterius`
2. Проверить Service: `kubectl get svc -n waterius`

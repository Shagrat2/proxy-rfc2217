BINARY_NAME=proxy-rfc2217
REGISTRY=registry.jad.ru
IMAGE_NAME=$(REGISTRY)/proxy-rfc2217

BUILD_DATE=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
GIT_COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS=-w -s -X main.BuildDate=$(BUILD_DATE) -X main.GitCommit=$(GIT_COMMIT)

.PHONY: build clean docker-build docker-push release test run deploy check-context release-deploy

build:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/proxy

build-local:
	go build -ldflags="$(LDFLAGS)" -o $(BINARY_NAME) ./cmd/proxy

run:
	go run -ldflags="$(LDFLAGS)" ./cmd/proxy

test:
	go test -v ./...

clean:
	rm -f $(BINARY_NAME)

docker-build: build
	docker build -t $(IMAGE_NAME):latest .

docker-push:
	docker push $(IMAGE_NAME):latest

release: build docker-build docker-push
	@echo "Release complete"

deploy: check-context
	kubectl rollout restart deployment/proxy-rfc2217 -n waterius
	kubectl rollout status deployment/proxy-rfc2217 -n waterius

check-context:
	@if [ "$$(kubectl config current-context)" != "home" ]; then \
		echo "Error: kubectl context is not 'home' (current: $$(kubectl config current-context))"; \
		echo "Run: kubectx home"; \
		exit 1; \
	fi

release-deploy: release deploy
	@echo "Release and deploy complete"

fmt:
	go fmt ./...

vet:
	go vet ./...

lint: fmt vet

.PHONY: build build-proxy build-defender test vet lint docker-build docker-push help

REGISTRY ?= ghcr.io/jlaska
VERSION  ?= dev

build: build-proxy build-defender

build-proxy:
	go build -ldflags="-s -w" -o bin/bbrokerd ./cmd/bbrokerd

build-defender:
	go build -ldflags="-s -w" -o bin/bbroker-defender ./cmd/bbroker-defender

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

docker-build:
	docker build -f Dockerfile.proxy -t $(REGISTRY)/bbroker-proxy:$(VERSION) .
	docker build -f Dockerfile.defender -t $(REGISTRY)/bbroker-defender:$(VERSION) .
	docker build -f deploy/xvfb/Dockerfile -t $(REGISTRY)/bbroker-xvfb:$(VERSION) deploy/xvfb/

docker-push:
	docker push $(REGISTRY)/bbroker-proxy:$(VERSION)
	docker push $(REGISTRY)/bbroker-defender:$(VERSION)
	docker push $(REGISTRY)/bbroker-xvfb:$(VERSION)

helm-lint:
	helm lint charts/bbroker

help:
	@echo "Targets: build build-proxy build-defender test vet lint docker-build docker-push helm-lint"

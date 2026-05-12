.PHONY: build build-proxy build-warden test vet lint docker-build docker-push docker-build-chrome docker-push-chrome help

REGISTRY ?= ghcr.io/jlaska
VERSION  ?= dev

build: build-proxy build-warden

build-proxy:
	go build -ldflags="-s -w" -o bin/bbrokerd ./cmd/bbrokerd

build-warden:
	go build -ldflags="-s -w" -o bin/bbroker-warden ./cmd/bbroker-warden

test:
	go test ./...

vet:
	go vet ./...

lint:
	golangci-lint run ./...

docker-build: docker-build-chrome
	docker build -f Dockerfile.proxy -t $(REGISTRY)/bbroker-proxy:$(VERSION) .
	docker build -f Dockerfile.warden -t $(REGISTRY)/bbroker-warden:$(VERSION) .
	docker build -f deploy/xvfb/Dockerfile -t $(REGISTRY)/bbroker-xvfb:$(VERSION) deploy/xvfb/

docker-build-chrome:
	docker build -f deploy/chrome/Dockerfile -t $(REGISTRY)/bbroker-chrome:$(VERSION) deploy/chrome/

docker-push: docker-push-chrome
	docker push $(REGISTRY)/bbroker-proxy:$(VERSION)
	docker push $(REGISTRY)/bbroker-warden:$(VERSION)
	docker push $(REGISTRY)/bbroker-xvfb:$(VERSION)

docker-push-chrome:
	docker push $(REGISTRY)/bbroker-chrome:$(VERSION)

helm-lint:
	helm lint charts/bbroker

help:
	@echo "Targets: build build-proxy build-warden test vet lint docker-build docker-push docker-build-chrome docker-push-chrome helm-lint"

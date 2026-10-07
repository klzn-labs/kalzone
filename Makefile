IMAGE := adrianozdp/platform-controlplane
VERSION := $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)

.PHONY: run test lint docker

run:
	go run ./cmd/controlplane

test:
	go test -p 1 -count=1 ./...

lint:
	go vet ./...
	test -z "$$(gofmt -l .)"

docker:
	docker build -t $(IMAGE):$(VERSION) .

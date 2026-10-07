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
	docker build --target controlplane -t $(IMAGE):$(VERSION) .
	docker build --target agent -t adrianozdp/platform-agent:$(VERSION) .

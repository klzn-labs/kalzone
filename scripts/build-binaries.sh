#!/bin/sh
set -eu
version="$1"
out="$2"
export CGO_ENABLED=0 GOOS=linux
go build -trimpath -ldflags "-s -w" -o "$out/controlplane" ./cmd/controlplane
go build -trimpath -ldflags "-s -w -X main.Version=$version" -o "$out/agent" ./cmd/agent
go build -trimpath -ldflags "-s -w -X main.Version=$version" -o "$out/queued" ./cmd/queued

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/controlplane ./cmd/controlplane && \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w -X main.Version=${VERSION}" -o /out/agent ./cmd/agent

FROM alpine:3.22 AS agent
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose
COPY --from=builder /out/agent /usr/local/bin/agent
ENTRYPOINT ["agent"]

FROM alpine:3.22 AS controlplane
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 platform
COPY --from=builder /out/controlplane /usr/local/bin/controlplane
USER platform
EXPOSE 8080
ENTRYPOINT ["controlplane"]

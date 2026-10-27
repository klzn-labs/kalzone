ARG BINARIES=builder

FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN sh scripts/build-binaries.sh "${VERSION}" /out

FROM scratch AS prebuilt
COPY out/ /out/

FROM ${BINARIES} AS binaries

FROM alpine:3.22 AS agent
RUN apk add --no-cache ca-certificates docker-cli docker-cli-compose
COPY --from=binaries /out/agent /usr/local/bin/agent
ENTRYPOINT ["agent"]

FROM alpine:3.22 AS queued
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10002 queued
COPY --from=binaries /out/queued /usr/local/bin/queued
USER queued
EXPOSE 8080 9090
ENTRYPOINT ["queued"]

FROM alpine:3.22 AS controlplane
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 platform
COPY --from=binaries /out/controlplane /usr/local/bin/controlplane
USER platform
EXPOSE 8080
ENTRYPOINT ["controlplane"]

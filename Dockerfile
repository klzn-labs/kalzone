FROM golang:1.26-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" -o /out/controlplane ./cmd/controlplane

FROM alpine:3.22
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 platform
COPY --from=builder /out/controlplane /usr/local/bin/controlplane
USER platform
EXPOSE 8080
ENTRYPOINT ["controlplane"]

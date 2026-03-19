# stage 1: build
FROM golang:1.26-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /otterly .

# stage 2: runtime
FROM alpine:3.21
RUN apk add --no-cache ffmpeg ca-certificates
RUN adduser -D -h /data -u 1000 otterly
COPY --from=builder /otterly /otterly
USER otterly
ENTRYPOINT ["/otterly"]

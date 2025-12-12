# Stage 1: Compilação do Go
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY main.go .

RUN go build -o connection-monitor main.go

FROM alpine:latest

RUN apk --no-cache add iproute2

WORKDIR /root/

COPY --from=builder /app/connection-monitor .

ENTRYPOINT ["./connection-monitor"]

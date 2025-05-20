# Этап сборки
FROM golang:1.21-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/notification_bot ./cmd/bot/main.go

# Этап выполнения
FROM alpine:latest
WORKDIR /root/

COPY --from=builder /app/notification_bot .
# COPY config.yml . # Раскомментировать при необходимости
# COPY migrations ./migrations # Раскомментировать при необходимости
EXPOSE 8080

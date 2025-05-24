# Stage 1: Build the Go application
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY go.mod go.sum ./
# Download dependencies
RUN go mod download

# Copy the source code
COPY . .

# Build the application
# Adjust the output path if your main.go is elsewhere or named differently
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o /app/teacher_bot_server ./cmd/bot/main.go

# Stage 2: Create a minimal production image
FROM alpine:latest

WORKDIR /root/

# Copy the pre-built binary from the builder stage
COPY --from=builder /app/teacher_bot_server .

# Command to run the executable
CMD ["./teacher_bot_server"] 
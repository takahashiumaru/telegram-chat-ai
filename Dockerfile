# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Copy go mod and sum files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-chat-ai main.go

# Final stage
FROM alpine:latest

WORKDIR /app

# Install ca-certificates for secure requests
RUN apk add --no-cache ca-certificates

# Copy binary from builder
COPY --from=builder /app/telegram-chat-ai .

# The app uses a state.json file, we should probably ensure it's handled or at least exists
# but the app creates it if missing in loadState()

CMD ["./telegram-chat-ai"]

# ==========================================
# BUILD STAGE
# ==========================================
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install git dan sertifikat keamanan
RUN apk add --no-cache git ca-certificates

# Copy go.mod dan go.sum terlebih dahulu untuk caching dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy seluruh source code
COPY . .

# Build aplikasi menjadi binary tunggal (tanpa dependensi C)
RUN CGO_ENABLED=0 GOOS=linux go build -o telegram-chat-ai main.go


# ==========================================
# FINAL STAGE (Runner)
# ==========================================
FROM alpine:latest

WORKDIR /app

# Install sertifikat keamanan dan tzdata untuk sinkronisasi zona waktu
RUN apk add --no-cache ca-certificates tzdata

# Ambil file binary dari tahap build sebelumnya
COPY --from=builder /app/telegram-chat-ai .

# Eksekusi aplikasi
CMD ["./telegram-chat-ai"]
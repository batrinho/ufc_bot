# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# If you have go.mod/go.sum, keep this
COPY go.mod go.sum ./
RUN go mod download

# Copy all source
COPY . .

# Build binary
RUN go build -o ufc_bot .

# Run stage
FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/ufc_bot /app/ufc_bot

# DB dir if you use SQLite
RUN mkdir -p /data

# Declare env vars (no secrets here)
ENV TELEGRAM_BOT_TOKEN=""
ENV UFC_BOT_DB_PATH="/data/ufc_bot.db"

CMD ["/app/ufc_bot"]

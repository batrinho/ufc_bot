# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Need C toolchain + sqlite headers for go-sqlite3
RUN apk add --no-cache build-base sqlite-dev

# Enable CGO explicitly
ENV CGO_ENABLED=1

# Go modules
COPY go.mod go.sum ./
RUN go mod download

# App source
COPY . .

# Build binary
RUN go build -o ufc_bot .

# Run stage
FROM alpine:3.19

WORKDIR /app

# Runtime deps: certs, tz, sqlite libs
RUN apk add --no-cache ca-certificates tzdata sqlite-libs

# Copy binary
COPY --from=builder /app/ufc_bot /app/ufc_bot

# SQLite will create ufc.db in /app
CMD ["/app/ufc_bot"]

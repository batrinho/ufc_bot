FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN go build -o ufc_bot .

FROM alpine:3.19

WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata

COPY --from=builder /app/ufc_bot /app/ufc_bot

# SQLite will create /app/ufc.db automatically
CMD ["/app/ufc_bot"]

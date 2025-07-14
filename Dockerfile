# syntax=docker/dockerfile:1

FROM golang:1.21

ENV CGO_ENABLED=1
ENV GOOS=linux

RUN apt-get update && apt-get install -y gcc sqlite3 libsqlite3-dev

WORKDIR /app

COPY go.mod ./
COPY go.sum ./
RUN go mod download

COPY . .

RUN go build -o bot .

CMD ["./bot"]

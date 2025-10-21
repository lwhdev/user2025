# syntax=docker/dockerfile:1

FROM golang:1.22 AS build

WORKDIR /app
COPY go.mod ./
COPY internal ./internal
COPY cmd ./cmd
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o server ./cmd/server

FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends ca-certificates default-mysql-client && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=build /app/server ./server

ENV SERVER_PORT=8080 \
    DB_HOST=mysql \
    DB_PORT=3306 \
    DB_USER=app \
    DB_PASSWORD=app_password \
    DB_NAME=app_db \
    JWT_SECRET=change-me

EXPOSE 8080

CMD ["/app/server"]

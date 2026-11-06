FROM golang:1.25.5-alpine3.21 AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tg-bot-proxy ./cmd

FROM alpine:3.21

RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -S app \
    && adduser -S -G app app

WORKDIR /app

COPY --from=builder --chown=app:app /out/tg-bot-proxy ./tg-bot-proxy

USER app

EXPOSE 5050 8080 9090

ENTRYPOINT ["./tg-bot-proxy"]

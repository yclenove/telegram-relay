FROM golang:1.22-alpine AS builder
WORKDIR /app

COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o relay ./cmd/relay

FROM alpine:3.20
WORKDIR /app
RUN adduser -D -g '' relay
COPY --from=builder /app/relay /app/relay
COPY --from=builder /app/configs /app/configs
COPY --from=builder /app/migrations /app/migrations
COPY --from=builder /app/web /app/web
USER relay
EXPOSE 8080
ENTRYPOINT ["/app/relay"]

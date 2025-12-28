# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Install dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build bot binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bot ./cmd/bot

# Build worker binary
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /worker ./cmd/worker

# Final stage - Bot
FROM alpine:3.19 AS bot

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /bot /app/bot
COPY --from=builder /app/migrations /app/migrations

ENV TZ=Asia/Almaty

EXPOSE 8080

CMD ["/app/bot"]

# Final stage - Worker
FROM alpine:3.19 AS worker

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /worker /app/worker
COPY --from=builder /app/migrations /app/migrations

ENV TZ=Asia/Almaty

CMD ["/app/worker"]

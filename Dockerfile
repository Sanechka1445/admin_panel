FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /app/server ./cmd/main.go

# ---- Runtime ----
FROM mariadb:11 AS runtime

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates mysql-client && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/server .
COPY internal/templates ./internal/templates

RUN mkdir -p dbBackup

EXPOSE 8080
CMD ["/app/server"]

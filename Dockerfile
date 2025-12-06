# Build stage
FROM golang:1.21-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -o gotunnel-server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -o gotunnel ./cmd/client

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/gotunnel-server .
COPY --from=builder /app/gotunnel .

# Create tokens file with default dev token
RUN echo "dev-token:dev-user" > /app/tokens.txt

EXPOSE 9000 8080

CMD ["./gotunnel-server", "--domain", "gotunnel.fly.dev", "--control-addr", ":9000", "--http-addr", ":8080", "--log-level", "info"]

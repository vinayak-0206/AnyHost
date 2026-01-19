# Frontend build stage
FROM node:18-alpine AS frontend

WORKDIR /app

# Copy package files
COPY package*.json ./

# Install dependencies
RUN npm ci --ignore-scripts 2>/dev/null || npm install --ignore-scripts 2>/dev/null || echo "No package.json found, skipping frontend build"

# Copy frontend source
COPY src/ ./src/
COPY index.html vite.config.js tailwind.config.js postcss.config.js ./

# Build frontend (if package.json exists)
RUN if [ -f package.json ]; then npm run build; else mkdir -p dist; fi

# Backend build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install build dependencies for SQLite
RUN apk add --no-cache gcc musl-dev

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build unified server (single-port, recommended) and client
# CGO enabled for SQLite support
RUN CGO_ENABLED=1 GOOS=linux go build -o gotunnel-server ./cmd/unified
RUN CGO_ENABLED=0 GOOS=linux go build -o gotunnel ./cmd/client

# Runtime stage
FROM alpine:3.19

RUN apk --no-cache add ca-certificates sqlite

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /app/gotunnel-server .
COPY --from=builder /app/gotunnel .

# Copy frontend build
COPY --from=frontend /app/dist ./dist

# Create data directory for database
RUN mkdir -p /data

# Create tokens file with default dev token
RUN echo "dev-token:dev-user" > /app/tokens.txt

# Environment variables
ENV PORT=8080
ENV DATABASE_PATH=/data/gotunnel.db
ENV LOG_LEVEL=info

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

CMD ["./gotunnel-server", "--port", "8080", "--log-level", "info"]

# GoTunnel - Technical Architecture Document

## Executive Summary

GoTunnel is a self-hosted, multiplexed tunneling service that exposes local development servers to the internet through secure tunnels. Unlike traditional solutions that create one connection per tunnel, GoTunnel multiplexes multiple tunnels over a single TCP connection using the yamux protocol.

---

## Problem Statement

Developers frequently need to expose local services to the internet for:
- Webhook testing (Stripe, GitHub, Twilio)
- Mobile app development against local APIs
- Sharing work-in-progress with clients
- QA testing on real devices

**Existing solutions (ngrok, localtunnel) have limitations:**
- Per-tunnel connections (inefficient)
- Random URLs on free tiers
- Limited customization
- No self-hosting option
- Expensive for teams

**GoTunnel solves these by providing:**
- Single connection, multiple tunnels (multiplexing)
- Permanent, predictable subdomains
- Full self-hosting capability
- Open protocol for extensibility

---

## System Architecture

```
                                    INTERNET
                                        |
                    +-------------------+-------------------+
                    |           PUBLIC SERVER              |
                    |                                       |
                    |   +-------------+   +-------------+   |
                    |   | HTTP Proxy  |   | Control     |   |
                    |   | :80/:443    |   | Plane :9000 |   |
                    |   +------+------+   +------+------+   |
                    |          |                 |          |
                    |          v                 v          |
                    |   +-----------------------------+     |
                    |   |         REGISTRY            |     |
                    |   | subdomain -> session map    |     |
                    |   +-----------------------------+     |
                    |                 |                     |
                    +-----------------+---------------------+
                                      |
                              yamux session
                           (multiplexed TCP)
                                      |
                    +-----------------+---------------------+
                    |           LOCAL CLIENT               |
                    |                                       |
                    |   +-------------+   +-------------+   |
                    |   |   Router    |   | Conn Pool   |   |
                    |   +------+------+   +------+------+   |
                    |          |                 |          |
                    |          v                 v          |
                    |   +-------------+   +-------------+   |
                    |   | localhost   |   | localhost   |   |
                    |   | :3000       |   | :8080       |   |
                    |   +-------------+   +-------------+   |
                    +---------------------------------------+
```

---

## Core Components

### 1. Protocol Layer (`internal/protocol/`)

The wire protocol defines how server and client communicate.

#### Message Envelope
All control messages are wrapped in an envelope:
```go
type Envelope struct {
    Type      MessageType     `json:"type"`
    Timestamp time.Time       `json:"timestamp"`
    RequestID string          `json:"request_id,omitempty"`
    Payload   json.RawMessage `json:"payload"`
}
```

#### Wire Format
```
+----------------+------------------+
| Length (4B BE) | JSON Payload     |
+----------------+------------------+
```
- Length: 4-byte big-endian uint32
- Payload: JSON-encoded Envelope
- Max message size: 64KB

#### Handshake Flow
```
CLIENT                                     SERVER
   |                                          |
   |-------- [TCP Connect] ------------------>|
   |                                          |
   |<------- [yamux session established] ---->|
   |                                          |
   |-------- [Open Stream 0] ---------------->|
   |                                          |
   |-------- HandshakeRequest --------------->|
   |         {                                |
   |           version: 1,                    |
   |           token: "xxx",                  |
   |           tunnels: [                     |
   |             {subdomain: "api", port: 3000}|
   |           ]                              |
   |         }                                |
   |                                          |
   |<------- HandshakeResponse ---------------|
   |         {                                |
   |           success: true,                 |
   |           session_id: "sess_xxx",        |
   |           tunnels: [                     |
   |             {subdomain: "api",           |
   |              url: "http://api.example.com"}|
   |           ]                              |
   |         }                                |
   |                                          |
   |-------- [Close Stream 0] --------------->|
   |                                          |
   |         SESSION ESTABLISHED              |
   |                                          |
```

#### Stream Header
When the server opens a stream to forward a request:
```go
type StreamHeader struct {
    Type      StreamType `json:"type"`       // "http", "tcp", "websocket"
    LocalPort int        `json:"local_port"`
    LocalHost string     `json:"local_host"`
    RequestID string     `json:"request_id"`
    Subdomain string     `json:"subdomain"`
}
```

### 2. Server Components (`internal/server/`)

#### Control Plane (`control.go`)
- Listens on port 9000 (configurable)
- Accepts client TCP connections
- Establishes yamux sessions
- Handles handshake and authentication
- Manages session lifecycle

#### Registry (`registry.go`)
- Thread-safe subdomain -> session mapping
- Validates subdomain format (3-63 chars, alphanumeric + hyphen)
- Enforces reserved subdomain list
- Supports session lookup by Host header

#### HTTP Proxy (`proxy.go`)
- Listens on port 80/443
- Extracts subdomain from Host header
- Looks up session in registry
- Opens yamux stream to client
- Writes stream header + HTTP request
- Reads response and forwards to browser
- WebSocket upgrade support via connection hijacking

#### Session (`session.go`)
- Represents a connected client
- Wraps yamux.Session for stream multiplexing
- Tracks registered tunnels
- Collects metrics (streams, bytes, requests)
- Handles graceful shutdown

#### Authentication (`auth.go`)
- Token-based authentication
- Constant-time comparison (timing attack prevention)
- Extensible interface for JWT/OAuth

### 3. Client Components (`internal/client/`)

#### Tunnel (`tunnel.go`)
- Establishes connection to server
- Creates yamux client session
- Performs handshake
- Accepts incoming streams from server
- Delegates to Router for forwarding

#### Router (`router.go`)
- Routes streams to local services
- Manages connection pools per local port
- Bidirectional data copying

#### Connection Pool (`pool.go`)
- Reuses connections to local services
- Configurable limits (idle, max open, lifetime)
- Automatic cleanup of stale connections
- Health checking

#### Reconnector (`reconnect.go`)
- Exponential backoff with jitter
- Configurable delays and max attempts
- Automatic reset on successful connection

---

## Request Flow (HTTP)

```
1. Browser requests: GET http://api.example.com/users

2. Server HTTP Proxy:
   - Extracts "api" from Host header
   - Looks up Registry: api -> Session{client_id: "abc"}
   - Opens yamux stream to client
   - Writes StreamHeader{type: "http", port: 3000, request_id: "req_123"}
   - Writes raw HTTP request bytes

3. Client Router:
   - Accepts stream
   - Reads StreamHeader
   - Gets connection from pool for localhost:3000
   - Pipes: stream <-> local connection

4. Local Service:
   - Receives HTTP request
   - Sends HTTP response

5. Response flows back:
   - Local -> Client -> yamux stream -> Server -> Browser
```

---

## Multiplexing Deep Dive

### Why Yamux?

Yamux (Yet Another Multiplexer) provides:
- Multiple logical streams over one TCP connection
- Flow control per stream
- Keepalive/heartbeat
- Backpressure handling

### Stream Lifecycle

```
SERVER                              CLIENT
   |                                   |
   |-- yamux.OpenStream() ------------>|
   |                                   |
   |-- WriteStreamHeader() ----------->|
   |                                   |
   |-- Write(HTTP Request) ----------->|
   |                                   |
   |<-- Read(HTTP Response) -----------|
   |                                   |
   |-- stream.Close() ---------------->|
   |                                   |
```

Each HTTP request = 1 yamux stream
Streams are lightweight (~100 bytes overhead)
Thousands of concurrent streams supported

---

## Configuration

### Server Configuration
```yaml
control_addr: ":9000"          # Client connection port
http_addr: ":80"               # Public HTTP port
https_addr: ":443"             # Public HTTPS port
domain: "tunnel.example.com"   # Base domain

tls:
  enabled: true
  cert_file: "/path/to/cert.pem"
  key_file: "/path/to/key.pem"

auth:
  mode: "token"                # token | jwt | none
  token_file: "/path/to/tokens"

limits:
  max_connections_per_user: 5
  max_tunnels_per_connection: 10
  max_requests_per_minute: 1000

timeouts:
  handshake_timeout: 10s
  idle_timeout: 5m
  request_timeout: 30s

reserved_subdomains:
  - www
  - api
  - admin
```

### Client Configuration
```yaml
server_addr: "tunnel.example.com:9000"
token: "your-secret-token"

tunnels:
  - subdomain: "myapp"
    local_port: 3000
  - subdomain: "dashboard"
    local_port: 8080

reconnect:
  enabled: true
  initial_delay: 1s
  max_delay: 30s
  multiplier: 2.0
```

---

## Security Considerations

### Authentication
- Tokens validated with constant-time comparison
- Session bound to authenticated token
- Extensible to JWT for stateless auth

### Network Security
- Control plane can be TLS-encrypted
- HTTP proxy terminates TLS (standard reverse proxy pattern)
- Client -> Server connection can traverse firewalls (outbound only)

### Subdomain Protection
- Reserved list prevents claiming system subdomains
- Validation regex: `^[a-z][a-z0-9-]{2,62}$`
- Future: User-specific subdomain namespaces

### Rate Limiting (Planned)
- Per-subdomain request limits
- Per-user bandwidth quotas
- Connection limits

---

## Deployment Architecture

### Single Server
```
                    [DNS: *.tunnel.example.com -> Server IP]
                                    |
                            +-------+-------+
                            |   GoTunnel   |
                            |   Server     |
                            | :80 :443 :9000|
                            +---------------+
```

### High Availability (Future)
```
                            [Load Balancer]
                                   |
                    +--------------+--------------+
                    |              |              |
              +---------+    +---------+    +---------+
              | Server1 |    | Server2 |    | Server3 |
              +---------+    +---------+    +---------+
                    |              |              |
                    +--------------+--------------+
                                   |
                              [Redis]
                         (shared registry)
```

---

## Metrics & Observability

### Available Metrics
```go
type SessionMetrics struct {
    StreamsOpened   int64  // Total streams opened
    StreamsClosed   int64  // Total streams closed
    BytesSent       int64  // Bytes sent to client
    BytesReceived   int64  // Bytes received from client
    RequestsHandled int64  // HTTP requests proxied
    Errors          int64  // Error count
}
```

### Prometheus Integration (Planned)
```
gotunnel_active_sessions{}
gotunnel_active_tunnels{}
gotunnel_requests_total{subdomain="api"}
gotunnel_bytes_transferred{direction="in|out"}
gotunnel_request_duration_seconds{subdomain="api"}
```

---

## Project Structure

```
gotunnel/
├── cmd/
│   ├── server/main.go        # Server entry point
│   └── client/main.go        # Client entry point
├── internal/
│   ├── protocol/             # Wire protocol
│   │   ├── version.go        # Protocol versioning
│   │   ├── messages.go       # Message types
│   │   ├── stream.go         # Stream headers
│   │   ├── codec.go          # Encoding/decoding
│   │   └── errors.go         # Protocol errors
│   ├── server/
│   │   ├── server.go         # Main orchestrator
│   │   ├── control.go        # Control plane
│   │   ├── proxy.go          # HTTP proxy
│   │   ├── registry.go       # Subdomain registry
│   │   ├── session.go        # Client sessions
│   │   └── auth.go           # Authentication
│   ├── client/
│   │   ├── tunnel.go         # Tunnel client
│   │   ├── router.go         # Stream routing
│   │   ├── pool.go           # Connection pooling
│   │   └── reconnect.go      # Reconnection logic
│   └── common/
│       ├── config.go         # Configuration
│       └── id.go             # ID generation
├── configs/
│   ├── server.example.yaml
│   └── client.example.yaml
└── docs/
    └── ARCHITECTURE.md       # This document
```

---

## Technology Stack

| Component | Technology | Purpose |
|-----------|------------|---------|
| Language | Go 1.21+ | Performance, concurrency |
| Multiplexing | hashicorp/yamux | Stream multiplexing |
| CLI | spf13/cobra | Command-line interface |
| Config | gopkg.in/yaml.v3 | YAML configuration |
| Logging | log/slog (stdlib) | Structured logging |

---

## Roadmap

### Phase 1 (Complete)
- [x] Core protocol implementation
- [x] Server: control plane, HTTP proxy, registry
- [x] Client: tunnel, router, connection pooling
- [x] Reconnection with exponential backoff
- [x] Token authentication
- [x] YAML configuration

### Phase 2 (Planned)
- [ ] TLS/HTTPS support with auto-cert (Let's Encrypt)
- [ ] TCP tunnel support (databases, SSH)
- [ ] Traffic inspection dashboard
- [ ] Prometheus metrics
- [ ] Request/response logging

### Phase 3 (Future)
- [ ] Multi-user with database backend
- [ ] Custom domain support (CNAME)
- [ ] Rate limiting and quotas
- [ ] High availability (Redis registry)
- [ ] Web management UI
- [ ] Webhook replay

---

## Quick Start

### Build
```bash
go build -o bin/gotunnel-server ./cmd/server
go build -o bin/gotunnel ./cmd/client
```

### Run Server
```bash
./bin/gotunnel-server --domain localhost --control-addr :9001 --http-addr :8081
```

### Run Client
```bash
./bin/gotunnel --server localhost:9001 --token dev-token --subdomain myapp --port 3000
```

### Test
```bash
curl -H "Host: myapp.localhost" http://localhost:8081/
```

---

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit a pull request

---

## License

MIT License - See LICENSE file for details.

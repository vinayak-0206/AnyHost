# GoTunnel

A self-hosted, multiplexed tunneling service. Expose local services to the internet with permanent subdomains over a single TCP connection.

## Why GoTunnel?

| Feature | ngrok (free) | GoTunnel |
|---------|--------------|----------|
| Self-hosted | No | Yes |
| Permanent URLs | No | Yes |
| Multiple tunnels | 1 | Unlimited |
| Connection per tunnel | 1 | Shared (multiplexed) |
| Custom domain | Paid | Yes |
| Open source | No | Yes |

## How It Works

```
Browser                Server                    Client                  Local
   |                      |                         |                      |
   |-- GET api.example.com -->|                     |                      |
   |                      |-- [yamux stream] ------>|                      |
   |                      |                         |-- forward to :3000 ->|
   |                      |                         |<---- response -------|
   |                      |<-----------------------|                      |
   |<---- response -------|                         |                      |
```

**Key insight:** Multiple tunnels share ONE TCP connection using yamux multiplexing.

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
# Start a local server
python3 -m http.server 3000

# Access through tunnel
curl -H "Host: myapp.localhost" http://localhost:8081/
```

## Multiple Tunnels

Create `tunnel.yaml`:
```yaml
server_addr: "localhost:9001"
token: "dev-token"
tunnels:
  - subdomain: "frontend"
    local_port: 3000
  - subdomain: "backend"
    local_port: 8080
  - subdomain: "docs"
    local_port: 4000
```

Run:
```bash
./bin/gotunnel --config tunnel.yaml
```

All three tunnels share a single TCP connection.

## Production Deployment

### 1. Server Setup (VPS)
```bash
./gotunnel-server \
  --domain tunnel.yourdomain.com \
  --control-addr :9000 \
  --http-addr :80
```

### 2. DNS Configuration
Add wildcard A record:
```
*.tunnel.yourdomain.com  ->  YOUR_SERVER_IP
```

### 3. Client Connection
```bash
./gotunnel \
  --server tunnel.yourdomain.com:9000 \
  --token your-secret-token \
  --subdomain myapp \
  --port 3000
```

Access at: `http://myapp.tunnel.yourdomain.com`

## Architecture

```
gotunnel/
├── cmd/
│   ├── server/          # Server binary
│   └── client/          # Client binary
├── internal/
│   ├── protocol/        # Wire protocol (handshake, messages)
│   ├── server/          # Control plane, HTTP proxy, registry
│   ├── client/          # Tunnel, router, connection pool
│   └── common/          # Shared config, utilities
├── configs/             # Example configurations
└── docs/                # Documentation
```

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) for deep technical details.

## Features

- **Multiplexed tunnels** - Multiple services over one connection
- **Subdomain routing** - `app.domain.com`, `api.domain.com`
- **Auto-reconnect** - Exponential backoff on disconnect
- **Connection pooling** - Efficient local service connections
- **WebSocket support** - Full duplex proxying
- **Token auth** - Simple, secure authentication
- **Reserved subdomains** - Protect system names

## Configuration Reference

### Server
| Flag | Default | Description |
|------|---------|-------------|
| `--domain` | localhost | Base domain for subdomains |
| `--control-addr` | :9000 | Client connection address |
| `--http-addr` | :8080 | Public HTTP address |
| `--config` | - | YAML config file path |
| `--log-level` | info | debug, info, warn, error |

### Client
| Flag | Default | Description |
|------|---------|-------------|
| `--server` | localhost:9000 | Server address |
| `--token` | - | Authentication token |
| `--subdomain` | - | Requested subdomain |
| `--port` | - | Local port to expose |
| `--config` | - | YAML config file path |

## Roadmap

- [ ] HTTPS with auto-cert (Let's Encrypt)
- [ ] TCP tunnels (databases, SSH)
- [ ] Traffic inspection UI
- [ ] Prometheus metrics
- [ ] Multi-user support
- [ ] Custom domains (CNAME)
- [ ] Rate limiting

## Tech Stack

- **Go** - Core language
- **yamux** - Stream multiplexing (HashiCorp)
- **cobra** - CLI framework

## License

MIT

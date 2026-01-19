# AnyHost

**The self-hosted tunnel for teams that care about security.**

Expose local services to the internet through secure tunnels. Like ngrok, but you own the infrastructure.

```
Your traffic stays on YOUR servers. HIPAA, SOC2, GDPR compliant by design.
```

## Why AnyHost?

| Feature | ngrok Free | ngrok Paid | AnyHost |
|---------|-----------|------------|---------|
| Self-hosted | No | No | **Yes** |
| Data sovereignty | No | No | **Yes** |
| Custom domains | No | $20/mo | **Yes** |
| Unlimited tunnels | No | Paid | **Yes** |
| Team management | No | $8/user/mo | **Yes** |
| Request inspection | Limited | Paid | **Yes** |
| Open source | No | No | **Yes** |

**Who is this for?**
- Teams blocked by corporate firewalls that ban ngrok
- Companies in regulated industries (healthcare, finance, government)
- Organizations that can't send traffic through third-party servers
- Developers who want unlimited tunnels without per-seat pricing

## Quick Start

### 1. Start the Server

```bash
# Using Docker (recommended)
docker run -p 8080:8080 -e DOMAIN=tunnel.yourdomain.com anyhost/gotunnel

# Or build from source
go build -o gotunnel-server ./cmd/unified
./gotunnel-server --domain tunnel.yourdomain.com --port 8080
```

### 2. Connect a Client

```bash
# Build the client
go build -o gotunnel ./cmd/client

# Expose local port 3000 as myapp.tunnel.yourdomain.com
./gotunnel --server ws://tunnel.yourdomain.com:8080/tunnel \
           --token dev-token \
           --subdomain myapp \
           --port 3000
```

### 3. Access Your Service

```
https://myapp.tunnel.yourdomain.com → localhost:3000
```

## Architecture

```
┌─────────────────────────────────────┐
│      Your Machine (Client)          │
│                                     │
│  gotunnel client                    │
│  └─ Connects via WebSocket          │
│  └─ Exposes localhost:3000          │
└────────────┬────────────────────────┘
             │
             │ WebSocket (encrypted)
             │
┌────────────▼────────────────────────┐
│      Your Server (AnyHost)          │
│                                     │
│  ┌─────────────────────────────┐    │
│  │ Subdomain Router            │    │
│  │ myapp.domain.com → client   │    │
│  └─────────────────────────────┘    │
│                                     │
│  ┌─────────────────────────────┐    │
│  │ Dashboard (React)           │    │
│  │ Manage tunnels & users      │    │
│  └─────────────────────────────┘    │
│                                     │
│  ┌─────────────────────────────┐    │
│  │ SQLite Database             │    │
│  │ Users, tokens, subdomains   │    │
│  └─────────────────────────────┘    │
└────────────┬────────────────────────┘
             │
             │ HTTPS
             │
┌────────────▼────────────────────────┐
│      Internet Users                 │
│      https://myapp.domain.com       │
└─────────────────────────────────────┘
```

## Installation

### Pre-built Binaries

```bash
# macOS
curl -sSL https://github.com/anyhost/gotunnel/releases/latest/download/gotunnel-darwin-amd64 -o gotunnel
chmod +x gotunnel

# Linux
curl -sSL https://github.com/anyhost/gotunnel/releases/latest/download/gotunnel-linux-amd64 -o gotunnel
chmod +x gotunnel
```

### From Source

```bash
git clone https://github.com/anyhost/gotunnel.git
cd gotunnel

# Build server
go build -o gotunnel-server ./cmd/unified

# Build client
go build -o gotunnel ./cmd/client
```

### Docker

```bash
# Server
docker pull anyhost/gotunnel:latest
docker run -p 8080:8080 -e DOMAIN=tunnel.example.com anyhost/gotunnel

# With persistent data
docker run -p 8080:8080 \
  -v $(pwd)/data:/app/data \
  -e DOMAIN=tunnel.example.com \
  -e DATABASE_PATH=/app/data/gotunnel.db \
  anyhost/gotunnel
```

## Configuration

### Server Configuration

Create `server.yaml`:

```yaml
# Base domain for subdomain routing
domain: "tunnel.example.com"

# Single port for both control plane and HTTP proxy
http_addr: ":8080"

# Authentication
auth:
  mode: "token"  # "token", "jwt", or "none"
  token_file: "./tokens.txt"  # format: token:userID

# TLS (recommended for production)
tls:
  enabled: true
  cert_file: "/path/to/cert.pem"
  key_file: "/path/to/key.pem"

# Resource limits
limits:
  max_connections_per_user: 5
  max_tunnels_per_connection: 10
  max_requests_per_minute: 1000

# Reserved subdomains
reserved_subdomains:
  - www
  - api
  - admin
```

Run with config:
```bash
./gotunnel-server --config server.yaml
```

### Client Configuration

Create `tunnel.yaml`:

```yaml
server_addr: "ws://tunnel.example.com:8080/tunnel"
token: "your-secret-token"

# Multiple tunnels over single connection
tunnels:
  - subdomain: "api"
    local_port: 3000
  - subdomain: "web"
    local_port: 8080
  - subdomain: "docs"
    local_port: 4000

# Auto-reconnect on disconnection
reconnect:
  enabled: true
  max_delay: 30s
```

Run with config:
```bash
./gotunnel --config tunnel.yaml
```

### Environment Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `DOMAIN` | Base domain for subdomains | `localhost` |
| `PORT` | Server listening port | `8080` |
| `DATABASE_PATH` | Path to SQLite database | `./gotunnel.db` |
| `LOG_LEVEL` | Logging verbosity | `info` |

## Deployment

### Deployment Options

| Method | Cost | Complexity | Best For |
|--------|------|------------|----------|
| [Oracle Cloud](#oracle-cloud-free) | **$0/mo** | Medium | Free forever |
| [Fly.io](#flyio) | **$0/mo** | Easy | Quick start |
| [Docker Compose](#docker-compose) | $4-6/mo | Easy | Any VPS |
| [Kubernetes](#kubernetes-helm) | Variable | Complex | Enterprise |
| [AWS Terraform](#terraform-aws) | ~$35/mo | Medium | Production |

### Oracle Cloud (FREE)

Deploy on Oracle Cloud's Always Free tier - **$0/month forever**:

```bash
cd deploy/terraform/oracle-cloud
cp terraform.tfvars.example terraform.tfvars
# Edit terraform.tfvars with your values
terraform init && terraform apply
```

See [deploy/terraform/oracle-cloud](./deploy/terraform/oracle-cloud) for details.

### Fly.io

Deploy with Fly.io's free tier (3 VMs, 160GB transfer):

```bash
cd deploy/fly
fly launch --copy-config --no-deploy
fly volumes create anyhost_data --size 1
fly secrets set DOMAIN=tunnel.example.com
fly deploy
```

See [deploy/fly](./deploy/fly) for details.

### Docker Compose

Deploy on any $4-6/month VPS (Hetzner, DigitalOcean, Vultr):

```bash
cd deploy/docker-compose
export DOMAIN=tunnel.example.com
echo "mytoken:myuser" > tokens.txt
docker compose up -d
```

See [deploy/docker-compose](./deploy/docker-compose) for details.

### Kubernetes (Helm)

```bash
helm repo add anyhost https://charts.anyhost.dev
helm install anyhost anyhost/gotunnel \
  --set domain=tunnel.example.com \
  --set ingress.enabled=true
```

See [deploy/helm](./deploy/helm) for full chart documentation.

### Terraform (AWS)

For production AWS deployment with ALB, ECS Fargate, and EFS:

```hcl
module "anyhost" {
  source = "github.com/anyhost/gotunnel//deploy/terraform/aws"

  domain             = "tunnel.example.com"
  vpc_id             = var.vpc_id
  public_subnet_ids  = var.public_subnets
  private_subnet_ids = var.private_subnets
  certificate_arn    = var.acm_cert_arn
}
```

See [deploy/terraform/aws](./deploy/terraform/aws) for details.

## Web Dashboard

AnyHost includes a web dashboard for managing tunnels:

```
http://localhost:8080/
```

Features:
- User registration and login
- Reserve subdomains
- View active tunnels
- Copy CLI commands
- Request inspection (coming soon)

## API Reference

### Authentication

All API requests require a Bearer token:

```bash
curl -H "Authorization: Bearer your-token" \
  https://tunnel.example.com/api/tunnels
```

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| POST | `/api/auth/register` | Create new user |
| POST | `/api/auth/login` | Get auth token |
| GET | `/api/tunnels` | List user's tunnels |
| POST | `/api/tunnels` | Reserve subdomain |
| DELETE | `/api/tunnels/:subdomain` | Release subdomain |
| GET | `/api/requests/:subdomain` | Get request logs |

## Security

### Best Practices

1. **Always use TLS in production** - Traffic contains sensitive data
2. **Rotate tokens regularly** - Tokens provide full tunnel access
3. **Use reserved subdomains** - Prevent users from claiming sensitive names
4. **Set rate limits** - Protect against abuse
5. **Run behind a reverse proxy** - nginx/Caddy for additional security

### Compliance

AnyHost is designed for self-hosting, giving you full control over:
- Where data is stored
- Who has access
- Audit logging
- Encryption at rest

This makes it suitable for:
- HIPAA (healthcare)
- SOC 2 (enterprise)
- GDPR (EU data protection)
- FedRAMP (government)

## CLI Reference

### Server

```bash
gotunnel-server [flags]

Flags:
  -c, --config string      Path to configuration file
  -d, --domain string      Base domain for subdomains
      --addr string        Address to listen on (e.g., :8080)
      --port string        Port to listen on
  -l, --log-level string   Log level: debug, info, warn, error (default "info")
  -h, --help              Help for gotunnel-server
```

### Client

```bash
gotunnel [flags]

Flags:
  -c, --config string      Path to configuration file
  -s, --server string      Server address (default "localhost:9000")
  -t, --token string       Authentication token
      --subdomain string   Subdomain to request
  -p, --port int           Local port to expose
  -l, --log-level string   Log level: debug, info, warn, error (default "info")
  -h, --help              Help for gotunnel
```

## Development

### Prerequisites

- Go 1.21+
- Node.js 18+ (for dashboard)
- SQLite

### Build

```bash
# Backend
go build -o bin/gotunnel-server ./cmd/unified
go build -o bin/gotunnel ./cmd/client

# Frontend
cd src && npm install && npm run build
```

### Run Tests

```bash
go test ./...
```

### Project Structure

```
.
├── cmd/
│   ├── client/         # CLI tunnel client
│   ├── server/         # Multi-port server
│   └── unified/        # Single-port server (recommended)
├── internal/
│   ├── client/         # Client tunnel logic
│   ├── common/         # Shared utilities
│   ├── database/       # SQLite operations
│   ├── protocol/       # Wire protocol
│   └── server/         # Server components
├── src/                # React dashboard
├── configs/            # Example configs
├── deploy/
│   ├── helm/           # Kubernetes Helm chart
│   └── terraform/      # Cloud deployment modules
└── Dockerfile
```

## Roadmap

- [x] Core tunneling
- [x] Web dashboard
- [x] User authentication
- [x] Subdomain reservation
- [ ] Request inspector UI
- [ ] Team/organization support
- [ ] Custom domains
- [ ] Webhook notifications
- [ ] Metrics and analytics
- [ ] SSO/SAML integration

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details.

```bash
# Fork the repo
git clone https://github.com/YOUR_USERNAME/gotunnel.git
cd gotunnel

# Create a branch
git checkout -b feature/amazing-feature

# Make changes and test
go test ./...

# Submit a PR
```

## License

MIT License - see [LICENSE](LICENSE) for details.

## Support

- [GitHub Issues](https://github.com/anyhost/gotunnel/issues) - Bug reports and feature requests
- [Discussions](https://github.com/anyhost/gotunnel/discussions) - Questions and community
- [Discord](https://discord.gg/anyhost) - Real-time chat

---

**Built with security in mind. Your tunnels, your infrastructure, your data.**

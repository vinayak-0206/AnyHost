# AnyHost Docker Compose Deployment

Deploy AnyHost on any VPS for **$4-6/month**.

## Recommended VPS Providers

| Provider | Price | Notes |
|----------|-------|-------|
| [Hetzner](https://hetzner.com) | €3.29/mo | Best value, EU/US |
| [DigitalOcean](https://digitalocean.com) | $4/mo | Easy to use |
| [Vultr](https://vultr.com) | $5/mo | Good performance |
| [Linode](https://linode.com) | $5/mo | Akamai network |
| [Oracle Cloud](https://cloud.oracle.com) | **FREE** | Always Free tier |

## Quick Start

### 1. Get a VPS

```bash
# Minimum specs: 1 CPU, 512MB RAM, Ubuntu 22.04
# Install Docker
curl -fsSL https://get.docker.com | sh
```

### 2. Configure DNS

Point your domain to the VPS IP:
```
tunnel.example.com    A    YOUR_VPS_IP
*.tunnel.example.com  A    YOUR_VPS_IP
```

### 3. Deploy

```bash
# Clone the repo
git clone https://github.com/anyhost/gotunnel.git
cd gotunnel/deploy/docker-compose

# Create tokens file
echo "your-token:your-user-id" > tokens.txt

# Set your domain
export DOMAIN=tunnel.example.com

# Option A: Simple (HTTP only)
docker compose up -d anyhost

# Option B: With Caddy for automatic HTTPS (recommended)
# First edit Caddyfile with your domain
docker compose --profile with-caddy up -d
```

### 4. Connect a client

```bash
./gotunnel --server wss://tunnel.example.com/tunnel \
           --token your-token \
           --subdomain myapp \
           --port 3000
```

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DOMAIN` | `tunnel.example.com` | Base domain |
| `PORT` | `8080` | Internal port |
| `DATABASE_PATH` | `/data/gotunnel.db` | SQLite path |
| `LOG_LEVEL` | `info` | Log verbosity |

### Adding Users

Edit `tokens.txt` (format: `token:user-id`):
```
token1:user1
token2:user2
devtoken:developer
```

Restart to apply:
```bash
docker compose restart
```

## With Nginx (Alternative to Caddy)

```nginx
# /etc/nginx/sites-available/anyhost
server {
    listen 80;
    server_name tunnel.example.com *.tunnel.example.com;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

Then use Certbot for HTTPS:
```bash
apt install certbot python3-certbot-nginx
certbot --nginx -d tunnel.example.com -d *.tunnel.example.com
```

## Monitoring

```bash
# View logs
docker compose logs -f anyhost

# Check health
curl http://localhost:8080/health

# Stats
docker stats anyhost
```

## Backup

```bash
# Backup database
docker compose exec anyhost cat /data/gotunnel.db > backup.db

# Or just copy the volume
docker run --rm -v anyhost-data:/data -v $(pwd):/backup alpine \
    cp /data/gotunnel.db /backup/
```

## Updates

```bash
docker compose pull
docker compose up -d
```

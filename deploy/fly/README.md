# AnyHost on Fly.io

Deploy AnyHost on Fly.io with their generous free tier.

## Free Tier Includes

- 3 shared-cpu VMs (256MB RAM each)
- 160GB outbound data transfer
- Shared IPv4 (dedicated IPv4 is $2/mo)
- Automatic HTTPS with Let's Encrypt

## Quick Start

### 1. Install Fly CLI

```bash
# macOS
brew install flyctl

# Linux
curl -L https://fly.io/install.sh | sh

# Login
fly auth login
```

### 2. Deploy

```bash
cd deploy/fly

# Create the app (first time only)
fly launch --copy-config --no-deploy

# Create persistent volume for database
fly volumes create anyhost_data --size 1 --region sjc

# Set your domain
fly secrets set DOMAIN=tunnel.example.com

# Set auth tokens
fly secrets set AUTH_TOKENS="token1:user1"

# Deploy
fly deploy
```

### 3. Configure DNS

Get your app's IP:
```bash
fly ips list
```

Add DNS records:
```
tunnel.example.com    A    <fly_ip>
*.tunnel.example.com  A    <fly_ip>
```

Or use Fly's automatic SSL with their domain:
```
your-app.fly.dev
```

### 4. Custom Domain with Fly

```bash
# Add your domain to Fly
fly certs add tunnel.example.com
fly certs add "*.tunnel.example.com"

# Follow instructions to add CNAME records
```

## Connect Client

```bash
./gotunnel --server wss://tunnel.example.com/tunnel \
           --token token1 \
           --subdomain myapp \
           --port 3000

# Or using Fly's domain
./gotunnel --server wss://your-app.fly.dev/tunnel \
           --token token1 \
           --subdomain myapp \
           --port 3000
```

## Scaling

```bash
# Scale to more instances (exits free tier)
fly scale count 2

# Scale memory
fly scale memory 512
```

## Monitoring

```bash
# View logs
fly logs

# SSH into the machine
fly ssh console

# Check status
fly status
```

## Regions

Fly.io has data centers worldwide. Change `primary_region` in fly.toml:

| Region | Location |
|--------|----------|
| `sjc` | San Jose, CA |
| `iad` | Ashburn, VA |
| `lhr` | London |
| `fra` | Frankfurt |
| `sin` | Singapore |
| `syd` | Sydney |

## Pricing After Free Tier

| Resource | Free | Paid |
|----------|------|------|
| Shared CPU | 3 VMs | $1.94/mo each |
| RAM | 256MB/VM | $0.00000193/MB/sec |
| Dedicated IPv4 | Shared only | $2/mo |
| Bandwidth | 160GB | $0.02/GB |

## Troubleshooting

### WebSocket issues?
Fly.io fully supports WebSockets. If issues occur:
```bash
# Check logs
fly logs

# Verify the app is running
fly status
```

### Volume not mounting?
```bash
# Check volumes
fly volumes list

# Recreate if needed
fly volumes create anyhost_data --size 1 --region sjc
fly deploy
```

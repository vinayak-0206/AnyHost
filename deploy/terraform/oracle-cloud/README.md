# AnyHost on Oracle Cloud - Always Free

Deploy AnyHost for **$0/month** using Oracle Cloud's Always Free tier.

## What's Included (Free Forever)

| Resource | Free Allocation | AnyHost Usage |
|----------|-----------------|---------------|
| ARM Compute | 4 OCPUs, 24GB RAM | 1 OCPU, 6GB RAM |
| Block Storage | 200GB | 50GB boot volume |
| Outbound Data | 10TB/month | More than enough |
| Load Balancer | 1 flexible | Optional |

## Prerequisites

1. [Oracle Cloud account](https://cloud.oracle.com/free) (free, no credit card for Always Free)
2. Terraform >= 1.0
3. OCI CLI configured

## Setup OCI CLI

```bash
# Install OCI CLI
bash -c "$(curl -L https://raw.githubusercontent.com/oracle/oci-cli/master/scripts/install/install.sh)"

# Configure
oci setup config
```

## Deploy

### 1. Create terraform.tfvars

```hcl
compartment_id      = "ocid1.compartment.oc1..xxxxx"  # Your compartment OCID
domain              = "tunnel.example.com"
ssh_public_key      = "ssh-rsa AAAA..."
availability_domain = "Uocm:PHX-AD-1"  # Check your region's ADs
use_arm             = true  # ARM gives more resources in free tier
auth_tokens         = "mytoken:myuser"
```

### 2. Deploy

```bash
terraform init
terraform plan
terraform apply
```

### 3. Configure DNS

After deploy, add DNS records:
```
tunnel.example.com    A    <public_ip>
*.tunnel.example.com  A    <public_ip>
```

### 4. (Optional) Add HTTPS with Caddy

SSH into the instance and add Caddy:

```bash
ssh opc@<public_ip>

# Edit docker-compose to add Caddy
sudo nano /opt/anyhost/docker-compose.yml
```

Add Caddy service:
```yaml
services:
  anyhost:
    # ... existing config ...
    ports:
      - "8080:8080"  # Change from 80

  caddy:
    image: caddy:2-alpine
    restart: unless-stopped
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./Caddyfile:/etc/caddy/Caddyfile:ro
      - caddy-data:/data
    depends_on:
      - anyhost

volumes:
  caddy-data:
```

Create Caddyfile:
```bash
cat > /opt/anyhost/Caddyfile << 'EOF'
tunnel.example.com, *.tunnel.example.com {
    reverse_proxy anyhost:8080
}
EOF
```

Restart:
```bash
cd /opt/anyhost
sudo docker compose up -d
```

## Connect Client

```bash
./gotunnel --server wss://tunnel.example.com/tunnel \
           --token mytoken \
           --subdomain myapp \
           --port 3000
```

## Monitoring

```bash
# SSH in
ssh opc@<public_ip>

# View logs
cd /opt/anyhost
sudo docker compose logs -f

# Check resources
free -h
df -h
```

## Free Tier Limits

- ARM: 4 OCPUs and 24GB total (we use 1 OCPU, 6GB)
- AMD: 2 micro instances (1GB each)
- Storage: 200GB total block storage
- Network: 10TB outbound/month

**Important**: Stay within these limits to avoid charges.

## Troubleshooting

### Instance not starting?
ARM instances in free tier are in high demand. Try:
- Different availability domain
- Different region
- AMD instance instead (`use_arm = false`)

### Can't SSH?
```bash
# Check security list allows port 22
# Verify SSH key is correct
ssh -vvv opc@<ip>
```

## Cleanup

```bash
terraform destroy
```

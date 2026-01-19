# AnyHost Oracle Cloud Always Free Deployment
# Copy this file to terraform.tfvars and fill in your values
#
# COST: $0/month - Uses Oracle Cloud Always Free Tier
#
# To get these values:
# 1. Log into Oracle Cloud Console: https://cloud.oracle.com
# 2. Compartment ID: Identity & Security > Compartments > Copy OCID
# 3. Availability Domain: Compute > Instances > Create Instance > Check AD names
# 4. SSH Key: Use your existing key or generate: ssh-keygen -t ed25519

# Required: Your OCI compartment OCID
# Find at: Identity & Security > Compartments > [Your Compartment] > Copy OCID
compartment_id = "ocid1.tenancy.oc1..aaaaaaaaoswhy3xndf7jyey2sfawzcp5pwmcnolwzv6incic3ga7tz3syf3a"

# Required: Base domain for your tunnels
# Example: tunnel.yourdomain.com
# Subdomains will be: myapp.tunnel.yourdomain.com
domain = "anyhost-tunnel.duckdns.org"

# Required: Your SSH public key for VM access
# Get with: cat ~/.ssh/id_ed25519.pub (or id_rsa.pub)
ssh_public_key = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIDaUp7ipSX2OOzO1gBGDhl4rITSBCEYyixb0aBP5rRAd vinayak.sharma@clear.in"

# Required: Availability domain
# Check available ADs in your region at: Compute > Instances > Create Instance
# Common formats:
#   - "Uocm:PHX-AD-1" (Phoenix)
#   - "Uocm:US-ASHBURN-AD-1" (Ashburn)
#   - "Uocm:EU-FRANKFURT-1-AD-1" (Frankfurt)
availability_domain = "tpJk:AP-MUMBAI-1-AD-1"

# Optional: Use ARM instances (recommended - more resources in free tier)
# ARM (A1.Flex): 1 OCPU, 6GB RAM - better performance
# AMD (E2.1.Micro): 1 OCPU, 1GB RAM - lower specs
# Note: ARM instances are in high demand, may need to try different ADs/regions
use_arm = false

# Optional: Authentication tokens for tunnel clients
# Format: "token:username" (one per line for multiple)
# Generate secure tokens with: openssl rand -hex 32
auth_tokens = "dev-token:dev-user"

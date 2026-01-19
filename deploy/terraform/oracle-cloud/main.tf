# AnyHost Terraform Module for Oracle Cloud Infrastructure
# COST: $0/month - Uses Oracle Cloud Always Free Tier
#
# Always Free includes:
# - 2 AMD VMs (1 OCPU, 1GB RAM each) OR
# - 4 ARM VMs (1 OCPU, 6GB RAM each) - recommended
# - 200GB block storage
# - 10TB outbound data transfer
#
# Usage:
#   1. Copy terraform.tfvars.example to terraform.tfvars
#   2. Fill in your values
#   3. Run: terraform init && terraform apply

# Variables
variable "compartment_id" {
  description = "OCI Compartment OCID"
  type        = string
}

variable "domain" {
  description = "Base domain for tunnels (e.g., tunnel.example.com)"
  type        = string
}

variable "ssh_public_key" {
  description = "SSH public key for VM access"
  type        = string
}

variable "availability_domain" {
  description = "Availability domain (e.g., 'Uocm:PHX-AD-1')"
  type        = string
}

variable "use_arm" {
  description = "Use ARM instances (A1.Flex) - more resources in free tier"
  type        = bool
  default     = true
}

variable "auth_tokens" {
  description = "Auth tokens in format 'token1:user1\\ntoken2:user2'"
  type        = string
  default     = "dev-token:dev-user"
  sensitive   = true
}

# Data sources
data "oci_identity_availability_domains" "ads" {
  compartment_id = var.compartment_id
}

# Get latest Oracle Linux image
data "oci_core_images" "oracle_linux" {
  compartment_id           = var.compartment_id
  operating_system         = "Oracle Linux"
  operating_system_version = "8"
  shape                    = var.use_arm ? "VM.Standard.A1.Flex" : "VM.Standard.E2.1.Micro"
  sort_by                  = "TIMECREATED"
  sort_order               = "DESC"
}

# VCN (Virtual Cloud Network)
resource "oci_core_vcn" "anyhost" {
  compartment_id = var.compartment_id
  cidr_blocks    = ["10.0.0.0/16"]
  display_name   = "anyhost-vcn"
  dns_label      = "anyhost"
}

# Internet Gateway
resource "oci_core_internet_gateway" "anyhost" {
  compartment_id = var.compartment_id
  vcn_id         = oci_core_vcn.anyhost.id
  display_name   = "anyhost-igw"
  enabled        = true
}

# Route Table
resource "oci_core_route_table" "anyhost" {
  compartment_id = var.compartment_id
  vcn_id         = oci_core_vcn.anyhost.id
  display_name   = "anyhost-rt"

  route_rules {
    destination       = "0.0.0.0/0"
    destination_type  = "CIDR_BLOCK"
    network_entity_id = oci_core_internet_gateway.anyhost.id
  }
}

# Security List
resource "oci_core_security_list" "anyhost" {
  compartment_id = var.compartment_id
  vcn_id         = oci_core_vcn.anyhost.id
  display_name   = "anyhost-sl"

  # Egress - allow all
  egress_security_rules {
    destination = "0.0.0.0/0"
    protocol    = "all"
  }

  # Ingress - SSH
  ingress_security_rules {
    protocol = "6" # TCP
    source   = "0.0.0.0/0"
    tcp_options {
      min = 22
      max = 22
    }
  }

  # Ingress - HTTP
  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 80
      max = 80
    }
  }

  # Ingress - HTTPS
  ingress_security_rules {
    protocol = "6"
    source   = "0.0.0.0/0"
    tcp_options {
      min = 443
      max = 443
    }
  }
}

# Subnet
resource "oci_core_subnet" "anyhost" {
  compartment_id    = var.compartment_id
  vcn_id            = oci_core_vcn.anyhost.id
  cidr_block        = "10.0.1.0/24"
  display_name      = "anyhost-subnet"
  dns_label         = "anyhost"
  route_table_id    = oci_core_route_table.anyhost.id
  security_list_ids = [oci_core_security_list.anyhost.id]
}

# Cloud-init script
locals {
  cloud_init = <<-EOF
    #!/bin/bash
    set -e

    # Install Docker
    dnf install -y dnf-utils
    dnf config-manager --add-repo https://download.docker.com/linux/centos/docker-ce.repo
    dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin
    systemctl enable docker
    systemctl start docker

    # Create app directory
    mkdir -p /opt/anyhost
    cd /opt/anyhost

    # Create tokens file
    cat > tokens.txt << 'TOKENS'
    ${var.auth_tokens}
    TOKENS

    # Create docker-compose.yml
    cat > docker-compose.yml << 'COMPOSE'
    version: '3.8'
    services:
      anyhost:
        image: anyhost/gotunnel:latest
        restart: unless-stopped
        ports:
          - "80:8080"
          - "443:8443"
        environment:
          - DOMAIN=${var.domain}
          - DATABASE_PATH=/data/gotunnel.db
        volumes:
          - ./data:/data
          - ./tokens.txt:/app/tokens.txt:ro
    COMPOSE

    # Start the service
    docker compose up -d

    # Setup auto-update (optional)
    cat > /etc/cron.daily/anyhost-update << 'CRON'
    #!/bin/bash
    cd /opt/anyhost
    docker compose pull
    docker compose up -d
    CRON
    chmod +x /etc/cron.daily/anyhost-update
  EOF
}

# Compute Instance (Always Free)
resource "oci_core_instance" "anyhost" {
  compartment_id      = var.compartment_id
  availability_domain = var.availability_domain
  display_name        = "anyhost"

  # Always Free shapes:
  # - VM.Standard.E2.1.Micro (AMD, 1 OCPU, 1GB) - 2 instances free
  # - VM.Standard.A1.Flex (ARM, up to 4 OCPU, 24GB) - recommended
  shape = var.use_arm ? "VM.Standard.A1.Flex" : "VM.Standard.E2.1.Micro"

  dynamic "shape_config" {
    for_each = var.use_arm ? [1] : []
    content {
      ocpus         = 1
      memory_in_gbs = 6
    }
  }

  source_details {
    source_type = "image"
    source_id   = data.oci_core_images.oracle_linux.images[0].id
    # 50GB boot volume (free tier allows up to 200GB total)
    boot_volume_size_in_gbs = 50
  }

  create_vnic_details {
    subnet_id        = oci_core_subnet.anyhost.id
    assign_public_ip = true
  }

  metadata = {
    ssh_authorized_keys = var.ssh_public_key
    user_data          = base64encode(local.cloud_init)
  }

  # Prevent accidental deletion
  lifecycle {
    prevent_destroy = false
  }
}

# Outputs are defined in outputs.tf

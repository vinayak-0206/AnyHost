# Outputs for AnyHost Oracle Cloud Deployment

output "public_ip" {
  description = "Public IP address of the AnyHost instance"
  value       = oci_core_instance.anyhost.public_ip
}

output "ssh_command" {
  description = "SSH command to connect to the instance"
  value       = "ssh opc@${oci_core_instance.anyhost.public_ip}"
}

output "tunnel_endpoint" {
  description = "WebSocket tunnel endpoint for clients"
  value       = "wss://${var.domain}/tunnel"
}

output "dashboard_url" {
  description = "URL for the web dashboard"
  value       = "https://${var.domain}"
}

output "dns_instructions" {
  description = "DNS records you need to create"
  value       = <<-EOF

    ============================================
    DNS CONFIGURATION REQUIRED
    ============================================

    Add these DNS records at your domain registrar:

    Type    Name                    Value
    ----    ----                    -----
    A       ${var.domain}           ${oci_core_instance.anyhost.public_ip}
    A       *.${var.domain}         ${oci_core_instance.anyhost.public_ip}

    Once DNS propagates, your tunnel will be available at:
    https://${var.domain}

    Client connection:
    ./gotunnel --server wss://${var.domain}/tunnel \
               --token <your-token> \
               --subdomain myapp \
               --port 3000

    ============================================
  EOF
}

output "next_steps" {
  description = "Next steps after deployment"
  value       = <<-EOF

    ============================================
    NEXT STEPS
    ============================================

    1. Configure DNS (see dns_instructions output)

    2. (Optional) Add HTTPS with Caddy:
       ssh opc@${oci_core_instance.anyhost.public_ip}
       # Follow README.md instructions for Caddy setup

    3. Connect a tunnel client:
       ./gotunnel --server wss://${var.domain}/tunnel \
                  --token dev-token \
                  --subdomain myapp \
                  --port 3000

    4. Access your service:
       https://myapp.${var.domain}

    ============================================
  EOF
}

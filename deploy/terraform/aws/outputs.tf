output "alb_dns_name" {
  description = "DNS name of the Application Load Balancer"
  value       = aws_lb.main.dns_name
}

output "alb_zone_id" {
  description = "Zone ID of the Application Load Balancer (for Route53 alias records)"
  value       = aws_lb.main.zone_id
}

output "alb_arn" {
  description = "ARN of the Application Load Balancer"
  value       = aws_lb.main.arn
}

output "ecs_cluster_name" {
  description = "Name of the ECS cluster"
  value       = aws_ecs_cluster.main.name
}

output "ecs_cluster_arn" {
  description = "ARN of the ECS cluster"
  value       = aws_ecs_cluster.main.arn
}

output "ecs_service_name" {
  description = "Name of the ECS service"
  value       = aws_ecs_service.main.name
}

output "cloudwatch_log_group" {
  description = "CloudWatch log group name"
  value       = aws_cloudwatch_log_group.main.name
}

output "security_group_alb_id" {
  description = "Security group ID for the ALB"
  value       = aws_security_group.alb.id
}

output "security_group_ecs_id" {
  description = "Security group ID for ECS tasks"
  value       = aws_security_group.ecs.id
}

output "efs_file_system_id" {
  description = "EFS file system ID (if enabled)"
  value       = var.enable_efs ? aws_efs_file_system.main[0].id : null
}

output "tunnel_endpoint" {
  description = "Tunnel WebSocket endpoint"
  value       = "wss://${var.domain}/tunnel"
}

output "dashboard_url" {
  description = "Dashboard URL"
  value       = "https://${var.domain}"
}

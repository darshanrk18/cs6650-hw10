output "aws_region" {
  value       = var.aws_region
  description = "Region used for ECR and EC2."
}

output "ecr_repository_url" {
  value       = aws_ecr_repository.kv.repository_url
  description = "ECR URL; tag images as <url>:leader-follower etc."
}

output "kv_public_ip" {
  value       = aws_eip.kv.public_ip
  description = "Elastic IP — use for load tests from your laptop."
}

output "instance_id" {
  value       = aws_instance.kv.id
  description = "EC2 instance id (SSM: aws ssm start-session --target <id>)."
}

output "ecr_push_commands" {
  value = <<-EOT
    aws ecr get-login-password --region ${var.aws_region} | docker login --username AWS --password-stdin ${aws_ecr_repository.kv.repository_url}
    docker build -f Dockerfile.leader-follower -t kv-leader-follower .
    docker tag kv-leader-follower:latest ${aws_ecr_repository.kv.repository_url}:leader-follower
    docker push ${aws_ecr_repository.kv.repository_url}:leader-follower
    docker build -f Dockerfile.leaderless -t kv-leaderless .
    docker tag kv-leaderless:latest ${aws_ecr_repository.kv.repository_url}:leaderless
    docker push ${aws_ecr_repository.kv.repository_url}:leaderless
  EOT
}

output "loadtest_example" {
  value = <<-EOT
    EPS="http://${aws_eip.kv.public_ip}:8080,http://${aws_eip.kv.public_ip}:8081,http://${aws_eip.kv.public_ip}:8082,http://${aws_eip.kv.public_ip}:8083,http://${aws_eip.kv.public_ip}:8084"
    go run ./cmd/loadtest -mode=leader-follower -leader=http://${aws_eip.kv.public_ip}:8080 -endpoints="$EPS" -write-ratio=0.5 -duration=60s -profile=w5r1 -out=results/aws-lf-w5r1-w0.5.json
  EOT
}

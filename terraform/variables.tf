variable "aws_region" {
  type        = string
  description = "AWS region for ECR and optional compute."
  default     = "us-east-1"
}

variable "ecr_repo_name" {
  type        = string
  description = "ECR repository name for KV images."
  default     = "cs6650-kv-replication"
}

variable "instance_type" {
  type        = string
  description = "EC2 instance size (5+ containers — avoid nano)."
  default     = "t3.small"
}

variable "allowed_cidr" {
  type        = list(string)
  description = "CIDRs allowed to reach KV ports (8080–8084, 18080–18084)."
  default     = ["0.0.0.0/0"]
}

variable "quorum_profile" {
  type        = string
  description = "Initial QUORUM_PROFILE baked into user-data (w5r1 | w1r5 | r3w3). Change on host and docker compose up -d if needed."
  default     = "w5r1"
}

variable "manage_iam" {
  type        = bool
  description = "If false, skip creating IAM role/profile (required on AWS Academy / accounts without iam:CreateRole). Use existing_instance_profile."
  default     = false
}

variable "existing_instance_profile" {
  type        = string
  description = "Instance profile name when manage_iam=false (ECR pull + SSM). AWS Academy often provides e.g. LabInstanceProfile — check EC2 launch wizard."
  default     = ""

  validation {
    condition     = var.manage_iam || length(var.existing_instance_profile) > 0
    error_message = "When manage_iam is false, existing_instance_profile must be set to a profile that can pull from your ECR registry."
  }
}

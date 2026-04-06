resource "aws_security_group" "kv_host" {
  name_prefix = "${var.ecr_repo_name}-sg-"
  description = "CS6650 KV HTTP ports from internet (tighten for production)"
  vpc_id      = data.aws_vpc.default.id

  ingress {
    description = "Leader-follower nodes lf0-lf4"
    from_port   = 8080
    to_port     = 8084
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidr
  }

  ingress {
    description = "Leaderless ll0-ll4 on host map"
    from_port   = 18080
    to_port     = 18084
    protocol    = "tcp"
    cidr_blocks = var.allowed_cidr
  }

  egress {
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = ["0.0.0.0/0"]
  }

  lifecycle {
    create_before_destroy = true
  }

  tags = { Project = "cs6650-hw10" }
}

resource "aws_instance" "kv" {
  ami                         = data.aws_ami.al2023.id
  instance_type               = var.instance_type
  subnet_id                   = local.instance_subnet_id
  associate_public_ip_address = true
  vpc_security_group_ids      = [aws_security_group.kv_host.id]
  iam_instance_profile        = local.instance_profile_name

  user_data = base64encode(templatefile("${path.module}/user-data.sh.tftpl", {
    aws_region       = var.aws_region
    ecr_registry     = aws_ecr_repository.kv.repository_url
    image_lf         = "${aws_ecr_repository.kv.repository_url}:leader-follower"
    image_ll         = "${aws_ecr_repository.kv.repository_url}:leaderless"
    quorum_profile   = var.quorum_profile
  }))

  metadata_options {
    http_tokens = "optional"
  }

  root_block_device {
    volume_size = 30
    volume_type = "gp3"
  }

  tags = {
    Name        = "${var.ecr_repo_name}-host"
    Project     = "cs6650-hw10"
    QuorumStart = var.quorum_profile
  }

  depends_on = [
    aws_ecr_repository.kv,
  ]
}

resource "aws_eip" "kv" {
  domain = "vpc"
  tags   = { Project = "cs6650-hw10" }
}

resource "aws_eip_association" "kv" {
  instance_id   = aws_instance.kv.id
  allocation_id = aws_eip.kv.id
}

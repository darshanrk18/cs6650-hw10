# When manage_iam=false (e.g. AWS Academy / restricted accounts), set
# existing_instance_profile to a profile that allows ecr:GetAuthorizationToken + batch checks
# (often the console shows a lab profile such as LabInstanceProfile).

resource "aws_iam_role" "kv_host" {
  count = var.manage_iam ? 1 : 0
  name  = "${var.ecr_repo_name}-ec2-role"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Action = "sts:AssumeRole"
      Effect = "Allow"
      Principal = {
        Service = "ec2.amazonaws.com"
      }
    }]
  })

  tags = { Project = "cs6650-hw10" }
}

resource "aws_iam_role_policy_attachment" "ecr_read" {
  count      = var.manage_iam ? 1 : 0
  role       = aws_iam_role.kv_host[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
}

resource "aws_iam_role_policy_attachment" "ssm" {
  count      = var.manage_iam ? 1 : 0
  role       = aws_iam_role.kv_host[0].name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

resource "aws_iam_instance_profile" "kv" {
  count = var.manage_iam ? 1 : 0
  name  = "${var.ecr_repo_name}-profile"
  role  = aws_iam_role.kv_host[0].name
}

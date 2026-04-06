locals {
  instance_profile_name = var.manage_iam ? aws_iam_instance_profile.kv[0].name : var.existing_instance_profile

  public_subnet_ids = [
    for id, sn in data.aws_subnet.from_vpc : id
    if sn.map_public_ip_on_launch
  ]
  instance_subnet_id = length(local.public_subnet_ids) > 0 ? sort(local.public_subnet_ids)[0] : data.aws_subnets.default.ids[0]
}

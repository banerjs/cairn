output "aws_region" {
  description = "AWS region used for the bucket and IAM entities."
  value       = var.aws_region
}

output "bucket_name" {
  description = "S3 bucket used by cairn."
  value       = aws_s3_bucket.cairn.bucket
}

output "bucket_arn" {
  description = "ARN of the backup bucket."
  value       = aws_s3_bucket.cairn.arn
}

output "iam_user_arn" {
  description = "IAM user granted scoped S3 access."
  value       = aws_iam_user.cairn.arn
}

output "access_key_id" {
  description = "Access key id for ~/.aws/credentials (rotate periodically)."
  value       = aws_iam_access_key.cairn.id
}

output "secret_access_key" {
  description = "Secret access key — copy once; Terraform stores in state."
  value       = aws_iam_access_key.cairn.secret
  sensitive   = true
}

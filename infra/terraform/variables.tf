variable "aws_region" {
  type        = string
  description = "AWS region for the bucket and IAM entities."
  default     = "us-west-2"
}

variable "bucket_name" {
  type        = string
  description = "Globally unique S3 bucket name for cairn backups."
}

variable "iam_user_name" {
  type        = string
  description = "IAM user that holds programmatic access for cairn CLI hosts."
  default     = "cairn-backup"
}

variable "transition_glacier_ir_days" {
  type        = number
  description = "Days after object creation before transitioning objects under cairn/v1/ to GLACIER_IR; set null to disable."
  default     = 15
  nullable    = true
}

variable "transition_deep_archive_days" {
  type        = number
  description = "Days after object creation before transitioning to DEEP_ARCHIVE (after GLACIER_IR); null disables."
  default     = 30
  nullable    = true
}

variable "noncurrent_expiration_days" {
  type        = number
  description = "Expire noncurrent object versions after this many days (versioning cleanup)."
  default     = 30
}

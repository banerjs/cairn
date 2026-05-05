terraform {
  required_version = ">= 1.5"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.0"
    }
  }
}

provider "aws" {
  region = var.aws_region
}

resource "aws_s3_bucket" "cairn" {
  bucket = var.bucket_name
}

resource "aws_s3_bucket_versioning" "cairn" {
  bucket = aws_s3_bucket.cairn.id
  versioning_configuration {
    status = "Enabled"
  }
}

resource "aws_s3_bucket_public_access_block" "cairn" {
  bucket = aws_s3_bucket.cairn.id

  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "cairn" {
  bucket = aws_s3_bucket.cairn.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "cairn" {
  bucket = aws_s3_bucket.cairn.id

  rule {
    id     = "cairn-tier-glacier-ir"
    status = var.transition_glacier_ir_days != null ? "Enabled" : "Disabled"

    filter {
      prefix = "cairn/v1/"
    }

    dynamic "transition" {
      for_each = var.transition_glacier_ir_days != null ? [var.transition_glacier_ir_days] : []
      content {
        days          = transition.value
        storage_class = "GLACIER_IR"
      }
    }
  }

  rule {
    id     = "cairn-tier-deep-archive"
    status = var.transition_deep_archive_days != null ? "Enabled" : "Disabled"

    filter {
      prefix = "cairn/v1/"
    }

    dynamic "transition" {
      for_each = var.transition_deep_archive_days != null ? [var.transition_deep_archive_days] : []
      content {
        days          = transition.value
        storage_class = "DEEP_ARCHIVE"
      }
    }
  }

  rule {
    id     = "expire-noncurrent-versions"
    status = "Enabled"

    filter {}

    noncurrent_version_expiration {
      noncurrent_days = var.noncurrent_expiration_days
    }
  }
}

resource "aws_iam_user" "cairn" {
  name = var.iam_user_name
}

data "aws_iam_policy_document" "cairn" {
  statement {
    sid    = "ListBucketScoped"
    effect = "Allow"
    actions = [
      "s3:ListBucket",
    ]
    resources = [
      aws_s3_bucket.cairn.arn,
    ]
    condition {
      test     = "StringLike"
      variable = "s3:prefix"
      values   = ["cairn/v1/*"]
    }
  }

  statement {
    sid    = "ObjectRW"
    effect = "Allow"
    actions = [
      "s3:GetObject",
      "s3:PutObject",
      "s3:DeleteObject",
      "s3:AbortMultipartUpload",
      "s3:ListMultipartUploadParts",
      "s3:CreateMultipartUpload",
      "s3:UploadPart",
      "s3:CompleteMultipartUpload",
    ]
    resources = [
      "${aws_s3_bucket.cairn.arn}/cairn/v1/*",
    ]
  }
}

resource "aws_iam_user_policy" "cairn" {
  name   = "cairn-s3-access"
  user   = aws_iam_user.cairn.name
  policy = data.aws_iam_policy_document.cairn.json
}

resource "aws_iam_access_key" "cairn" {
  user = aws_iam_user.cairn.name
}

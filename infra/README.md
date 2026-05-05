# Cairn AWS infrastructure (Terraform)

Minimal bucket + IAM setup matching the cairn design: versioning, SSE-S3, public access blocked, optional lifecycle tiers, and an IAM user restricted to `cairn/v1/*`.

## Prerequisites

- Terraform >= 1.5
- AWS credentials with permission to create S3 buckets and IAM users/policies

## Apply

```bash
cd infra/terraform
terraform init
terraform apply \
  -var='bucket_name=my-org-cairn-backups-unique' \
  -var='aws_region=us-west-2'
```

Optional lifecycle knobs:

```bash
terraform apply \
  -var='bucket_name=...' \
  -var='transition_glacier_ir_days=90' \
  -var='transition_deep_archive_days=null' \
  -var='noncurrent_expiration_days=30'
```

Set `transition_glacier_ir_days = null` to disable GLACIER_IR transitions entirely.

## Outputs & credentials

After apply:

```bash
terraform output access_key_id
terraform output -raw secret_access_key
```

Configure `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` (or `~/.aws/credentials`) on backup hosts. **Never commit secrets.**

### Rotate keys

1. Create a second access key in IAM (or `terraform taint aws_iam_access_key.cairn` then apply — destructive to old key).
2. Update hosts to the new key.
3. Delete the old inactive key.

### State & locking

For production, use remote state (e.g. S3 backend + DynamoDB lock). This repo ships local state only as a starting point.

## Scope

The IAM policy allows:

- `s3:ListBucket` with `s3:prefix` `cairn/v1/*`
- Object read/write/delete under `arn:aws:s3:::BUCKET/cairn/v1/*`, including multipart upload APIs used by the AWS SDK uploader.

It does **not** grant bucket-wide delete outside that prefix or unrelated buckets.

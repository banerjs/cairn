# Cairn Design (Descriptive)

This document preserves the descriptive design context for `cairn` before implementation.

## Scope and Principles

- Personal, zero-trust backup tool targeting AWS S3 only.
- Stream pipeline: `read -> sha256 -> zstd -> age -> S3`.
- No shelling out for core operations.
- Simplicity first: avoid optional complexity unless it materially improves correctness.

## Platform and Language

- Language: Go.
- OS targets: Linux, macOS, Windows.
- Post-quantum-only encryption in v1: age hybrid `mlkem768x25519`.

## Key Dependencies

- `filippo.io/age >= v1.3.0`
- `github.com/klauspost/compress/zstd`
- `github.com/aws/aws-sdk-go-v2/service/s3`
- `github.com/aws/aws-sdk-go-v2/feature/s3/manager`
- `github.com/BurntSushi/toml`
- `github.com/sabhiram/go-gitignore`
- `github.com/google/uuid`
- test-only: `github.com/testcontainers/testcontainers-go`

## S3 Layout and Versioning

All keys are under `cairn/v1/` (bucket-layout version), while manifest/index schema versions are independent (`cairn.manifest.v1`, `cairn.index.v1`).

```text
s3://<bucket>/cairn/v1/
  hosts/<host-id>/snapshots/<snapshot-id>/
    manifest.age
    objects/<object-id>
  hosts/<host-id>/index.age
```

## Manifest and Index

- Manifest format: JSON schema v1, envelope `json -> zstd -> age`.
- Index format: encrypted derived cache, non-authoritative.
- Manifest key fields include snapshot metadata, file entries, directory entries, and stats.
- `encryption.recipient_type` in v1 is always `mlkem768x25519`.

## Backup/Restore/Verify Behavior

- Backup creates a snapshot directory, uploads encrypted objects, then writes `manifest.age` as commit point.
- Partial snapshots are detected by missing `manifest.age`; stale partials are garbage-collected based on `cleanup_grace`.
- Restore rehydrates files and verifies plaintext SHA-256 while streaming.
- Verify samples objects (`--sample 0` means full verification) and checks object existence against the manifest.

## Key Management

- Recipients are PQ-only: `age1pq1...` (and optional `age1tagpq1...`).
- Identities are `AGE-SECRET-KEY-PQ-1...`.
- Identity path sources: `CAIRN_IDENTITY_FILE` env, then config.
- Multiple recipients do not duplicate object bodies: data is encrypted once, with multiple recipient stanzas in header.

## Concurrency and Safety

- Parallel uploads with bounded worker pool.
- No S3 lock object in v1; snapshot IDs are collision-resistant.
- Explicit error handling, no silent failures, no `panic` control-flow.

## Windows Notes

- Manifest paths are normalized to forward slash.
- Mode/uid/gid are optional and omitted on Windows.
- Symlink behavior and permissions are best-effort and documented.

## IaC and Cost Reporting

- Separate `infra/terraform` section for bucket, IAM, lifecycle, and policies.
- Main CLI remains limited to object verbs (`PutObject`, `GetObject`, `ListObjectsV2`, `DeleteObject`).
- `status` command estimates usage and optional cost via storage-class totals from listing.

## Forward Compatibility

- v1 ships without dedup.
- v2 plan: start with whole-file dedup (shared blob pool), keep CDC as potential later phase.
- Preserve compatibility by treating `object_id` as opaque and dispatching prune behavior by manifest schema version.

## Configuration and Ignore Rules

- TOML config includes host, S3, encryption, and backup sections.
- Include/exclude behavior combines config rules with hierarchical `.cairnignore` files (gitignore semantics).

## CLI Surface

Core Prompt 2 commands:

- `backup <config-file> [--storage-class CLASS]`
- `restore <snapshot-id> --target <path>`
- `snapshots [--host HOSTNAME]`
- `verify <snapshot-id> [--sample N]`
- `prune --keep-last N [--keep-monthly M] [--dry-run]`

Additive commands:

- `status [--host HOSTNAME] [--show-cost]`
- `keygen --output PATH`
- `version`

# Cairn Prompt 2 Implementation Plan (Prescriptive)

This document is implementation-facing and intentionally concise. Follow in order.

## 0) Hard Constraints (must hold)

- Use Go.
- Use native libraries only for encryption/compression/hashing/S3.
- Post-quantum-only recipients in v1 (`mlkem768x25519` via age v1.3.0+).
- Secrets from env/config only; do not accept identity path via CLI.
- Required object-level S3 operations only: `PutObject`, `GetObject`, `ListObjectsV2`, `DeleteObject`.

## 1) Repository Bootstrap

1. Initialize Go module.
2. Create top-level structure:
   - `cmd/cairn/`
   - `internal/config/`
   - `internal/crypto/`
   - `internal/s3store/`
   - `internal/manifest/`
   - `internal/backup/`
   - `internal/restore/`
   - `internal/verify/`
   - `internal/prune/`
   - `internal/ignore/`
   - `internal/status/`
   - `internal/keygen/`
   - `infra/terraform/`
   - `examples/`
3. Add `Makefile` targets: `fmt`, `lint`, `test`, `integration`, `build-all`.
4. Add CI workflow matrix (linux/macos/windows).

## 2) Config and Validation

1. Implement `config.toml` parsing with env expansion.
2. Support sections/fields from design:
   - host ID, cleanup grace
   - S3 bucket/region/default storage class
   - encryption recipients + identity file path
   - backup source roots, parallelism, symlink policy, includes/excludes
3. Add strong validation:
   - recipient list non-empty
   - recipients must be PQ-only formats
   - host ID regex
   - storage class enum
4. Identity path resolution: env first, then config.

## 3) Data Model and Serialization

1. Implement manifest structs for `cairn.manifest.v1`.
2. Implement index structs for `cairn.index.v1`.
3. Implement envelope helpers:
   - encode: `json -> zstd -> age`
   - decode: `age -> zstd -> json`
4. Ensure unknown fields are tolerated on deserialize.

## 4) S3 Store Layer

1. Build thin typed wrapper around AWS SDK:
   - Put object
   - Get object stream
   - List with prefix (paged)
   - Delete object
2. Add multipart upload path via `s3/manager`.
3. Keep all S3 paths centralized (namespace builder utilities).

## 5) Core Commands

### backup

1. Load config and recipients.
2. Walk source trees with ignore engine (`config rules + .cairnignore`).
3. For each file: stream plaintext hash + compress + encrypt + upload.
4. Collect metadata and assemble manifest.
5. Write `manifest.age` as commit point.
6. Refresh `index.age`.
7. Before upload phase, clean stale partial snapshots per `cleanup_grace`.

### restore

1. Fetch/decrypt manifest.
2. Recreate directory structure.
3. Download/decrypt/decompress/write each file to temporary path, then rename.
4. Validate plaintext hash for every restored file.
5. Reapply directory metadata best-effort.

### snapshots

1. Attempt index read.
2. Fallback to list+discover from manifests if index missing/stale/undecryptable.
3. Support optional host filter.

### verify

1. Fetch/decrypt manifest.
2. Random sample entries (`0` = all).
3. Stream decrypt/decompress/hash and compare.
4. Also check object existence coverage via list against manifest references.

### prune

1. Resolve retention set from `--keep-last` and optional `--keep-monthly`.
2. For v1, delete unretained snapshot prefixes.
3. Honor `--dry-run`.

### status / keygen / version

- Implement as additive subcommands.
- `keygen` emits PQ identity and matching recipient.

## 6) CLI Surface

Implement these command forms:

- `backup <config-file> [--storage-class CLASS] [--parallelism N] [-v]`
- `restore <snapshot-id> --target <path> [--config PATH] [--parallelism N] [-v]`
- `snapshots [--host HOSTNAME] [--config PATH] [-v]`
- `verify <snapshot-id> [--sample N] [--config PATH] [--parallelism N] [-v]`
- `prune --keep-last N [--keep-monthly M] [--dry-run] [--config PATH] [-v]`
- `status [--host HOSTNAME] [--show-cost] [--config PATH] [-v]`
- `keygen --output PATH`
- `version`

## 7) Code Quality Requirements

1. Add doc comments for exported crypto/hash/S3/filesystem-touching functions.
2. Ensure explicit error returns and `%w` wrapping at boundaries.
3. No silent error suppression.
4. No panic-based control flow.

## 8) Test Plan (minimum)

Unit tests:

- manifest serialize/encrypt/decrypt/deserialize round-trip
- verify command logic (good/tampered objects)
- prune retention selection
- interrupted backup cleanup behavior (fresh partial kept, stale partial removed)
- recipient validation (reject classical; accept PQ)
- path normalization behavior (including Windows normalization)

Integration test:

- MinIO/local S3-compatible: backup -> snapshots -> verify -> restore

## 9) Infra Deliverable

In `infra/terraform` implement:

- bucket
- versioning
- public access block
- SSE-S3 baseline
- optional lifecycle transitions
- IAM user + scoped policy matching CLI object-level verbs

Include `infra/README.md` with apply/rotate usage.

## 10) Documentation Deliverables

1. `README.md` with:
   - install
   - first-time key generation
   - config setup
   - backup/restore usage
   - scheduling (cron/systemd/launchd/Task Scheduler)
   - threat model
2. `examples/config.toml` with comments for every field.

## 11) Definition of Done

- All commands compile and run.
- `make fmt lint test` passes.
- Integration test passes.
- No CLI identity-path flag exists.
- PQ-only recipient policy is enforced.
- Required docs and infra files are present.

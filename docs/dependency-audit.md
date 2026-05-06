# Dependency audit (longevity / Prompt 4)

Goal: keep the core backup path understandable and rebuildable years later. Third-party surface is intentionally small.

**Last reviewed:** 2026-05-06 (pinned versions below match `go.mod` at that time; re-check before each release).

## Runtime (locked in design)

| Module | Version (pinned) | Role | Notes |
|--------|------------------|------|--------|
| `filippo.io/age` | v1.3.1 | PQ hybrid encryption | Reference implementation; v1.3+ supplies `age1pq1` / hybrid APIs. |
| `filippo.io/age/tag` | (via age) | Tagged PQ recipients | `age1tagpq1...` parsing. |
| `filippo.io/hpke` | v0.4.0 (indirect) | Crypto primitive | Pulled by age for hybrid path; not imported by cairn directly. |
| `filippo.io/nistec` | v0.0.4 (indirect) | Curve / PQ plumbing | Indirect via age. |
| `github.com/klauspost/compress/zstd` | v1.18.6 | Compression | Pure Go; standard Zstandard frames (see [RFC 8878](https://www.rfc-editor.org/rfc/rfc8878.html)). |
| `github.com/aws/aws-sdk-go-v2` (+ `config`, `credentials`, `service/s3`, `feature/s3/manager`) | see `go.mod` | Object API + multipart uploads | Industry standard; IAM in Terraform should track List/Get/Put/Delete/Multipart as used. |
| `github.com/BurntSushi/toml` | v1.6.0 | Config parsing | Stable, minimal API. |
| `github.com/sabhiram/go-gitignore` | pseudo-version | Ignore rules | Matches `.gitignore` semantics; avoids hand-rolled glob edge cases. |
| `github.com/google/uuid` | v1.6.0 | Random object IDs | UUIDv4 strings per layout spec. |

Go toolchain: **`go 1.25.0`** in `go.mod` — release builds assume that language/runtime baseline.

## Test-only

| Module | Version (pinned) | Role |
|--------|------------------|------|
| `github.com/testcontainers/testcontainers-go` | v0.40.0 | MinIO integration (`//go:build integration`) |
| `github.com/johannesboyne/gofakes3` | pseudo-version | In-process S3 fake for `internal/s3test` unit tests |

These pull a larger transitive graph (Docker / OTel for testcontainers). **Not linked into release binaries.** Acceptable trade-off for realistic S3 coverage on Linux CI.

## Stdlib / generated

- `log/slog`, `crypto/sha256`, `encoding/json`, `flag` — preferred over heavier CLI frameworks per design.
- `path` for S3 key composition (forward slashes; matches `format/FORMAT.md`).

## Implementation fidelity and longevity risks

This is the code-side companion to **Prompt 4 §2**: behavior that is *not* fully specified in `format/FORMAT.md` or the age spec, but that a future restorer should know about.

| Area | What the code does | Risk / mitigation |
|------|--------------------|-------------------|
| **JSON** | `encoding/json` decodes manifests and indexes into structs without `DisallowUnknownFields`. | Unknown fields are dropped on read, matching FORMAT’s “ignore unknown fields within a known major.” New writers can add optional fields without breaking this binary. |
| **Zstd level** | Default level **3** for manifest/index-sized blobs; file-object level comes from config and is recorded in `manifest.compression`. | Restorers must use a Zstandard decoder, not a specific encoder level. |
| **Envelope order** | Buffered and streaming paths both implement **plaintext → zstd → age** (decrypt then decompress). | Order is normative in FORMAT; both code paths match. |
| **age API** | Uses `age.Encrypt` / `age.Decrypt` and hybrid recipient helpers. | Wire format is the [age](https://github.com/FiloSottile/age/blob/main/doc/age.md) file format; library is the reference implementation but long-term truth is the age spec + FORMAT. |
| \*\* POSIX metadata\*\* | Unix: `syscall.Stat_t` for mode/uid/gid. Windows: optional fields omitted. | Restore applies safe defaults when fields missing; not a cross-platform identity-preservation guarantee. |
| **Local filesystem paths** | Restore targets use `filepath` and OS separator rules. | Only S3 keys are POSIX-style; local layout is host-specific. |
| **AWS behavior** | Default credential chain and region from TOML via `config.LoadDefaultConfig`. | Depends on AWS SDK and environment conventions (profiles, IMDS, env vars) — operational, not on-disk format. |
| **Multipart uploads** | S3 manager for large objects. | Committed object is a normal S3 object; multipart is transport-only. |

Nothing in the backup path depends on cgo for crypto or compression in this module set (pure Go + stdlib).

## Audit cadence

Before each tagged release:

1. `go list -m -u all` — note upgrades; prefer patch/minor bumps where possible.
1. Re-run `go test ./...` and `-tags=integration` on Linux with Docker.
1. Skim AWS SDK + age release notes for breaking crypto or S3 client behavior changes.
1. If `format/FORMAT.md` changed, refresh this doc’s “implementation fidelity” table only if new couplings appeared.

## FORMAT.md independence

On-disk semantics are documented in `format/FORMAT.md` (also embedded via `export-recovery-kit`). A future reader can reimplement restore without this Go codebase, using that document and the age format specification.

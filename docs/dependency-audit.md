# Dependency audit (longevity / Prompt 4)

Goal: keep the core backup path understandable and rebuildable years later. Third-party surface is intentionally small.

## Runtime (locked in design)

| Module | Role | Notes |
|--------|------|--------|
| `filippo.io/age` | PQ hybrid encryption | Reference implementation; v1.3+ for `age1pq1` / `age1tagpq1`. |
| `github.com/klauspost/compress/zstd` | Compression | Pure Go; widely used. |
| AWS SDK v2 (`service/s3`, `feature/s3/manager`) | Object API + multipart uploads | Industry standard; multipart verbs reflected in Terraform IAM. |
| `github.com/BurntSushi/toml` | Config parsing | Stable, minimal API. |
| `github.com/sabhiram/go-gitignore` | Ignore rules | Matches design; avoids hand-rolled glob edge cases. |
| `github.com/google/uuid` | Random object IDs | v4 strings per layout spec. |

## Test-only

| Module | Role |
|--------|------|
| `github.com/testcontainers/testcontainers-go` | MinIO integration (`//go:build integration`) |

Pulls a larger transitive graph (Docker/Otel). **Not linked into release binaries.** Acceptable trade-off for realistic S3 integration on Linux CI.

## Stdlib / generated

- `log/slog`, `crypto/sha256`, `encoding/json`, `flag` — preferred over heavier CLI frameworks per design.

## Audit cadence

Before each tagged release:

1. `go list -m -u all` — note upgrades; prefer patch/minor bumps.
1. Re-run `go test ./...` and `-tags=integration` on Linux with Docker.
1. Skim AWS SDK + age release notes for breaking crypto or S3 behavior changes.

## FORMAT.md independence

On-disk semantics are documented in `format/FORMAT.md` (also embedded via `export-recovery-kit`). A future reader can reimplement restore without this Go codebase.

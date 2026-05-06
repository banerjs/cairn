# Cairn

[![CI](https://github.com/banerjs/cairn/actions/workflows/ci.yml/badge.svg)](https://github.com/banerjs/cairn/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/banerjs/cairn/graph/badge.svg)](https://codecov.io/gh/banerjs/cairn)

Personal, zero-trust backups to **Amazon S3**. Files are streamed **`read → SHA-256 → zstd → age → S3`**. The bucket sees only opaque ciphertext under random object IDs inside the `cairn/v1/` layout.

- **Post-quantum age only** for recipients: `age1pq1...` / `age1tagpq1...` (hybrid ML-KEM + X25519 / hardware-backed variants via `filippo.io/age` v1.3+).
- **Single static binary** (Go), Linux/macOS/Windows.
- **Manifest & index**: JSON documents stored as **`json → zstd → age`**.

Full layout & envelope specification: **[format/FORMAT.md](format/FORMAT.md)** (also emitted by `export-recovery-kit`).

## Install / build

Requires **Go 1.25+** (see `go.mod`).

```bash
go install github.com/banerjs/cairn/cmd/cairn@latest
# or from clone:
go build -o cairn ./cmd/cairn
# or (trimmed release-style binary for this machine):
make build   # writes ./bin/cairn
```

Makefile targets: `make fmt`, `make lint`, `make test`, `make integration` (Docker), `make build` (native binary in `./bin/cairn`), `make build-all`, `make terraform-lint` (Terraform fmt-check + validate under `infra/terraform/`), `make hooks-install`, `make hooks-run`.

## Pre-commit hooks

This repo uses [pre-commit](https://pre-commit.com/) for local quality gates aligned with CI.
Recommended tool manager: [`uv`](https://docs.astral.sh/uv/).

```bash
uv tool install --upgrade pre-commit
make hooks-install
```

Run manually at any time:

```bash
make hooks-run
```

Enabled hooks:

- `gofmt` check (fails if formatting drift exists; run `make fmt`)
- `golangci-lint`
- Terraform checks (`terraform fmt`, `terraform validate`, `tflint`)
- shell checks (`shfmt` check + `shellcheck`)
- markdown checks (`markdownlint` + `mdformat`)
- basic text hygiene (trailing whitespace, EOF newline)

## Quick start

1. **Create an age identity** (PQ hybrid):

   ```bash
   cairn keygen --output ~/.config/cairn/identity.age
   ```

   Note the printed `age1pq1...` recipient.

1. **Create AWS bucket + IAM** — see [infra/README.md](infra/README.md) and Terraform under `infra/terraform/`.

1. **Write `config.toml`** — start from [examples/config.toml](examples/config.toml).

1. **Backup**

   ```bash
   cairn backup /path/to/config.toml
   ```

1. **List / restore**

   ```bash
   cairn snapshots --config /path/to/config.toml
   cairn restore SNAPSHOT_ID --target /restore/here --config /path/to/config.toml
   ```

## Secrets & configuration rules

| Secret | How it is supplied |
|--------|---------------------|
| Age identity | `CAIRN_IDENTITY_FILE` **or** `[encryption].identity_file` — **never** a CLI flag |
| AWS credentials | Standard SDK chain (`AWS_ACCESS_KEY_ID`, `AWS_SECRET_ACCESS_KEY`, profiles, SSO, IMDS) |
| Passphrase identities | Interactive prompt, or `CAIRN_PASSPHRASE` for automation |

Logs default to **warn**. `-v` / `-vv` raise to info / debug. Avoid posting debug logs publicly: they may include paths.

## Commands

```
cairn backup <config.toml> [--storage-class CLASS] [--parallelism N] [-v|-vv]
cairn restore <snapshot-id> --target DIR [--config PATH] [--parallelism N] [-v|-vv]
cairn snapshots [--host HOST] [--config PATH] [-v|-vv]
cairn verify <snapshot-id> [--sample N] [--config PATH] [-v|-vv]
cairn prune --keep-last N [--keep-monthly M] [--dry-run] [--config PATH] [-v|-vv]
cairn status [--host HOST] [--show-cost] [--config PATH] [-v|-vv]
cairn export-recovery-kit --output DIR [--config PATH]
cairn keygen --output PATH
cairn version
```

- **`verify`** — default `--sample 10`; `--sample 0` hashes every regular file.
- **`export-recovery-kit`** — writes `FORMAT.md`, `RESTORE.txt`, and optional `recipients_public.txt` (no secrets). Files are mode **0600**; widen with `chmod` if you copy them to removable media.

## Scheduling (no daemon)

Examples only — adapt paths:

```cron
# cron (daily 02:30)
30 2 * * * env AWS_PROFILE=backup CAIRN_IDENTITY_FILE=/home/you/.config/cairn/identity.age /usr/local/bin/cairn backup /home/you/.config/cairn/config.toml
```

```xml
<!-- launchd LaunchAgent (outline): ProgramArguments + WorkingDirectory + EnvironmentVariables -->
```

Windows Task Scheduler: run `cairn.exe backup C:\Users\you\AppData\Roaming\cairn\config.toml` with AWS env vars set on the task.

## Threat model (concise)

**Goals**

- **Confidentiality:** Objects on S3 remain unreadable without an age identity matching configured recipients.
- **Integrity:** Restore and `verify` check `sha256_plain` after decrypt; tampered ciphertext fails verification.
- **Authenticity of snapshots:** Committed snapshots are those with a written `manifest.age`; partial prefixes can exist transiently and are GC’d after `cleanup_grace`.

**Non-goals / limits**

- **Bucket operator / AWS admin** can delete or replace objects, enumerate approximate sizes and storage classes, and infer **host id** and **snapshot timestamps** from key names. They cannot decrypt without age keys.
- **Compromised backup host** during a run could refuse to back up, corrupt local sources before read, or exfiltrate plaintext — backups do not prevent live host compromise.
- **Metadata leakage:** If `host_id` is omitted, it is derived from the OS hostname by slugifying to the allowed id format; an explicit `host_id` is validated as-is. Snapshot IDs are time-sortable.
- **No ransomware-only rollback guarantee** without **S3 versioning** + lifecycle discipline (enabled in reference Terraform). Versioning helps recover from malicious deletes/overwrites if detected in time.

**Operational mitigations**

- MFA + least-privilege IAM scoped to `cairn/v1/*` (see Terraform).
- Offline copy of age identity + periodic **`export-recovery-kit`** stored safely.
- Periodic `verify` with `--sample 0` on critical snapshots.

## Docs

- [docs/design.md](docs/design.md) — design snapshot
- [docs/dependency-audit.md](docs/dependency-audit.md) — third-party rationale

## License

[MIT](LICENSE)

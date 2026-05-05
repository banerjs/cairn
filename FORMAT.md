# Cairn on-disk & S3 format (v1)

The authoritative specification text is embedded in releases and maintained in-repo as **[format/FORMAT.md](format/FORMAT.md)** (package `github.com/banerjs/cairn/format`).

For offline recovery kits, run:

```bash
cairn export-recovery-kit --output ./recovery-kit [--config /path/to/config.toml]
```

which writes `FORMAT.md` plus recovery hints.

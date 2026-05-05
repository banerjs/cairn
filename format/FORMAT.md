# Cairn format specification — bucket layout & envelopes (v1)

This document is **normative** for readers independent of the Go implementation.  
Two version axes:

- **`cairn/v1/`** — S3 **bucket-layout** version (where keys live).
- **`cairn.manifest.v1`**, **`cairn.index.v1`** — **JSON schema** versions inside encrypted documents.

They bump independently.

---

## 1. S3 key layout

All objects live under the fixed prefix **`cairn/v1/`**.

```
s3://<bucket>/cairn/v1/
  hosts/
    <host-id>/
      snapshots/
        <snapshot-id>/
          manifest.age          # written LAST; presence == committed snapshot
          objects/
            <object-id>         # ciphertext payload for one file (see §3)
      index.age                 # optional non-authoritative cache
```

### Identifiers

| Field | Rule |
|-------|------|
| `<host-id>` | Operator-chosen; MUST match `^[a-z0-9._-]{1,64}$`. |
| `<snapshot-id>` | `YYYYMMDDTHHMMSSZ-` + 8 lowercase hex chars (UTC). Sortable, collision-resistant. |
| `<object-id>` | UUIDv4 string. Opaque random ID — MUST NOT encode paths or hashes (listing keys must not leak structure). |

### Partial snapshots

If `objects/` entries exist but **`manifest.age` is absent**, the snapshot is **uncommitted**. Implementations may garbage-collect such prefixes after a configurable grace period.

---

## 2. Manifest (`cairn.manifest.v1`)

Plaintext is a single JSON object. Required top-level fields match the implementation schema version `cairn.manifest.v1`.

### Path rules

- `path` in `files` / `directories` is **relative** to a `source_root`, uses **forward slashes**, no leading slash, no `.` or `..` segments.
- Windows backups MUST normalize to forward slashes before serialization; restore maps back to OS separators.

### File entries

- `type` is one of: `regular`, `symlink`, `directory` (symlink-as-entry; directories also appear under `directories`).
- `sha256_plain` — SHA-256 of **plaintext** before compression (what verification compares).
- `size_plain` / `size_object` — logical size vs ciphertext object size on S3.
- `mode`, `uid`, `gid` — optional; omitted on Windows; restore uses safe defaults when missing.

### Encryption metadata (informational)

`encryption.recipient_type` for v1 PQ hybrid is documented as **`mlkem768x25519`**. The armored age header is authoritative.

### At-rest encoding for `manifest.age` (and `index.age`)

**Pipeline:** `JSON UTF-8 → zstd → age` (binary ciphertext).

- **Manifest / index envelope:** zstd level is implementation-defined but fixed per schema (readers MUST support default zstd frames).
- **Data objects** (`objects/<uuid>`): same envelope; the manifest’s `compression` block describes the data-object zstd settings used at backup time.

Readers MUST refuse unknown **major** schema versions and SHOULD ignore unknown JSON fields within a known major.

---

## 3. Data objects (`objects/<object-id>`)

Each object stores **one** logical file (v1; no deduplication).

**Pipeline:** `plaintext file → SHA-256 (for manifest) → zstd → age → PUT S3`

The blob on S3 is opaque ciphertext. Decryption order: **age decrypt → zstd decompress → plaintext**.

---

## 4. Index (`cairn.index.v1`)

Optional JSON cache derived from listing `manifest.age` keys. **Not authoritative.** Fields include `host_id`, `updated_at`, and a `snapshots` array with summary metadata per snapshot.

Encoded like the manifest: **`json → zstd → age`** as `index.age`.

---

## 5. Cryptography expectations

- **age** recipients MUST be post-quantum hybrid (**`age1pq1...`** / **`age1tagpq1...`**) for v1 backups; classical X25519-only `age1...` is out of scope for v1 writers.
- **Identity material** (`AGE-SECRET-KEY-PQ-1...`) MUST be protected at least as well as the bucket credentials; loss equals loss of decrypt capability.

---

## 6. Recovery procedure (high level)

1. Obtain bucket credentials with read access to `cairn/v1/`.
2. List `cairn/v1/hosts/<host-id>/snapshots/*/manifest.age`.
3. Download chosen `manifest.age`, decrypt with age identity, decompress zstd, parse JSON.
4. For each `regular` file entry, download `objects/<object_id>`, decrypt/decompress, verify `sha256_plain`.
5. Recreate directories and symlinks per manifest metadata.

---

## 7. Forward compatibility (informative)

Future manifest schemas (e.g. v2 deduplication) may introduce shared blob prefixes **without** changing the `cairn/v1/` layout prefix; snapshot directories may reference shared blobs by new ID fields. v1 snapshots remain readable indefinitely.

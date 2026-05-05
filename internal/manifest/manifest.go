// Package manifest defines versioned JSON documents stored as json→zstd→age blobs.
package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const SchemaV1 = "cairn.manifest.v1"

// Manifest is the plaintext snapshot metadata (before envelope compression/encryption).
type Manifest struct {
	Schema       string          `json:"schema"`
	SnapshotID   string          `json:"snapshot_id"`
	HostID       string          `json:"host_id"`
	HostOS       string          `json:"host_os"`
	CreatedAt    string          `json:"created_at"`
	CompletedAt  string          `json:"completed_at"`
	Tool         ToolInfo        `json:"tool"`
	Compression  CompressionInfo `json:"compression"`
	Encryption   EncryptionInfo  `json:"encryption"`
	SourceRoots  []string        `json:"source_roots"`
	StorageClass string          `json:"storage_class"`
	Files        []FileEntry     `json:"files"`
	Directories  []DirEntry      `json:"directories"`
	Stats        Stats           `json:"stats"`
}

// ToolInfo identifies the backup software writing the manifest.
type ToolInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// CompressionInfo describes how file payloads under objects/ were compressed.
type CompressionInfo struct {
	Algorithm string `json:"algorithm"`
	Level     int    `json:"level"`
}

// EncryptionInfo records recipients used for age encryption (informational).
type EncryptionInfo struct {
	Algorithm     string   `json:"algorithm"`
	RecipientType string   `json:"recipient_type"`
	Recipients    []string `json:"recipients"`
}

// FileEntry is one backed-up file or symlink.
type FileEntry struct {
	Path          string  `json:"path"`
	Type          string  `json:"type"`
	ObjectID      string  `json:"object_id,omitempty"`
	SizePlain     int64   `json:"size_plain"`
	SizeObject    int64   `json:"size_object"`
	SHA256Plain   string  `json:"sha256_plain,omitempty"`
	Mode          *uint32 `json:"mode,omitempty"`
	UID           *int64  `json:"uid,omitempty"`
	GID           *int64  `json:"gid,omitempty"`
	MtimeNs       int64   `json:"mtime_ns"`
	SymlinkTarget *string `json:"symlink_target,omitempty"`
}

// DirEntry records directory metadata for restore.
type DirEntry struct {
	Path    string  `json:"path"`
	Mode    *uint32 `json:"mode,omitempty"`
	UID     *int64  `json:"uid,omitempty"`
	GID     *int64  `json:"gid,omitempty"`
	MtimeNs int64   `json:"mtime_ns"`
}

// Stats aggregates snapshot totals.
type Stats struct {
	FilesTotal       int   `json:"files_total"`
	BytesPlainTotal  int64 `json:"bytes_plain_total"`
	BytesObjectTotal int64 `json:"bytes_object_total"`
}

// ValidateSchema returns an error if the manifest schema is not supported.
func ValidateSchema(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest: nil manifest")
	}
	if m.Schema != SchemaV1 {
		return fmt.Errorf("manifest: unsupported schema %q (want %s)", m.Schema, SchemaV1)
	}
	return nil
}

// MarshalJSONForManifest produces canonical JSON bytes for the manifest.
func MarshalJSONForManifest(m *Manifest) ([]byte, error) {
	return json.MarshalIndent(m, "", "  ")
}

// UnmarshalManifestJSON parses manifest JSON and validates the schema major.
// Unknown JSON fields are ignored per format spec.
func UnmarshalManifestJSON(data []byte) (*Manifest, error) {
	var m Manifest
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("manifest: decode: %w", err)
	}
	if err := ValidateSchema(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

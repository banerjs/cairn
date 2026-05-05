package manifest

import (
	"bytes"
	"encoding/json"
	"fmt"
)

const IndexSchemaV1 = "cairn.index.v1"

// Index is the optional encrypted snapshot listing cache for a host.
type Index struct {
	Schema    string      `json:"schema"`
	HostID    string      `json:"host_id"`
	UpdatedAt string      `json:"updated_at"`
	Snapshots []IndexSnap `json:"snapshots"`
}

// IndexSnap is one row in the index cache.
type IndexSnap struct {
	SnapshotID       string `json:"snapshot_id"`
	CreatedAt        string `json:"created_at"`
	FilesTotal       int    `json:"files_total"`
	BytesObjectTotal int64  `json:"bytes_object_total"`
	StorageClass     string `json:"storage_class"`
}

// ValidateIndexSchema checks index schema version.
func ValidateIndexSchema(ix *Index) error {
	if ix == nil {
		return fmt.Errorf("index: nil")
	}
	if ix.Schema != IndexSchemaV1 {
		return fmt.Errorf("index: unsupported schema %q", ix.Schema)
	}
	return nil
}

// MarshalIndexJSON serializes an index document.
func MarshalIndexJSON(ix *Index) ([]byte, error) {
	return json.MarshalIndent(ix, "", "  ")
}

// UnmarshalIndexJSON parses index JSON.
func UnmarshalIndexJSON(data []byte) (*Index, error) {
	var ix Index
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&ix); err != nil {
		return nil, fmt.Errorf("index: decode: %w", err)
	}
	if err := ValidateIndexSchema(&ix); err != nil {
		return nil, err
	}
	return &ix, nil
}

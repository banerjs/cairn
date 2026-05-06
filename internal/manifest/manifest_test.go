package manifest

import (
	"strings"
	"testing"

	"filippo.io/age"

	"github.com/banerjs/cairn/internal/envelope"
)

func TestValidateSchema(t *testing.T) {
	if err := ValidateSchema(nil); err == nil {
		t.Fatal("expected nil manifest error")
	}
	if err := ValidateSchema(&Manifest{Schema: "other"}); err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("got %v", err)
	}
	if err := ValidateSchema(&Manifest{Schema: SchemaV1}); err != nil {
		t.Fatal(err)
	}
}

func TestUnmarshalManifestJSON_Errors(t *testing.T) {
	if _, err := UnmarshalManifestJSON([]byte(`not json`)); err == nil {
		t.Fatal("expected decode error")
	}
	if _, err := UnmarshalManifestJSON([]byte(`{"schema":"wrong.v1","snapshot_id":"s"}`)); err == nil {
		t.Fatal("expected schema error")
	}
}

func TestManifestRoundTrip_Encrypted(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec := id.Recipient()
	m := &Manifest{
		Schema:      SchemaV1,
		SnapshotID:  "20990101T000000Z-aaaaaaaa",
		HostID:      "test-host",
		HostOS:      "linux",
		CreatedAt:   "2099-01-01T00:00:00Z",
		CompletedAt: "2099-01-01T00:00:01Z",
		Tool:        ToolInfo{Name: "cairn", Version: "test"},
		Compression: CompressionInfo{Algorithm: "zstd", Level: 3},
		Encryption: EncryptionInfo{
			Algorithm:     "age",
			RecipientType: "mlkem768x25519",
			Recipients:    []string{rec.String()},
		},
		SourceRoots:  []string{"/tmp"},
		StorageClass: "STANDARD",
		Files: []FileEntry{
			{Path: "a.txt", Type: "regular", ObjectID: "u1", SizePlain: 1, SizeObject: 10, SHA256Plain: "ab"},
		},
		Directories: []DirEntry{},
		Stats:       Stats{FilesTotal: 1, BytesPlainTotal: 1, BytesObjectTotal: 10},
	}
	raw, err := MarshalJSONForManifest(m)
	if err != nil {
		t.Fatal(err)
	}
	blob, err := envelope.Encrypt(raw, []age.Recipient{rec})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := envelope.Decrypt(blob, []age.Identity{id})
	if err != nil {
		t.Fatal(err)
	}
	out, err := UnmarshalManifestJSON(plain)
	if err != nil {
		t.Fatal(err)
	}
	if out.SnapshotID != m.SnapshotID || len(out.Files) != 1 {
		t.Fatalf("unexpected %+v", out)
	}
}

func TestIndexRoundTripAndValidation(t *testing.T) {
	if err := ValidateIndexSchema(nil); err == nil {
		t.Fatal("expected nil error")
	}
	if err := ValidateIndexSchema(&Index{Schema: "x"}); err == nil || !strings.Contains(err.Error(), "unsupported schema") {
		t.Fatalf("got %v", err)
	}

	ix := &Index{
		Schema:    IndexSchemaV1,
		HostID:    "h",
		UpdatedAt: "2026-01-01T00:00:00Z",
		Snapshots: []IndexSnap{{SnapshotID: "20260101T000000Z-a", CreatedAt: "2026-01-01T00:00:00Z"}},
	}
	raw, err := MarshalIndexJSON(ix)
	if err != nil {
		t.Fatal(err)
	}
	out, err := UnmarshalIndexJSON(raw)
	if err != nil || out.HostID != "h" || len(out.Snapshots) != 1 {
		t.Fatalf("%v %+v", err, out)
	}
	if _, err := UnmarshalIndexJSON([]byte(`oops`)); err == nil {
		t.Fatal("expected decode error")
	}
	if _, err := UnmarshalIndexJSON([]byte(`{"schema":"bad"}`)); err == nil {
		t.Fatal("expected schema validation error")
	}
}

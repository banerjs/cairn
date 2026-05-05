package manifest

import (
	"testing"

	"filippo.io/age"

	"github.com/banerjs/cairn/internal/envelope"
)

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

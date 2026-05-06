package backup

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3test"

	"filippo.io/age"
)

func TestRebuildIndex(t *testing.T) {
	id, err := age.GenerateHybridIdentity()
	if err != nil {
		t.Fatal(err)
	}
	rec, err := age.ParseHybridRecipient(id.Recipient().String())
	if err != nil {
		t.Fatal(err)
	}
	recipients := []age.Recipient{rec}

	st, cleanup := s3test.NewStore(t, "bucket-idx")
	defer cleanup()
	ctx := context.Background()

	host := "idx-host"
	snapKey := "cairn/v1/hosts/" + host + "/snapshots/20200101T000000Z-deadbeef/manifest.age"
	if err := st.PutObject(ctx, snapKey, strings.NewReader("blob"), "STANDARD"); err != nil {
		t.Fatal(err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := RebuildIndex(ctx, st, host, recipients, "STANDARD_IA", log); err != nil {
		t.Fatal(err)
	}

	rc, err := st.GetObject(ctx, paths.IndexKey(host))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(rc)
	if err != nil || len(b) < 10 {
		t.Fatalf("index object: %v len %d", err, len(b))
	}
}

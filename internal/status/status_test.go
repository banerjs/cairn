package status

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3test"
)

func TestRun_Basic(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "status-buck")
	defer cleanup()
	ctx := context.Background()
	k := paths.ManifestKey("sh", "20200101T000000Z-cccccccc")
	if err := st.PutObject(ctx, k, strings.NewReader("m"), "STANDARD"); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, "", false, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_WithCost(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "status-buck2")
	defer cleanup()
	ctx := context.Background()
	k := paths.ManifestKey("sh2", "20200102T000000Z-dddddddd")
	if err := st.PutObject(ctx, k, strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, "", true, log); err != nil {
		t.Fatal(err)
	}
}

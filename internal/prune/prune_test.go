package prune

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3test"
)

func TestRun_DryRun(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "prune-bucket")
	defer cleanup()
	ctx := context.Background()
	host := "ph"
	old := "20200101T000000Z-aaaaaaaa"
	newer := "20250101T000000Z-bbbbbbbb"
	for _, sid := range []string{old, newer} {
		k := paths.ManifestKey(host, sid)
		if err := st.PutObject(ctx, k, strings.NewReader("x"), ""); err != nil {
			t.Fatal(err)
		}
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(ctx, st, host, 1, 0, true, log); err != nil {
		t.Fatal(err)
	}
}

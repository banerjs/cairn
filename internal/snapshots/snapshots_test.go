package snapshots

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"

	appcfg "github.com/banerjs/cairn/internal/config"
	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3test"
)

func TestList_FromManifests(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "snap-bucket")
	defer cleanup()
	ctx := context.Background()
	host := "listh"
	sid := "20200101T000000Z-aaaaaa01"
	k := paths.ManifestKey(host, sid)
	if err := st.PutObject(ctx, k, strings.NewReader("m"), ""); err != nil {
		t.Fatal(err)
	}
	cfg := &appcfg.Config{HostID: host}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := List(ctx, cfg, st, nil, "", log); err != nil {
		t.Fatal(err)
	}
}

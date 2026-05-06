package prune

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/s3store"
)

type errOnListPrefix struct{}

func (errOnListPrefix) ListPrefix(context.Context, string) ([]s3store.ListedObject, error) {
	return nil, errors.New("list root")
}

func (errOnListPrefix) DeleteObject(context.Context, string) error {
	return errors.New("no delete expected")
}

func TestRun_ListRootError(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), errOnListPrefix{}, "h", nil, 1, 0, false, log); err == nil {
		t.Fatal("expected error")
	}
}

// failSecondSnapList errors when listing …/snapshots/<oldSid>/ subtree.
type failSecondSnapList struct {
	host      string
	oldSnap   string
	newSnap   string
	listCalls int
}

func (s *failSecondSnapList) ListPrefix(_ context.Context, prefix string) ([]s3store.ListedObject, error) {
	s.listCalls++
	root := paths.SnapshotsListPrefix(s.host)
	if prefix == root {
		return []s3store.ListedObject{
			{Key: paths.ManifestKey(s.host, s.oldSnap)},
			{Key: paths.ManifestKey(s.host, s.newSnap)},
		}, nil
	}
	if strings.HasPrefix(prefix, paths.SnapshotPrefix(s.host, s.oldSnap)) {
		return nil, errors.New("list snapshot subtree")
	}
	return []s3store.ListedObject{}, nil
}

func (s *failSecondSnapList) DeleteObject(context.Context, string) error {
	return errors.New("no delete expected")
}

func TestRun_ListSnapshotSubtreeError(t *testing.T) {
	st := &failSecondSnapList{
		host:    "eh",
		oldSnap: "20200101T000000Z-aaaaaaaa",
		newSnap: "20250101T000000Z-bbbbbbbb",
	}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), st, st.host, nil, 1, 0, false, log); err == nil {
		t.Fatal("expected error")
	}
}

type failDelete struct {
	host string
	sid  string
}

func (s *failDelete) ListPrefix(_ context.Context, prefix string) ([]s3store.ListedObject, error) {
	root := paths.SnapshotsListPrefix(s.host)
	if prefix == root {
		return []s3store.ListedObject{{Key: paths.ManifestKey(s.host, s.sid)}}, nil
	}
	return []s3store.ListedObject{{Key: paths.SnapshotPrefix(s.host, s.sid) + "objects/o1"}}, nil
}

func (failDelete) DeleteObject(context.Context, string) error {
	return errors.New("delete failed")
}

func TestRun_DeleteError(t *testing.T) {
	st := &failDelete{host: "dh", sid: "20000101T000000Z-cccccccc"}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), st, st.host, []string{st.sid}, 0, 0, false, log); err == nil {
		t.Fatal("expected error")
	}
}

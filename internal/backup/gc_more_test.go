package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/banerjs/cairn/internal/s3store"
)

type errPartial struct {
	memPartial
	listErr error
	delErr  error
}

func (e *errPartial) ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error) {
	if e.listErr != nil {
		return nil, e.listErr
	}
	return e.memPartial.ListPrefix(ctx, prefix)
}

func (e *errPartial) DeleteObject(ctx context.Context, key string) error {
	if e.delErr != nil {
		return e.delErr
	}
	return e.memPartial.DeleteObject(ctx, key)
}

func TestCleanupPartialSnapshots_ErrorBranches(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := CleanupPartialSnapshots(ctx, &errPartial{listErr: errors.New("list")}, "h", time.Hour, log); err == nil {
		t.Fatal("expected list error")
	}

	malformed := &errPartial{memPartial: memPartial{keys: []string{
		"cairn/v1/hosts/h/snapshots//manifest.age",
		"cairn/v1/hosts/h/snapshots/not-a-sid/objects/u1",
	}}}
	if err := CleanupPartialSnapshots(ctx, malformed, "h", time.Hour, log); err != nil {
		t.Fatal(err)
	}

	withManifest := &errPartial{memPartial: memPartial{keys: []string{
		"cairn/v1/hosts/h/snapshots/20000101T000000Z-deadbeef/manifest.age",
		"cairn/v1/hosts/h/snapshots/20000101T000000Z-deadbeef/objects/u1",
	}}, delErr: errors.New("del")}
	if err := CleanupPartialSnapshots(ctx, withManifest, "h", time.Hour, log); err != nil {
		t.Fatal(err)
	}

	deleteErr := &errPartial{memPartial: memPartial{keys: []string{
		"cairn/v1/hosts/h/snapshots/20000101T000000Z-feedbeef/objects/u2",
	}}, delErr: errors.New("del")}
	if err := CleanupPartialSnapshots(ctx, deleteErr, "h", time.Hour, log); err == nil {
		t.Fatal("expected delete error")
	}
}

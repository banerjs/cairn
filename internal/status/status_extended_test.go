package status

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/banerjs/cairn/internal/s3store"
)

type statusListStub struct {
	objs []s3store.ListedObject
	err  error
}

func (s *statusListStub) ListPrefix(context.Context, string) ([]s3store.ListedObject, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.objs, nil
}

func TestRun_ListError(t *testing.T) {
	st := &statusListStub{err: errors.New("boom")}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), st, "", false, log); err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_HostFilterAndCost(t *testing.T) {
	glacier := types.ObjectStorageClassGlacier
	st := &statusListStub{objs: []s3store.ListedObject{
		{Key: "cairn/v1/hosts/a/snapshots/20250101T000000Z-aaaaaaaa/manifest.age", Size: 100, StorageClass: glacier},
		{Key: "cairn/v1/hosts/b/snapshots/20250102T000000Z-bbbbbbbb/manifest.age", Size: 200},
		{Key: "cairn/v1/hosts/a/other", Size: 50},
		{Key: "weird", Size: 9},
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), st, "a", true, log); err != nil {
		t.Fatal(err)
	}
}

func TestRun_ManifestBadSnapshotIDSkipped(t *testing.T) {
	st := &statusListStub{objs: []s3store.ListedObject{
		{Key: "cairn/v1/hosts/hh/snapshots//manifest.age", Size: 1},
	}}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := Run(context.Background(), st, "", false, log); err != nil {
		t.Fatal(err)
	}
}

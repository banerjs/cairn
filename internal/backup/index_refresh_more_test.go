package backup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/banerjs/cairn/internal/envelope"
	"github.com/banerjs/cairn/internal/manifest"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/s3test"

	"filippo.io/age"
)

func TestRebuildIndex_ErrorAndEdgeBranches(t *testing.T) {
	prevParse := parseSnapshotTimeFn
	prevMarshal := marshalIndexFn
	prevEncrypt := encryptIndexFn
	prevNow := nowUTCIndexFn
	t.Cleanup(func() {
		parseSnapshotTimeFn = prevParse
		marshalIndexFn = prevMarshal
		encryptIndexFn = prevEncrypt
		nowUTCIndexFn = prevNow
	})

	ctx := context.Background()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := aws.Config{
		Region: "us-east-1",
		HTTPClient: &http.Client{
			Transport: rtFunc(func(*http.Request) (*http.Response, error) { return nil, context.DeadlineExceeded }),
		},
	}
	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://127.0.0.1:2")
		o.UsePathStyle = true
	})
	bad := s3store.NewWithClient(cl, "b")
	if err := RebuildIndex(ctx, bad, "h", nil, "STANDARD", log); err == nil {
		t.Fatal("expected list error")
	}

	id, _ := age.GenerateHybridIdentity()
	rec, _ := age.ParseHybridRecipient(id.Recipient().String())
	recips := []age.Recipient{rec}
	st, cleanup := s3test.NewStore(t, "idx-more")
	defer cleanup()
	host := "h"
	keys := []string{
		"cairn/v1/hosts/h/snapshots/20200101T000000Z-deadbeef/manifest.age",
		"cairn/v1/hosts/h/snapshots/20200101T000000Z-deadbeef/extra/manifest.age", // duplicate sid
		"cairn/v1/hosts/h/snapshots//manifest.age",                                // sid empty
		"cairn/v1/hosts/h/snapshots/not-a-sid/manifest.age",                       // parse warning
		"cairn/v1/hosts/h/snapshots/20200101T000001Z-feedbeef/manifest.age",
		"cairn/v1/hosts/h/snapshots/20200101T000002Z-cafebabe/other", // non-manifest
	}
	for _, k := range keys {
		if err := st.PutObject(ctx, k, strings.NewReader("x"), "STANDARD"); err != nil {
			t.Fatal(err)
		}
	}
	if err := RebuildIndex(ctx, st, host, recips, "STANDARD_IA", log); err != nil {
		t.Fatal(err)
	}
	if err := RebuildIndex(ctx, st, host, nil, "STANDARD_IA", log); err == nil {
		t.Fatal("expected encrypt error with no recipients")
	}

	marshalIndexFn = func(*manifest.Index) ([]byte, error) { return nil, errors.New("marshal") }
	if err := RebuildIndex(ctx, st, host, recips, "STANDARD_IA", log); err == nil {
		t.Fatal("expected marshal error")
	}
	marshalIndexFn = manifest.MarshalIndexJSON

	encryptIndexFn = func([]byte, []age.Recipient) ([]byte, error) { return nil, errors.New("enc") }
	if err := RebuildIndex(ctx, st, host, recips, "STANDARD_IA", log); err == nil {
		t.Fatal("expected encrypt error")
	}
	encryptIndexFn = envelope.Encrypt

	nowUTCIndexFn = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC) }
	if err := RebuildIndex(ctx, st, host, recips, "STANDARD_IA", log); err != nil {
		t.Fatal(err)
	}
}

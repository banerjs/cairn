package s3store_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/banerjs/cairn/internal/s3test"
)

func TestNew_EmptyBucket(t *testing.T) {
	if _, err := s3store.New(context.Background(), aws.Config{}, ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestRoundTrip(t *testing.T) {
	st, cleanup := s3test.NewStore(t, "buck")
	defer cleanup()
	ctx := context.Background()
	key := "cairn/v1/hosts/h/snapshots/s/obj"
	body := []byte("cipher")
	if err := st.PutObject(ctx, key, bytes.NewReader(body), "STANDARD"); err != nil {
		t.Fatal(err)
	}
	rc, err := st.GetObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	defer rc.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, rc); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), body) {
		t.Fatalf("got %q", buf.Bytes())
	}
	objs, err := st.ListPrefix(ctx, "cairn/v1/")
	if err != nil || len(objs) != 1 {
		t.Fatalf("list: %v len %d", err, len(objs))
	}
	if err := st.DeleteObject(ctx, key); err != nil {
		t.Fatal(err)
	}
}

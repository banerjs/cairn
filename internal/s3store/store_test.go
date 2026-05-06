package s3store_test

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
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
	defer func() { _ = rc.Close() }()
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

func TestNew_NonEmptyBucketSuccess(t *testing.T) {
	if _, err := s3store.New(context.Background(), aws.Config{}, "ok-bucket"); err != nil {
		t.Fatal(err)
	}
}

func TestTransportErrors(t *testing.T) {
	cfg := aws.Config{
		Region: "us-east-1",
		HTTPClient: &http.Client{
			Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				return nil, context.DeadlineExceeded
			}),
		},
	}
	cl := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://127.0.0.1:2")
		o.UsePathStyle = true
	})
	st := s3store.NewWithClient(cl, "b")
	ctx := context.Background()

	if err := st.PutObject(ctx, "k", bytes.NewReader([]byte("x")), "STANDARD"); err == nil {
		t.Fatal("expected put error")
	}
	if _, err := st.GetObject(ctx, "k"); err == nil {
		t.Fatal("expected get error")
	}
	if err := st.DeleteObject(ctx, "k"); err == nil {
		t.Fatal("expected delete error")
	}
	if _, err := st.ListPrefix(ctx, "p/"); err == nil {
		t.Fatal("expected list error")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

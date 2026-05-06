package s3test

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/banerjs/cairn/internal/s3store"
)

func TestTryNewStoreRoundTrip(t *testing.T) {
	st, cleanup, err := TryNewStore("try-bucket")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	objs, err := st.ListPrefix(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = objs
}

func TestNewStore(t *testing.T) {
	st, cleanup := NewStore(t, "test-bucket")
	defer cleanup()
	objs, err := st.ListPrefix(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = objs
}

func TestNewBackedStore_CreateBucketError(t *testing.T) {
	_, cleanup, err := newBackedStore("any-bucket", func(o *s3.Options) {
		// Nothing is listening — CreateBucket fails.
		o.BaseEndpoint = aws.String("http://127.0.0.1:2")
	})
	if cleanup != nil {
		cleanup()
	}
	if err == nil {
		t.Fatal("expected CreateBucket error")
	}
}

func TestNewStore_PanicsOnTryNewStoreError(t *testing.T) {
	prev := tryNewStore
	tryNewStore = func(string, ...func(*s3.Options)) (*s3store.Store, func(), error) {
		return nil, nil, errors.New("boom")
	}
	defer func() { tryNewStore = prev }()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	NewStore(t, "b")
}

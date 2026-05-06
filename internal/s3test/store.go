// Package s3test provides an in-memory S3 fake for unit tests (not used by production code).
package s3test

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

// tryNewStore is swapped in tests (see s3test_test.go).
var tryNewStore = newBackedStore

// newBackedStore starts an in-memory faker and optionally applies extra S3 client options.
func newBackedStore(bucket string, extraClientOpts ...func(*s3.Options)) (*s3store.Store, func(), error) {
	backend := s3mem.New()
	faker := gofakes3.New(backend)
	ts := httptest.NewServer(faker.Server())
	cleanup := func() { ts.Close() }

	cfg := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(
			"AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "",
		),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(ts.URL)
		o.UsePathStyle = true
		for _, fn := range extraClientOpts {
			if fn != nil {
				fn(o)
			}
		}
	})
	ctx := context.Background()
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	return s3store.NewWithClient(client, bucket), cleanup, nil
}

// TryNewStore is like NewStore but returns an error if the bucket cannot be created.
func TryNewStore(bucket string, extraClientOpts ...func(*s3.Options)) (*s3store.Store, func(), error) {
	return newBackedStore(bucket, extraClientOpts...)
}

// NewStore returns a Store backed by an in-memory S3 server. Call cleanup() when done.
func NewStore(t *testing.T, bucket string) (st *s3store.Store, cleanup func()) {
	t.Helper()
	st, cleanup, err := tryNewStore(bucket)
	if err != nil {
		panic(fmt.Errorf("s3test NewStore: CreateBucket: %w", err))
	}
	return st, cleanup
}

// Package s3test provides an in-memory S3 fake for unit tests (not used by production code).
package s3test

import (
	"context"
	"net/http/httptest"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/banerjs/cairn/internal/s3store"
	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"
)

// NewStore returns a Store backed by an in-memory S3 server. Call cleanup() when done.
func NewStore(t *testing.T, bucket string) (st *s3store.Store, cleanup func()) {
	t.Helper()
	backend := s3mem.New()
	faker := gofakes3.New(backend)
	ts := httptest.NewServer(faker.Server())

	cfg := aws.Config{
		Region: "us-east-1",
		Credentials: credentials.NewStaticCredentialsProvider(
			"AKIAIOSFODNN7EXAMPLE", "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY", "",
		),
		EndpointResolverWithOptions: aws.EndpointResolverWithOptionsFunc(
			func(service, region string, opts ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{URL: ts.URL}, nil
			},
		),
	}
	client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})
	ctx := context.Background()
	_, err := client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(bucket)})
	if err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	return s3store.NewWithClient(client, bucket), func() { ts.Close() }
}

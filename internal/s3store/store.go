// Package s3store wraps AWS S3 Put/Get/List/Delete used by cairn (object-level API only).
package s3store

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

var (
	newS3Client = s3.NewFromConfig
	newUploader = manager.NewUploader
)

// ListedObject is one row from ListObjectsV2 (latest non-delete marker version).
type ListedObject struct {
	Key          string
	Size         int64
	StorageClass types.ObjectStorageClass
}

// Store performs bucket-scoped object operations.
type Store struct {
	bucket string
	client *s3.Client
	up     *manager.Uploader
}

// New connects to S3 in the given region and bucket.
func New(ctx context.Context, cfg aws.Config, bucket string) (*Store, error) {
	_ = ctx
	if bucket == "" {
		return nil, fmt.Errorf("s3store: empty bucket")
	}
	cl := newS3Client(cfg)
	return NewWithClient(cl, bucket), nil
}

// NewWithClient builds a Store from an existing S3 client (e.g. MinIO integration tests).
func NewWithClient(cl *s3.Client, bucket string) *Store {
	return &Store{
		bucket: bucket,
		client: cl,
		up:     newUploader(cl),
	}
}

// PutObject uploads an object with optional storage class.
//
// It assumes the caller never sends plaintext to S3; body is opaque ciphertext.
func (s *Store) PutObject(ctx context.Context, key string, body io.Reader, storageClass string) error {
	in := &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   body,
	}
	if storageClass != "" {
		in.StorageClass = types.StorageClass(storageClass)
	}
	_, err := s.up.Upload(ctx, in)
	if err != nil {
		return fmt.Errorf("s3store PutObject: %w", err)
	}
	return nil
}

// GetObject streams an object body.
func (s *Store) GetObject(ctx context.Context, key string) (io.ReadCloser, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return nil, fmt.Errorf("s3store GetObject: %w", err)
	}
	return out.Body, nil
}

// DeleteObject removes one object key.
func (s *Store) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("s3store DeleteObject: %w", err)
	}
	return nil
}

// ListPrefix lists all object keys under prefix (paginated).
func (s *Store) ListPrefix(ctx context.Context, prefix string) ([]ListedObject, error) {
	var out []ListedObject
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("s3store ListPrefix: %w", err)
		}
		for _, ent := range page.Contents {
			lo, ok := listedObjectFromS3(ent)
			if ok {
				out = append(out, lo)
			}
		}
	}
	return out, nil
}

func listedObjectFromS3(ent types.Object) (ListedObject, bool) {
	if ent.Key == nil {
		return ListedObject{}, false
	}
	lo := ListedObject{Key: *ent.Key}
	if ent.Size != nil {
		lo.Size = *ent.Size
	}
	lo.StorageClass = ent.StorageClass
	return lo, true
}

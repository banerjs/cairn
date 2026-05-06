package s3store

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

func TestListedObjectFromS3_NilKeySkipped(t *testing.T) {
	if _, ok := listedObjectFromS3(types.Object{}); ok {
		t.Fatal("expected nil key to be skipped")
	}
}

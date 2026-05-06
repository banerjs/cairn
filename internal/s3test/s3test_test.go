package s3test

import (
	"context"
	"testing"
)

func TestNewStore(t *testing.T) {
	st, cleanup := NewStore(t, "test-bucket")
	defer cleanup()
	objs, err := st.ListPrefix(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	_ = objs
}

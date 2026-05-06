package snapshotid

import (
	"errors"
	"testing"
)

func TestNew(t *testing.T) {
	s, err := New()
	if err != nil {
		t.Fatal(err)
	}
	if len(s) < 20 {
		t.Fatalf("short id: %q", s)
	}
}

func TestNew_RandFailure(t *testing.T) {
	prev := randRead
	defer func() { randRead = prev }()
	randRead = func(b []byte) (int, error) {
		return 0, errors.New("boom")
	}
	if _, err := New(); err == nil {
		t.Fatal("expected error")
	}
}

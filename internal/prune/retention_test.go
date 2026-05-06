package prune

import "testing"

func TestSelectSnapshotsToKeep_LastOnly(t *testing.T) {
	ids := []string{"20990103T000000Z-01", "20990102T000000Z-02", "20990101T000000Z-03"}
	keep := SelectSnapshotsToKeep(ids, 2, 0)
	if len(keep) != 2 {
		t.Fatalf("keep size %d", len(keep))
	}
	if _, ok := keep["20990101T000000Z-03"]; ok {
		t.Fatal("oldest should not be kept")
	}
}

func TestSelectSnapshotsToKeep_NoRules(t *testing.T) {
	ids := []string{"20990103T000000Z-01"}
	if len(SelectSnapshotsToKeep(nil, 0, 0)) != 0 || len(SelectSnapshotsToKeep([]string{}, 1, 0)) != 0 {
		t.Fatal("expected empty keep maps")
	}
	if k := SelectSnapshotsToKeep(ids, 0, 0); len(k) != 0 {
		t.Fatalf("got %+v", k)
	}
}

func TestSelectSnapshotsToKeep_KeepLastClamp(t *testing.T) {
	ids := []string{"20990101T000000Z-aa"}
	k := SelectSnapshotsToKeep(ids, 99, 0)
	if len(k) != 1 {
		t.Fatalf("%+v", k)
	}
}

func TestSelectSnapshotsToKeep_KeepMonthly_May2026(t *testing.T) {
	// Aligns with host machine date used in CI/local (snapshot-time months must intersect last N UTC months).
	ids := []string{
		"20260510T010203Z-aaaaaaaa",
		"20260415T010203Z-bbbbbbbb",
		"20260301T010203Z-cccccccc",
		"nope-hyphen-malformed",
	}
	k := SelectSnapshotsToKeep(ids, 0, 2)
	if len(k) != 2 {
		t.Fatalf("want April+May anchors, got %+v size %d", k, len(k))
	}
}

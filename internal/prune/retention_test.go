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

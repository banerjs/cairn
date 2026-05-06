package paths

import "testing"

func TestLayoutPaths(t *testing.T) {
	if g := HostsRootPrefix(); g != "cairn/v1/hosts/" {
		t.Fatalf("HostsRootPrefix: %q", g)
	}
	hp := HostPrefix("myhost")
	if want := "cairn/v1/hosts/myhost/"; hp != want {
		t.Fatalf("HostPrefix: %q want %q", hp, want)
	}
	sp := SnapshotPrefix("h", "20260101T000000Z-abc")
	if want := "cairn/v1/hosts/h/snapshots/20260101T000000Z-abc/"; sp != want {
		t.Fatalf("SnapshotPrefix: %q", sp)
	}
	mk := ManifestKey("h", "s1")
	if want := "cairn/v1/hosts/h/snapshots/s1/manifest.age"; mk != want {
		t.Fatalf("ManifestKey: %q", mk)
	}
	ok := ObjectKey("h", "s1", "obj1")
	if want := "cairn/v1/hosts/h/snapshots/s1/objects/obj1"; ok != want {
		t.Fatalf("ObjectKey: %q", ok)
	}
	if ix := IndexKey("hh"); ix != "cairn/v1/hosts/hh/index.age" {
		t.Fatalf("IndexKey: %q", ix)
	}
	if p := SnapshotsListPrefix("x"); p != "cairn/v1/hosts/x/snapshots/" {
		t.Fatalf("SnapshotsListPrefix: %q", p)
	}
}

func TestParseSnapshotIDFromManifestKey(t *testing.T) {
	k := "cairn/v1/hosts/h1/snapshots/20230101T010203Z-deadbeef/manifest.age"
	if s := ParseSnapshotIDFromManifestKey(k); s != "20230101T010203Z-deadbeef" {
		t.Fatalf("got %q", s)
	}
	if ParseSnapshotIDFromManifestKey("nope") != "" {
		t.Fatal("expected empty")
	}
}

func TestSnapshotIDFromKey(t *testing.T) {
	k := "cairn/v1/hosts/h/snapshots/my-snap/objects/o"
	if s := SnapshotIDFromKey(k); s != "my-snap" {
		t.Fatalf("got %q", s)
	}
	if SnapshotIDFromKey("no-marker-at-all") != "" {
		t.Fatal("expected empty without snapshots segment")
	}
	if SnapshotIDFromKey("no/snapshots/here") != "" || SnapshotIDFromKey("x/snapshots/only") != "" {
		t.Fatal("expected empty")
	}
	// No slash after snapshot id segment → empty.
	if SnapshotIDFromKey("pre/snapshots/onlyid") != "" {
		t.Fatal("expected empty when rest has no slash")
	}
	if SnapshotIDFromKey("pre/snapshots//tail") != "" {
		t.Fatal("expected empty for empty segment")
	}
}

func TestParseHostIDFromHostsPath(t *testing.T) {
	if h := ParseHostIDFromHostsPath("cairn/v1/hosts/my-host/foo"); h != "my-host" {
		t.Fatalf("got %q", h)
	}
	if ParseHostIDFromHostsPath("bad") != "" {
		t.Fatal("expected empty")
	}
}

func TestParseSnapshotTime(t *testing.T) {
	ts, err := ParseSnapshotTime("20260206T120000Z-deadbeef")
	if err != nil {
		t.Fatal(err)
	}
	if ts.UTC().Year() != 2026 {
		t.Fatal(ts)
	}
	for _, id := range []string{"", "nohyphen", "short-bad", "20260206120000Z-x"} {
		if _, err := ParseSnapshotTime(id); err == nil {
			t.Fatalf("expected error for %q", id)
		}
	}
	if _, err := ParseSnapshotTime("badprefix-bad"); err == nil {
		t.Fatal("expected error")
	}
}

func TestParseSnapshotTime_FormatBounds(t *testing.T) {
	ts, err := ParseSnapshotTime("20200101T000000Z-aaaaaaaa")
	if err != nil {
		t.Fatal(err)
	}
	if ts.Year() != 2020 {
		t.Fatalf("year %d", ts.Year())
	}
}


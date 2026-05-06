package main

import (
	"slices"
	"testing"
)

func TestReorderFlagsBeforePositionals_verifySnapshotFirst(t *testing.T) {
	in := []string{"20260506T044454Z-11e117af", "--config", "/x/config.toml", "-v"}
	got := reorderFlagsBeforePositionals(in, commandValuedVerifyFlags)
	want := []string{"--config", "/x/config.toml", "-v", "20260506T044454Z-11e117af"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReorderFlagsBeforePositionals_flagOrderPreservedWithinKind(t *testing.T) {
	in := []string{"snap", "-v", "--config", "/c.toml", "--sample", "0"}
	got := reorderFlagsBeforePositionals(in, commandValuedVerifyFlags)
	want := []string{"-v", "--config", "/c.toml", "--sample", "0", "snap"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReorderFlagsBeforePositionals_doubleDashRestPositional(t *testing.T) {
	in := []string{"a", "--", "-looks-like-flag"}
	got := reorderFlagsBeforePositionals(in, commandValuedVerifyFlags)
	want := []string{"a", "-looks-like-flag"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestSplitFlagToken(t *testing.T) {
	n, v, eq := splitFlagToken("--foo=bar")
	if n != "foo" || v != "bar" || !eq {
		t.Fatalf("got %q %q %v", n, v, eq)
	}
	n, v, eq = splitFlagToken("-sample=12")
	if n != "sample" || v != "12" || !eq {
		t.Fatalf("got %q %q %v", n, v, eq)
	}
	n, v, eq = splitFlagToken("-vv")
	if n != "vv" || v != "" || eq {
		t.Fatalf("got %q %q %v", n, v, eq)
	}
	for _, s := range []string{"", "x", "no-leading-dash"} {
		n, v, eq := splitFlagToken(s)
		if n != "" || v != "" || eq {
			t.Fatalf("non-flag %q parsed as %q %q %v", s, n, v, eq)
		}
	}
}

func TestReorderFlagsBeforePositionals_equalsValueNoExtraToken(t *testing.T) {
	in := []string{"snap", "--config=/c.toml", "-v"}
	got := reorderFlagsBeforePositionals(in, commandValuedVerifyFlags)
	want := []string{"--config=/c.toml", "-v", "snap"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReorderFlagsBeforePositionals_loneDashPositional(t *testing.T) {
	in := []string{"-", "-v", "id"}
	got := reorderFlagsBeforePositionals(in, commandValuedVerifyFlags)
	want := []string{"-v", "-", "id"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestReorderFlagsBeforePositionals_valuedFlagMissingValue(t *testing.T) {
	in := []string{"sid", "--config"}
	got := reorderFlagsBeforePositionals(in, commandValuedVerifyFlags)
	want := []string{"--config", "sid"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
}

package pricing

import (
	"strings"
	"testing"
)

func TestMonthlyEstimateUSD(t *testing.T) {
	v := MonthlyEstimateUSD("STANDARD", 1024*1024*1024)
	if v <= 0 {
		t.Fatal(v)
	}
	for sc := range USDPerGBMonth {
		if sc == "" {
			continue // tested via empty-ish input below
		}
		_ = MonthlyEstimateUSD(sc, 10)
	}
	// Blank / whitespace → STANDARD.
	if MonthlyEstimateUSD("   ", int64(len(USDPerGBMonth))*1024) <= 0 {
		t.Fatal("blank class")
	}
	// Unknown class falls back to STANDARD rate.
	got := MonthlyEstimateUSD("ALIEN_STORAGE_CLASS_XYZZY", 1024*1024*1024)
	want := MonthlyEstimateUSD("STANDARD", 1024*1024*1024)
	if got != want {
		t.Fatalf("fallback: got %v want %v", got, want)
	}
	if !strings.Contains(Disclaimer(), "stale table") {
		t.Fatal(Disclaimer())
	}
	if FormatUSD(1.23456) != "$1.2346" {
		t.Fatal(FormatUSD(1.23456))
	}
}

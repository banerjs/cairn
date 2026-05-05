package pricing

import "testing"

func TestMonthlyEstimateUSD(t *testing.T) {
	v := MonthlyEstimateUSD("STANDARD", 1024*1024*1024)
	if v <= 0 {
		t.Fatal(v)
	}
}

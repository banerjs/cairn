// Package pricing holds stale-by-default US monthly storage estimates ($/GB-month).
package pricing

import (
	"fmt"
	"strings"
)

// USDPerGBMonth is a rough public price table; update at release time.
var USDPerGBMonth = map[string]float64{
	"STANDARD":            0.023,
	"STANDARD_IA":         0.0125,
	"GLACIER_IR":          0.004,
	"DEEP_ARCHIVE":        0.00099,
	"GLACIER":             0.0036,
	"INTELLIGENT_TIERING": 0.023,
	"ONEZONE_IA":          0.01,
	"REDUCED_REDUNDANCY":  0.024,
	"":                    0.023, // unset / STANDARD default guess
}

// MonthlyEstimateUSD returns bytes * price / (1024^3).
func MonthlyEstimateUSD(storageClass string, bytesTotal int64) float64 {
	key := strings.TrimSpace(string(storageClass))
	if key == "" {
		key = "STANDARD"
	}
	rate, ok := USDPerGBMonth[key]
	if !ok {
		rate = USDPerGBMonth["STANDARD"]
	}
	gb := float64(bytesTotal) / (1024 * 1024 * 1024)
	return gb * rate
}

// Disclaimer is printed next to cost estimates.
func Disclaimer() string {
	return "estimated monthly storage cost (US$, stale table in internal/pricing/pricing.go; not a quote)"
}

// FormatUSD prints a short dollar string.
func FormatUSD(v float64) string {
	return fmt.Sprintf("$%.4f", v)
}

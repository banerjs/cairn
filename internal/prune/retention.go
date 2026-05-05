package prune

import (
	"fmt"
	"sort"
	"time"

	"github.com/banerjs/cairn/internal/paths"
)

// SelectSnapshotsToKeep returns snapshot IDs to retain given keep-last and keep-monthly rules.
//
// allIDs must be unique committed snapshot IDs (those with a manifest). Order does not matter;
// the function sorts descending lexicographically (compatible with cairn snapshot id format).
func SelectSnapshotsToKeep(allIDs []string, keepLast, keepMonthly int) map[string]struct{} {
	keep := make(map[string]struct{})
	if len(allIDs) == 0 {
		return keep
	}
	sorted := append([]string(nil), allIDs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] > sorted[j] })

	if keepLast > 0 {
		n := keepLast
		if n > len(sorted) {
			n = len(sorted)
		}
		for i := 0; i < n; i++ {
			keep[sorted[i]] = struct{}{}
		}
	}

	if keepMonthly > 0 {
		now := time.Now().UTC()
		monthWant := make(map[string]struct{})
		for i := 0; i < keepMonthly; i++ {
			t := now.AddDate(0, -i, 0)
			key := fmt.Sprintf("%04d-%02d", t.Year(), t.Month())
			monthWant[key] = struct{}{}
		}
		// sorted is newest-first; first hit per calendar month wins.
		chosenPerMonth := make(map[string]string)
		for _, sid := range sorted {
			ts, err := paths.ParseSnapshotTime(sid)
			if err != nil {
				continue
			}
			mk := fmt.Sprintf("%04d-%02d", ts.Year(), ts.Month())
			if _, ok := monthWant[mk]; !ok {
				continue
			}
			if _, ok := chosenPerMonth[mk]; !ok {
				chosenPerMonth[mk] = sid
				keep[sid] = struct{}{}
			}
		}
	}

	return keep
}

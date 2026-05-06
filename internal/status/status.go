package status

import (
	"context"
	"log/slog"
	"strings"

	"github.com/banerjs/cairn/internal/paths"
	"github.com/banerjs/cairn/internal/pricing"
	"github.com/banerjs/cairn/internal/s3store"
)

// PrefixLister lists objects (*s3store.Store satisfies this).
type PrefixLister interface {
	ListPrefix(ctx context.Context, prefix string) ([]s3store.ListedObject, error)
}

// Run prints aggregate listing totals under cairn/v1/hosts/.
func Run(ctx context.Context, st PrefixLister, filterHost string, showCost bool, log *slog.Logger) error {
	objs, err := st.ListPrefix(ctx, paths.HostsRootPrefix())
	if err != nil {
		return err
	}
	perHostManifests := make(map[string]int)
	bytesByClass := make(map[string]int64)
	var oldest, newest map[string]string // host -> snapshot id

	for _, o := range objs {
		host := paths.ParseHostIDFromHostsPath(o.Key)
		if host == "" {
			continue
		}
		if filterHost != "" && host != filterHost {
			continue
		}
		sc := string(o.StorageClass)
		if sc == "" {
			sc = "STANDARD"
		}
		bytesByClass[sc] += o.Size

		if strings.Contains(o.Key, "/snapshots/") && strings.HasSuffix(o.Key, "manifest.age") {
			perHostManifests[host]++
			sid := paths.SnapshotIDFromKey(o.Key)
			if sid == "" {
				continue
			}
			if oldest == nil {
				oldest = make(map[string]string)
				newest = make(map[string]string)
			}
			if prev, ok := oldest[host]; !ok || sid < prev {
				oldest[host] = sid
			}
			if prev, ok := newest[host]; !ok || sid > prev {
				newest[host] = sid
			}
		}
	}

	var total int64
	for _, sz := range bytesByClass {
		total += sz
	}

	for host, n := range perHostManifests {
		log.Info("host", "host_id", host, "snapshots", n, "oldest", oldest[host], "newest", newest[host])
	}
	for sc, sz := range bytesByClass {
		log.Info("bytes_by_class", "storage_class", sc, "bytes", sz)
	}
	log.Info("bytes_total", "bytes", total)

	if showCost {
		var est float64
		for sc, sz := range bytesByClass {
			est += pricing.MonthlyEstimateUSD(sc, sz)
		}
		log.Info("cost_estimate", "usd_month", pricing.FormatUSD(est), "note", pricing.Disclaimer())
	}
	return nil
}

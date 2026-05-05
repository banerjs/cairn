// Package paths builds S3 object keys under the cairn/v1 layout prefix.
package paths

import (
	"fmt"
	"path"
	"strings"
	"time"
)

const LayoutPrefix = "cairn/v1"

// HostsRootPrefix lists every host subtree (trailing slash).
func HostsRootPrefix() string {
	return LayoutPrefix + "/hosts/"
}

// HostPrefix returns the prefix for all objects belonging to a host.
func HostPrefix(hostID string) string {
	return path.Join(LayoutPrefix, "hosts", hostID) + "/"
}

// SnapshotPrefix returns the prefix for one snapshot directory (with trailing slash).
func SnapshotPrefix(hostID, snapshotID string) string {
	return path.Join(LayoutPrefix, "hosts", hostID, "snapshots", snapshotID) + "/"
}

// ManifestKey is the committed manifest object for a snapshot.
func ManifestKey(hostID, snapshotID string) string {
	return path.Join(LayoutPrefix, "hosts", hostID, "snapshots", snapshotID, "manifest.age")
}

// ObjectKey is an encrypted payload object for a file within a snapshot.
func ObjectKey(hostID, snapshotID, objectID string) string {
	return path.Join(LayoutPrefix, "hosts", hostID, "snapshots", snapshotID, "objects", objectID)
}

// IndexKey is the optional encrypted index cache for a host.
func IndexKey(hostID string) string {
	return path.Join(LayoutPrefix, "hosts", hostID, "index.age")
}

// SnapshotsListPrefix lists all snapshot subtrees for a host.
func SnapshotsListPrefix(hostID string) string {
	return path.Join(LayoutPrefix, "hosts", hostID, "snapshots") + "/"
}

// ParseSnapshotIDFromManifestKey extracts snapshot ID from a manifest object key, or "" if not a manifest path.
func ParseSnapshotIDFromManifestKey(key string) string {
	// .../hosts/<host>/snapshots/<snap>/manifest.age
	parts := strings.Split(key, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "snapshots" && i+2 < len(parts) && parts[i+2] == "manifest.age" {
			return parts[i+1]
		}
	}
	return ""
}

// SnapshotIDFromKey extracts the snapshot directory name from an object key under .../snapshots/<id>/...
func SnapshotIDFromKey(key string) string {
	const marker = "/snapshots/"
	i := strings.Index(key, marker)
	if i < 0 {
		return ""
	}
	rest := key[i+len(marker):]
	j := strings.IndexByte(rest, '/')
	if j <= 0 {
		return ""
	}
	return rest[:j]
}

// ParseHostIDFromHostsPath extracts host id from keys like cairn/v1/hosts/<host>/...
func ParseHostIDFromHostsPath(key string) string {
	parts := strings.Split(key, "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "hosts" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// ParseSnapshotTime parses the UTC timestamp embedded at the start of snapshot IDs (before the hyphen suffix).
func ParseSnapshotTime(snapshotID string) (time.Time, error) {
	if snapshotID == "" {
		return time.Time{}, fmt.Errorf("empty snapshot id")
	}
	idx := strings.IndexByte(snapshotID, '-')
	if idx < 0 {
		return time.Time{}, fmt.Errorf("snapshot id missing hyphen suffix: %q", snapshotID)
	}
	ts := snapshotID[:idx]
	if len(ts) != 16 || ts[15] != 'Z' {
		return time.Time{}, fmt.Errorf("invalid snapshot timestamp in id %q", snapshotID)
	}
	return time.Parse("20060102T150405Z", ts)
}

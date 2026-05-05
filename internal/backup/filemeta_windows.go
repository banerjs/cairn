//go:build windows

package backup

import "io/fs"

func fileMeta(info fs.FileInfo) (mode *uint32, uid *int64, gid *int64) {
	return nil, nil, nil
}

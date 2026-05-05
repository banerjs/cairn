//go:build !windows

package backup

import (
	"io/fs"
	"syscall"
)

func fileMeta(info fs.FileInfo) (mode *uint32, uid *int64, gid *int64) {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil, nil, nil
	}
	m := uint32(st.Mode)
	u := int64(st.Uid)
	g := int64(st.Gid)
	return &m, &u, &g
}

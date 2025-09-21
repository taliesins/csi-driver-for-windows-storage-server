//go:build linux

package iscsi

import "golang.org/x/sys/unix"

// fsUsage returns (available, capacity, used, inodes, inodesFree, inodesUsed)
func fsUsage(path string) (int64, int64, int64, int64, int64, int64, error) {
	var st unix.Statfs_t
	if err := unix.Statfs(path, &st); err != nil {
		return 0, 0, 0, 0, 0, 0, err
	}
	bsize := int64(st.Bsize)
	capacity := int64(st.Blocks) * bsize
	available := int64(st.Bavail) * bsize
	used := capacity - (int64(st.Bfree) * bsize)

	inodesTotal := int64(st.Files)
	inodesFree := int64(st.Ffree)
	inodesUsed := inodesTotal - inodesFree
	return available, capacity, used, inodesTotal, inodesFree, inodesUsed, nil
}

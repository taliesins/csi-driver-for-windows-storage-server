//go:build !linux

package iscsi

import "fmt"

func fsUsage(path string) (int64, int64, int64, int64, int64, int64, error) {
	return 0, 0, 0, 0, 0, 0, fmt.Errorf("filesystem stats not supported on this OS")
}

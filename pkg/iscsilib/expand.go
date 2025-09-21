package iscsilib

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	klog "k8s.io/klog/v2"
	mount "k8s.io/mount-utils"
)

// Resizer is the minimal interface we need from the Kubernetes resizefs helper.
// It matches the Resize(devicePath, mountPoint) (bool, error) signature.
type Resizer interface {
	Resize(devicePath, mountPoint string) (bool, error)
}

// ExpandVolume rescans iSCSI sessions (using iscsiCmd), resizes multipath (if present),
// and grows the filesystem mounted at volumePath (if not a raw block).
func ExpandVolume(mounter mount.Interface, resizer Resizer, volumePath string) error {
	// 1) Rescan all iSCSI sessions (safe & idempotent). Non-fatal if it fails.
	if out, err := iscsiCmd("-m", "session", "--rescan"); err != nil {
		klog.V(2).Infof("iscsiadm rescan failed (non-fatal): %v, out=%s", err, out)
	}

	// 2) Raw block? nothing further to do.
	st, err := os.Stat(volumePath)
	if err != nil {
		return fmt.Errorf("stat %q failed: %w", volumePath, err)
	}
	if st.Mode()&os.ModeDevice != 0 {
		return nil
	}

	// 3) Resolve device backing the mount.
	dev, _, err := mount.GetDeviceNameFromMount(mounter, volumePath)
	if err != nil || dev == "" {
		return fmt.Errorf("cannot resolve device from mount %q: %v", volumePath, err)
	}

	// 4) If the device is a multipath map (dm-* or /mapper/), resize the map first.
	if strings.HasPrefix(dev, "/dev/dm-") || strings.Contains(dev, "/mapper/") {
		d := &Device{Name: filepath.Base(dev)}
		if err := ResizeMultipathDevice(d); err != nil {
			// Non-fatal: continue to filesystem grow; log for diagnostics.
			klog.Warningf("multipath resize failed for %s: %v", dev, err)
		}
	}

	// 5) Grow filesystem in place.
	if _, err := resizer.Resize(dev, volumePath); err != nil {
		return fmt.Errorf("filesystem resize failed for %s at %s: %w", dev, volumePath, err)
	}
	return nil
}

//go:build windows

package path

import (
	"syscall"
	"unsafe"
)

var (
	kernel32            = syscall.NewLazyDLL("kernel32.dll")
	getDiskFreeSpaceExW = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// freeBytes returns the bytes available to the caller on the volume holding p.
func freeBytes(p string) (int64, error) {
	ptr, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		return 0, err
	}
	var free, total, totalFree uint64
	r, _, callErr := getDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(ptr)),
		uintptr(unsafe.Pointer(&free)),
		uintptr(unsafe.Pointer(&total)),
		uintptr(unsafe.Pointer(&totalFree)),
	)
	if r == 0 {
		return 0, callErr
	}
	return int64(free), nil
}

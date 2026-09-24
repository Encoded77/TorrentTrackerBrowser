//go:build linux || darwin || freebsd || netbsd || openbsd

package path

import "syscall"

// freeBytes returns the bytes available to unprivileged users on the
// filesystem holding p.
func freeBytes(p string) (int64, error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(p, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

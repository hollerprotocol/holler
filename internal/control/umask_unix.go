//go:build unix

package control

import (
	"fmt"
	"os"
	"syscall"
)

func umask(m int) int { return syscall.Umask(m) }

// checkPrivateDir reports an error unless dir is a directory owned by the
// current user and closed to everyone else.
func checkPrivateDir(dir string) error {
	fi, err := os.Lstat(dir)
	if err != nil {
		return err
	}
	st, ok := fi.Sys().(*syscall.Stat_t)
	if !fi.IsDir() || fi.Mode().Perm()&0o077 != 0 || !ok || int(st.Uid) != os.Getuid() {
		return fmt.Errorf("%s is not a private directory owned by you", dir)
	}
	return nil
}

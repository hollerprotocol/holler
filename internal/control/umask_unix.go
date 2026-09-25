//go:build unix

package control

import "syscall"

func umask(m int) int { return syscall.Umask(m) }

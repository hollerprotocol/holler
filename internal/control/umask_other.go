//go:build !unix

package control

func umask(m int) int { return 0 }

//go:build !unix

package control

func umask(m int) int { return 0 }

func checkPrivateDir(dir string) error { return nil }

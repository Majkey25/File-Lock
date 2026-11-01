package main

import (
	"fmt"
	"os/exec"

	"golang.org/x/sys/unix"
)

const getTermios = unix.TIOCGETA
const setTermios = unix.TIOCSETA

func renameNoReplace(from, to string) error {
	return unix.RenameatxNp(unix.AT_FDCWD, from, unix.AT_FDCWD, to, unix.RENAME_EXCL)
}

func secureDirectory(path string) error {
	// macOS ACL inheritance can grant access independently of POSIX mode bits.
	if err := exec.Command("/bin/chmod", "-N", path).Run(); err != nil {
		return fmt.Errorf("remove inherited staging ACL: %w", err)
	}
	return nil
}

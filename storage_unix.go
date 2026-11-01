//go:build linux || darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

func cleanPath(input string) (string, error) {
	path := strings.Trim(strings.TrimSpace(input), `"`)
	if !filepath.IsAbs(path) || filepath.Clean(path) == "/" || strings.ContainsRune(path, 0) {
		return "", errors.New("use a full file path, such as /home/me/Documents/notes.txt")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func holdParents(path string) (func(), error) {
	var handles []int
	closeAll := func() {
		for _, fd := range handles {
			unix.Close(fd)
		}
	}
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		fd, err := unix.Open(dir, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
		if err != nil {
			closeAll()
			return nil, err
		}
		handles = append(handles, fd)
		var info unix.Stat_t
		if err := unix.Fstat(fd, &info); err != nil {
			closeAll()
			return nil, err
		}
		if (info.Uid != 0 && info.Uid != uint32(os.Geteuid())) || (info.Mode&0022 != 0 && info.Mode&unix.S_ISVTX == 0) {
			closeAll()
			return nil, errors.New("use a trusted local folder; parent directories must not be writable by other users")
		}
		if dir == "/" {
			return closeAll, nil
		}
	}
}

func openSource(path string) (*os.File, error) {
	item, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !item.Mode().IsRegular() {
		return nil, errors.New("select one regular file; links and special files are unsupported")
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(fd), path)
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		f.Close()
		return nil, err
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG || info.Nlink != 1 || info.Uid != uint32(os.Geteuid()) {
		f.Close()
		return nil, errors.New("select one regular file you own; folders, links and special files are unsupported")
	}
	if err := unix.Flock(fd, unix.LOCK_EX|unix.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("file is in use by another locker: %w", err)
	}
	n, err := unix.Flistxattr(fd, nil)
	if err != nil || n != 0 {
		f.Close()
		return nil, errors.New("files with extended attributes or unsupported metadata are refused to avoid losing that metadata")
	}
	return f, nil
}

func privateDirectory(parent string) (string, error) {
	dir, err := os.MkdirTemp(parent, ".file-lock-")
	if err != nil {
		return "", err
	}
	if err := secureDirectory(dir); err != nil {
		os.Remove(dir)
		return "", err
	}
	return dir, nil
}

func createStage(path string) (*os.File, error) {
	return os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
}

func publish(stage *os.File, destination string) error {
	if err := renameNoReplace(stage.Name(), destination); err != nil {
		return fmt.Errorf("publish refused; existing files are never overwritten: %w", err)
	}
	held, err := stage.Stat()
	if err != nil {
		return err
	}
	actual, err := os.Lstat(destination)
	if err != nil || !os.SameFile(held, actual) {
		return errors.New("published file identity mismatch; source was kept")
	}
	return nil
}

func removeSource(f *os.File, stageDir string, original os.FileInfo) error {
	var links unix.Stat_t
	if err := unix.Fstat(int(f.Fd()), &links); err != nil {
		return err
	}
	if links.Nlink != 1 {
		return errors.New("source gained a hard link; both copies were kept")
	}
	current, err := f.Stat()
	if err != nil {
		return err
	}
	if current.Size() != original.Size() || !current.ModTime().Equal(original.ModTime()) {
		return errors.New("source changed during processing; both copies were kept")
	}
	quarantine := filepath.Join(stageDir, "original")
	if err := renameNoReplace(f.Name(), quarantine); err != nil {
		return err
	}
	actual, err := os.Lstat(quarantine)
	if err != nil || !os.SameFile(original, actual) || actual.Size() != original.Size() || !actual.ModTime().Equal(original.ModTime()) {
		return fmt.Errorf("source changed; recover the retained file at %q before cleanup", quarantine)
	}
	return os.Remove(quarantine)
}

func hideInput(fd int) (func() error, error) {
	state, err := unix.IoctlGetTermios(fd, getTermios)
	if err != nil {
		return nil, err
	}
	hidden := *state
	hidden.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(fd, setTermios, &hidden); err != nil {
		return nil, err
	}
	return func() error { return unix.IoctlSetTermios(fd, setTermios, state) }, nil
}

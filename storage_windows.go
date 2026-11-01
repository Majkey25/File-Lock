package main

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func cleanPath(input string) (string, error) {
	path := strings.Trim(strings.TrimSpace(input), `"`)
	if !filepath.IsAbs(path) || len(path) < 3 || path[1] != ':' || strings.Contains(path[2:], ":") {
		return "", errors.New("use a full local file path, such as C:\\Documents\\notes.txt")
	}
	path = filepath.Clean(path)
	if !filepath.IsLocal(path[3:]) {
		return "", errors.New("select a regular file path, not a drive root or device name")
	}
	for _, part := range strings.Split(path[3:], `\`) {
		if strings.TrimRight(part, ". ") != part {
			return "", errors.New("paths ending in a dot or space are unsupported")
		}
	}
	return path, nil
}

func holdParents(path string) (func(), error) {
	var handles []windows.Handle
	var directories []string
	for dir := filepath.Dir(path); ; dir = filepath.Dir(dir) {
		directories = append(directories, dir)
		if filepath.Dir(dir) == dir {
			break
		}
	}
	closeAll := func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}
	for i := len(directories) - 1; i >= 0; i-- {
		dir := directories[i]
		p, err := windows.UTF16PtrFromString(dir)
		if err != nil {
			closeAll()
			return nil, err
		}
		h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
			nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
		if err != nil {
			closeAll()
			return nil, fmt.Errorf("hold parent directory: %w", err)
		}
		handles = append(handles, h)
		var info windows.ByHandleFileInformation
		if err := windows.GetFileInformationByHandle(h, &info); err != nil {
			closeAll()
			return nil, err
		}
		if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			closeAll()
			return nil, errors.New("paths through junctions, symbolic links and cloud reparse points are unsupported")
		}
	}
	return closeAll, nil
}

// Hold the exact source object against writes, renames and deletion until commit.
func openSource(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.DELETE, windows.FILE_SHARE_READ,
		nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT|windows.FILE_FLAG_SEQUENTIAL_SCAN, 0)
	if err != nil {
		return nil, fmt.Errorf("open file (close editors and other locker instances first): %w", err)
	}
	f := os.NewFile(uintptr(h), path)
	if err := validateSource(h, path); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func validateSource(h windows.Handle, path string) error {
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&(windows.FILE_ATTRIBUTE_REPARSE_POINT|windows.FILE_ATTRIBUTE_DIRECTORY|windows.FILE_ATTRIBUTE_ENCRYPTED) != 0 || info.NumberOfLinks != 1 {
		return errors.New("select one regular file; folders, links, cloud placeholders and EFS files are unsupported")
	}
	volume, err := windows.UTF16PtrFromString(filepath.VolumeName(path) + `\`)
	if err != nil {
		return err
	}
	if windows.GetDriveType(volume) != windows.DRIVE_FIXED {
		return errors.New("select a file on a local fixed NTFS drive")
	}
	var filesystem [32]uint16
	if err := windows.GetVolumeInformationByHandle(h, nil, 0, nil, nil, nil, &filesystem[0], uint32(len(filesystem))); err != nil {
		return err
	}
	if windows.UTF16ToString(filesystem[:]) != "NTFS" {
		return errors.New("only NTFS volumes are supported to protect file transactions and staging permissions")
	}
	// Replacing a file with extra NTFS streams would silently discard those streams.
	// FILE_STREAM_INFO must be aligned to an eight-byte boundary.
	var streams [8192]uint64
	if err := windows.GetFileInformationByHandleEx(h, windows.FileStreamInfo, (*byte)(unsafe.Pointer(&streams[0])), uint32(unsafe.Sizeof(streams))); err != nil {
		return fmt.Errorf("check alternate data streams: %w", err)
	}
	stream := (*struct {
		Next       uint32
		NameLength uint32
		Size       int64
		Allocated  int64
		Name       [10]uint16
	})(unsafe.Pointer(&streams[0]))
	if stream.Next != 0 || stream.NameLength != 14 || windows.UTF16ToString(stream.Name[:7]) != "::$DATA" {
		return errors.New("files with alternate data streams are unsupported; no data was changed")
	}
	return nil
}

func privateDirectory(parent string) (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")")
	if err != nil {
		return "", err
	}
	path := filepath.Join(parent, ".file-lock-"+rand.Text())
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return "", err
	}
	attributes := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}
	if err := windows.CreateDirectory(p, &attributes); err != nil {
		return "", err
	}
	return path, nil
}

func createStage(path string) (*os.File, error) {
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE|windows.DELETE,
		windows.FILE_SHARE_READ, nil, windows.CREATE_NEW, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), path), nil
}

func publish(stage *os.File, destination string) error {
	name, err := windows.UTF16FromString(destination)
	if err != nil {
		return err
	}
	// Keep the UTF-16 terminator inside the buffer passed to Win32.
	type renameInfo struct {
		Replace    uint32
		Root       windows.Handle
		NameLength uint32
		Name       [1]uint16
	}
	size := int(unsafe.Offsetof(renameInfo{}.Name)) + len(name)*2
	buffer := make([]uint64, (size+7)/8)
	info := (*renameInfo)(unsafe.Pointer(&buffer[0]))
	info.NameLength = uint32((len(name) - 1) * 2)
	copy(unsafe.Slice(&info.Name[0], len(name)), name)
	// Rename this held object without replacement; keep it protected until commit.
	if err := windows.SetFileInformationByHandle(windows.Handle(stage.Fd()), windows.FileRenameInfo,
		(*byte)(unsafe.Pointer(info)), uint32(size)); err != nil {
		return fmt.Errorf("publish refused; source is unchanged and existing files are never overwritten: %w", err)
	}
	held, err := stage.Stat()
	if err != nil {
		return err
	}
	actual, err := os.Stat(destination)
	if err != nil {
		return fmt.Errorf("verify published path; source was kept: %w", err)
	}
	if !os.SameFile(held, actual) {
		return errors.New("published file identity mismatch; source was kept")
	}
	return nil
}

func removeSource(f *os.File) error {
	deletePending := uint32(1)
	return windows.SetFileInformationByHandle(windows.Handle(f.Fd()), windows.FileDispositionInfo,
		(*byte)(unsafe.Pointer(&deletePending)), uint32(unsafe.Sizeof(deletePending)))
}

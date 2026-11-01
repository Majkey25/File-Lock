//go:build linux || darwin

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestUnixSourceValidation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	writeFixture(t, path, []byte("private"))
	f, err := openSource(path)
	if err != nil {
		t.Fatal(err)
	}
	if other, err := openSource(path); err == nil {
		other.Close()
		t.Fatal("second locker acquired source")
	}
	f.Close()
	link := filepath.Join(dir, "link")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if f, err := openSource(link); err == nil {
		f.Close()
		t.Fatal("symlink accepted")
	}
	alias := filepath.Join(dir, "alias")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if f, err := openSource(path); err == nil {
		f.Close()
		t.Fatal("hardlink accepted")
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(path, "user.filelock-test", []byte("metadata"), 0); err == nil {
		if f, err := openSource(path); err == nil {
			f.Close()
			t.Fatal("extended attributes accepted")
		}
	}
}

func TestUnixPrivateStagingAndPublish(t *testing.T) {
	dir := t.TempDir()
	stageDir, err := privateDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(stageDir)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("unsafe staging permissions: %v", err)
	}
	f, err := createStage(filepath.Join(stageDir, "data"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "output")
	writeFixture(t, dest, []byte("keep"))
	if err := publish(f, dest); err == nil {
		t.Fatal("publish overwrote existing output")
	}
	if string(contents(t, dest)) != "keep" {
		t.Fatal("existing output changed")
	}
	if err := os.Remove(dest); err != nil {
		t.Fatal(err)
	}
	if err := publish(f, dest); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if string(contents(t, dest)) != "new" {
		t.Fatal("published bytes changed")
	}
	info, err = os.Stat(dest)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("unsafe output permissions: %v", err)
	}
	if err := cleanupStage(stageDir); err != nil {
		t.Fatal(err)
	}
}

func TestUnixSourceReplacementIsNeverDeleted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	writeFixture(t, path, []byte("original"))
	f, err := openSource(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".moved"); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path, []byte("replacement"))
	stageDir, err := privateDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := removeSource(f, stageDir, before); err == nil {
		t.Fatal("replacement source removed")
	}
	if err := cleanupStage(stageDir); err == nil {
		t.Fatal("cleanup removed a quarantined source")
	}
	if string(contents(t, path+".moved")) != "original" || string(contents(t, filepath.Join(stageDir, "original"))) != "replacement" {
		t.Fatal("source or replacement data lost")
	}
}

func TestUnixChangedSourceIsPreserved(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source")
	writeFixture(t, path, []byte("before"))
	f, err := openSource(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	before, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path, []byte("changed by another program"))
	stageDir, err := privateDirectory(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := removeSource(f, stageDir, before); err == nil || !strings.Contains(err.Error(), "source changed") {
		t.Fatalf("change ignored: %v", err)
	}
	if string(contents(t, path)) != "changed by another program" {
		t.Fatal("changed source deleted")
	}
	if err := cleanupStage(stageDir); err != nil {
		t.Fatal(err)
	}
}

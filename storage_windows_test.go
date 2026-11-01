package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestSourceHeldAgainstChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "source.txt")
	writeFixture(t, path, []byte("original"))
	f, err := openSource(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if other, err := os.OpenFile(path, os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
		other.Close()
		t.Fatal("source allowed concurrent truncation")
	}
	if err := os.Rename(path, path+".moved"); err == nil {
		t.Fatal("source allowed concurrent rename")
	}
	if err := os.Remove(path); err == nil {
		t.Fatal("source allowed concurrent deletion")
	}
	if other, err := openSource(path); err == nil {
		other.Close()
		t.Fatal("source allowed a second locker")
	}
	data, err := io.ReadAll(f)
	if err != nil || string(data) != "original" {
		t.Fatal("source changed")
	}
}

func TestLinksAndExtraStreamsRejected(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.txt")
	writeFixture(t, path, []byte("original"))
	alias := filepath.Join(dir, "alias.txt")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if f, err := openSource(path); err == nil {
		f.Close()
		t.Fatal("hardlinked source accepted")
	} else if !strings.Contains(err.Error(), "folders, links") {
		t.Fatalf("wrong hardlink error: %v", err)
	}
	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path+":extra", []byte("must not disappear"))
	if f, err := openSource(path); err == nil {
		f.Close()
		t.Fatal("extra data stream accepted")
	} else if !strings.Contains(err.Error(), "alternate data streams are unsupported") {
		t.Fatalf("wrong stream error: %v", err)
	}
	if string(contents(t, path+":extra")) != "must not disappear" {
		t.Fatal("extra stream changed")
	}
}

func TestPublishRaceNeverClobbers(t *testing.T) {
	dir := t.TempDir()
	stage, dest := filepath.Join(dir, "stage"), filepath.Join(dir, "dest")
	f, err := createStage(stage)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("new"); err != nil {
		t.Fatal(err)
	}
	writeFixture(t, dest, []byte("keep"))
	if err := publish(f, dest); err == nil {
		f.Close()
		t.Fatal("publish overwrote a racing destination")
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if string(contents(t, stage)) != "new" || string(contents(t, dest)) != "keep" {
		t.Fatal("publish changed existing files")
	}
}

func TestPublishedOutputHeldUntilClose(t *testing.T) {
	dir := t.TempDir()
	f, err := createStage(filepath.Join(dir, "stage"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("verified output"); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "output")
	if err := publish(f, dest); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dest); err == nil {
		t.Fatal("published output allowed deletion before commit")
	}
	if err := os.Rename(dest, dest+".moved"); err == nil {
		t.Fatal("published output allowed rename before commit")
	}
	if other, err := os.OpenFile(dest, os.O_WRONLY|os.O_TRUNC, 0600); err == nil {
		other.Close()
		t.Fatal("published output allowed modification before commit")
	}
}

func TestPublishDifferentNameLengths(t *testing.T) {
	dir := t.TempDir()
	for length := 1; length <= 16; length++ {
		f, err := createStage(filepath.Join(dir, "stage"))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.WriteString("verified bytes"); err != nil {
			t.Fatal(err)
		}
		dest := filepath.Join(dir, strings.Repeat("a", length)+"ž.age")
		if err := publish(f, dest); err != nil {
			f.Close()
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
		if string(contents(t, dest)) != "verified bytes" {
			t.Fatal("published path or bytes changed")
		}
	}
}

func TestPrivateStagingACL(t *testing.T) {
	dir, err := privateDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := sd.Control()
	if err != nil || control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("staging ACL inherits parent access: %v", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	if dacl.AceCount != 2 {
		t.Fatalf("expected only current user and SYSTEM, got %d entries", dacl.AceCount)
	}
}

func TestPathValidation(t *testing.T) {
	for _, path := range []string{"", "relative.txt", `\\server\share\file`, `\\?\C:\file`, `C:\file:stream`, `C:\file.`, `C:\folder \file`} {
		if _, err := cleanPath(path); err == nil {
			t.Errorf("unsafe path accepted: %q", path)
		}
	}
	want := `C:\Documents\žluťoučký notes [1].txt`
	if got, err := cleanPath(`"` + want + `"`); err != nil || got != want {
		t.Fatalf("quoted Unicode path failed: %q, %v", got, err)
	}
}

func TestParentDirectoriesHeld(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "parent")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	release, err := holdParents(filepath.Join(dir, "file"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(dir, dir+".moved"); err == nil {
		release()
		t.Fatal("parent directory allowed rename during operation")
	}
	release()
	if err := os.Rename(dir, dir+".moved"); err != nil {
		t.Fatalf("parent lock was not released: %v", err)
	}
}

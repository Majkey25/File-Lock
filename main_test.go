package main

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetupPersistsOnlyPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	writeFixture(t, path, []byte("private"))
	config := filepath.Join(dir, "config", "target.txt")
	got, err := setup(bufio.NewReader(strings.NewReader(`"`+path+"\"\n")), config)
	if err != nil || got != path || string(contents(t, config)) != path {
		t.Fatalf("setup did not persist path: %q, %v", got, err)
	}
	if _, err := setup(bufio.NewReader(strings.NewReader("missing\n")), config); err == nil {
		t.Fatal("setup accepted invalid path")
	}
	if string(contents(t, config)) != path {
		t.Fatal("failed setup changed saved path")
	}
}

func TestMenuStatesAndMovedVault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notes.txt")
	if status("") != "No file selected" {
		t.Fatal("empty state wrong")
	}
	writeFixture(t, path, []byte("plain"))
	if status(path) != "Unlocked" {
		t.Fatal("unlocked state wrong")
	}
	writeFixture(t, path+".age", []byte(ageHeader))
	if !strings.HasPrefix(status(path), "Both copies") {
		t.Fatal("conflict state hidden")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if status(path) != "Locked" || status(path+".age") != "Locked" {
		t.Fatal("locked or moved vault state wrong")
	}
	if got, err := selectedFile(path+".age", false); err != nil || got != path+".age" {
		t.Fatalf("moved vault not selected: %q, %v", got, err)
	}
	if _, err := selectedFile("", true); err == nil {
		t.Fatal("operation allowed without setup")
	}
}

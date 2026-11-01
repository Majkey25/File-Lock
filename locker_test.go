package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"filippo.io/age"
)

const testPassword = "test-only random passphrase 2026"

func writeFixture(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
}

func contents(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertNoStages(t *testing.T, dir string) {
	t.Helper()
	stages, err := filepath.Glob(filepath.Join(dir, ".file-lock-*"))
	if err != nil || len(stages) != 0 {
		t.Fatalf("staging files left: %v, %v", stages, err)
	}
}

func makeAgeFixture(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := age.NewScryptRecipient(testPassword)
	if err != nil {
		t.Fatal(err)
	}
	// Only synthetic failure fixtures use a reduced cost; production stays at 18.
	r.SetWorkFactor(10)
	var out bytes.Buffer
	w, err := age.Encrypt(&out, r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}

func TestRoundTripAndUpstreamCompatibility(t *testing.T) {
	for _, size := range []int{0, 1, 65536, 65537, 2 << 20} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "žluťoučký notes [1].bin")
			original := bytes.Repeat([]byte{0x00, 0xff, 0x80, 0x0a}, (size+3)/4)[:size]
			writeFixture(t, path, original)
			vault, err := transform(path, testPassword, true)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("plaintext still exists: %v", err)
			}
			cipher := contents(t, vault)
			if !bytes.HasPrefix(cipher, []byte(ageHeader)) || !bytes.Contains(cipher[:150], []byte(" 18\n")) {
				t.Fatal("not a standard age file with the production work factor")
			}
			identity, err := age.NewScryptIdentity(testPassword)
			if err != nil {
				t.Fatal(err)
			}
			reader, err := age.Decrypt(bytes.NewReader(cipher), identity)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := io.ReadAll(reader)
			if err != nil || !bytes.Equal(decoded, original) {
				t.Fatalf("upstream age decryption mismatch: %v", err)
			}
			// A renamed vault stays readable, with no dependency on its original path.
			moved := filepath.Join(dir, "moved.bin.age")
			if err := os.Rename(vault, moved); err != nil {
				t.Fatal(err)
			}
			restored, err := transform(moved, testPassword, false)
			if err != nil || !bytes.Equal(contents(t, restored), original) {
				t.Fatalf("round trip mismatch: %v", err)
			}
			if _, err := os.Stat(moved); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("vault still exists: %v", err)
			}
			assertNoStages(t, dir)
		})
	}
}

func TestFailedUnlockPreservesVault(t *testing.T) {
	original := makeAgeFixture(t, bytes.Repeat([]byte("private data"), 10000))
	for _, scenario := range []string{"wrong-password", "truncated", "tampered", "trailing-data", "bad-header"} {
		t.Run(scenario, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "secret.age")
			cipher := bytes.Clone(original)
			password := testPassword
			switch scenario {
			case "wrong-password":
				password = "incorrect passphrase"
			case "truncated":
				cipher = cipher[:len(cipher)-1]
			case "tampered":
				cipher[len(cipher)-10] ^= 1
			case "trailing-data":
				cipher = append(cipher, 1)
			case "bad-header":
				cipher[0] ^= 1
			}
			writeFixture(t, path, cipher)
			_, err := transform(path, password, false)
			want := "operation failed; source was kept"
			if scenario == "bad-header" {
				want = "unlock requires a binary age file"
			}
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("expected %q, got %v", want, err)
			}
			if !bytes.Equal(contents(t, path), cipher) {
				t.Fatal("failed unlock changed ciphertext")
			}
			if _, err := os.Stat(path[:len(path)-4]); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed unlock published plaintext: %v", err)
			}
			assertNoStages(t, dir)
		})
	}
}

func TestExistingDestinationsNeverOverwritten(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "secret")
	vault := plain + ".age"
	writeFixture(t, plain, []byte("keep plaintext"))
	cipher := makeAgeFixture(t, []byte("different data"))
	writeFixture(t, vault, cipher)
	for _, operation := range []struct {
		path string
		lock bool
	}{{plain, true}, {vault, false}} {
		if _, err := transform(operation.path, testPassword, operation.lock); err == nil || !strings.Contains(err.Error(), "destination already exists") {
			t.Fatalf("expected destination conflict, got %v", err)
		}
	}
	if string(contents(t, plain)) != "keep plaintext" || !bytes.Equal(contents(t, vault), cipher) {
		t.Fatal("conflict changed existing data")
	}
	assertNoStages(t, dir)
}

func TestInvalidSources(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.txt")
	writeFixture(t, plain, []byte("plain"))
	vault := filepath.Join(dir, "already.age")
	writeFixture(t, vault, makeAgeFixture(t, []byte("locked")))
	for _, operation := range []struct {
		path string
		lock bool
	}{
		{filepath.Join(dir, "missing"), true}, {dir, true}, {plain, false}, {vault, true},
	} {
		if _, err := transform(operation.path, testPassword, operation.lock); err == nil {
			t.Fatalf("invalid operation accepted: %+v", operation)
		}
	}
	assertNoStages(t, dir)
}

func TestUntrustedWorkFactorIsBounded(t *testing.T) {
	cipher := makeAgeFixture(t, []byte("test"))
	cipher = bytes.Replace(cipher, []byte(" 10\n"), []byte(" 30\n"), 1)
	if err := decryptTo(io.Discard, bytes.NewReader(cipher), testPassword); err == nil {
		t.Fatal("excessive work factor accepted")
	}
}

package main

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
)

const ageHeader = "age-encryption.org/v1\n"

func encrypted(f *os.File) (bool, error) {
	var header [len(ageHeader)]byte
	n, err := f.ReadAt(header[:], 0)
	if err != nil && !errors.Is(err, io.EOF) {
		return false, err
	}
	return n == len(header) && string(header[:]) == ageHeader, nil
}

func transform(path, password string, lock bool) (destination string, result error) {
	path, err := cleanPath(path)
	if err != nil {
		return "", err
	}
	releaseParents, err := holdParents(path)
	if err != nil {
		return "", err
	}
	defer releaseParents()
	source, err := openSource(path)
	if err != nil {
		return "", err
	}
	var stage *os.File
	var stageDir string
	defer func() {
		result = errors.Join(result, source.Close())
		if stage != nil {
			result = errors.Join(result, stage.Close())
		}
		if stageDir != "" {
			if err := os.RemoveAll(stageDir); err != nil {
				result = errors.Join(result, fmt.Errorf("remove staging directory %q manually; it may contain plaintext: %w", stageDir, err))
			}
		}
	}()
	isEncrypted, err := encrypted(source)
	if err != nil {
		return "", err
	}
	if lock && (isEncrypted || strings.HasSuffix(strings.ToLower(path), ".age")) {
		return "", errors.New("file is already encrypted or has a reserved .age extension")
	}
	if !lock && (!isEncrypted || !strings.HasSuffix(strings.ToLower(path), ".age")) {
		return "", errors.New("unlock requires a binary age file ending in .age")
	}
	destination = path + ".age"
	if !lock {
		destination = path[:len(path)-4]
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return "", errors.New("destination already exists or cannot be checked; nothing was changed")
	}
	stageDir, err = privateDirectory(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("create private staging directory: %w", err)
	}
	stagePath := filepath.Join(stageDir, "data")
	stage, err = createStage(stagePath)
	if err != nil {
		return "", err
	}
	if lock {
		err = encryptVerified(stage, source, password)
	} else {
		err = decryptTo(stage, source, password)
	}
	if err == nil {
		err = stage.Sync()
	}
	if err != nil {
		return "", fmt.Errorf("operation failed; source was kept: %w", err)
	}
	if err := publish(stage, destination); err != nil {
		return "", err
	}
	if err := removeSource(source); err != nil {
		return destination, fmt.Errorf("output is safe at %q, but source removal failed; BOTH copies remain: %w", destination, err)
	}
	return destination, nil
}

func encryptVerified(out *os.File, in io.Reader, password string) error {
	recipient, err := age.NewScryptRecipient(password)
	if err != nil {
		return err
	}
	w, err := age.Encrypt(out, recipient)
	if err != nil {
		return err
	}
	original := sha256.New()
	if _, err := io.Copy(w, io.TeeReader(in, original)); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	if _, err := out.Seek(0, io.SeekStart); err != nil {
		return err
	}
	verified := sha256.New()
	if err := decryptTo(verified, out, password); err != nil {
		return fmt.Errorf("verify encrypted copy: %w", err)
	}
	if !bytes.Equal(original.Sum(nil), verified.Sum(nil)) {
		return errors.New("verification failed; source was kept")
	}
	return nil
}

func decryptTo(out io.Writer, in io.Reader, password string) error {
	identity, err := age.NewScryptIdentity(password)
	if err != nil {
		return err
	}
	// Accept age's standard work factor while bounding hostile header resource use.
	identity.SetMaxWorkFactor(18)
	r, err := age.Decrypt(in, identity)
	if err != nil {
		return fmt.Errorf("wrong passphrase or invalid age header: %w", err)
	}
	if _, err := io.Copy(out, r); err != nil {
		return fmt.Errorf("authentication or write failed: %w", err)
	}
	return nil
}

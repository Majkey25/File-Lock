package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

const version = "1.1.0"

func configPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "File-Lock", "target.txt"), nil
}

func readLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	return strings.TrimSpace(line), err
}

func passphrase(confirm bool) (string, error) {
	fmt.Print("  Passphrase: ")
	value, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return "", fmt.Errorf("read hidden passphrase from terminal: %w", err)
	}
	defer clear(value)
	if !confirm {
		return string(value), nil
	}
	if !utf8.Valid(value) || utf8.RuneCount(value) < 12 || strings.TrimSpace(string(value)) == "" {
		return "", errors.New("use at least 12 characters; five or more random words are better")
	}
	fmt.Print("  Confirm:    ")
	again, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	defer clear(again)
	if err != nil {
		return "", err
	}
	if string(value) != string(again) {
		return "", errors.New("passphrases differ; nothing was changed")
	}
	return string(value), nil
}

func setup(reader *bufio.Reader, config string) (string, error) {
	fmt.Print("\n  Paste the full path to your file or moved .age file:\n  > ")
	line, err := readLine(reader)
	if err != nil {
		return "", err
	}
	path, err := cleanPath(line)
	if err != nil {
		return "", err
	}
	f, err := openSource(path)
	if err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(config), 0700); err != nil {
		return "", err
	}
	if err := os.WriteFile(config, []byte(path), 0600); err != nil {
		return "", fmt.Errorf("save selected path: %w", err)
	}
	return path, nil
}

func selectedFile(target string, lock bool) (string, error) {
	if target == "" {
		return "", errors.New("press 0 to select a file first")
	}
	if lock {
		return target, nil
	}
	if strings.HasSuffix(strings.ToLower(target), ".age") {
		return target, nil
	}
	return target + ".age", nil
}

func status(target string) string {
	if target == "" {
		return "No file selected"
	}
	plain, vault := target, target+".age"
	if strings.HasSuffix(strings.ToLower(target), ".age") {
		plain, vault = target[:len(target)-4], target
	}
	_, p := os.Lstat(plain)
	_, v := os.Lstat(vault)
	if p == nil && v == nil {
		return "Both copies exist | check them before continuing"
	}
	if p == nil && errors.Is(v, os.ErrNotExist) {
		return "Unlocked"
	}
	if v == nil && errors.Is(p, os.ErrNotExist) {
		return "Locked"
	}
	return "Missing or inaccessible | use setup after moving the file"
}

func run(reader *bufio.Reader) error {
	config, err := configPath()
	if err != nil {
		return err
	}
	var target string
	data, err := os.ReadFile(config)
	if err == nil {
		target, err = cleanPath(string(data))
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fmt.Printf("  Saved path unavailable: %v\n", err)
	}
	for {
		fmt.Printf("\n  FILE LOCK  /  %s\n  ────────────────────────────────────\n", version)
		if target != "" {
			fmt.Printf("  %s\n", target)
		}
		fmt.Printf("  %s\n\n  [0] Setup    [1] Lock    [2] Unlock    [Q] Quit\n\n  > ", status(target))
		choice, err := readLine(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch strings.ToLower(choice) {
		case "q":
			return nil
		case "0":
			path, err := setup(reader, config)
			if err != nil {
				fmt.Printf("\n  Error: %v\n", err)
			} else {
				target = path
				fmt.Println("\n  Path saved. Your passphrase is never saved.")
			}
		case "1", "2":
			lock := choice == "1"
			path, err := selectedFile(target, lock)
			if err == nil {
				if lock && strings.HasSuffix(strings.ToLower(path), ".age") {
					path = path[:len(path)-4]
				}
				var password string
				if lock {
					fmt.Println("\n  Keep your passphrase. There is no reset or recovery key.")
				}
				password, err = passphrase(lock)
				if err == nil {
					fmt.Println("  Working. Keep this window open...")
					var output string
					output, err = transform(path, password, lock)
					password = ""
					if err == nil {
						fmt.Printf("\n  Done: %s\n", output)
					}
				}
			}
			if err != nil {
				fmt.Printf("\n  Error: %v\n", err)
			}
		default:
			fmt.Println("\n  Choose 0, 1, 2 or Q.")
		}
	}
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--version" {
		fmt.Println("File Lock " + version)
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: %s [--version]\n", filepath.Base(os.Args[0]))
		os.Exit(1)
	}
	if err := run(bufio.NewReader(os.Stdin)); err != nil {
		fmt.Fprintln(os.Stderr, "Error:", err)
		os.Exit(1)
	}
}

# File Lock

[![Download latest](https://img.shields.io/badge/Download-latest_release-167D57?style=for-the-badge&logo=windows)](https://github.com/Majkey25/File-Lock/releases/latest/download/File-Lock-windows-x64.zip)
[![Release](https://img.shields.io/github/v/release/Majkey25/File-Lock)](https://github.com/Majkey25/File-Lock/releases/latest)
[![Tests](https://github.com/Majkey25/File-Lock/actions/workflows/ci.yml/badge.svg)](https://github.com/Majkey25/File-Lock/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Windows x64](https://img.shields.io/badge/Windows-10%20%2F%2011-0078D4)](https://github.com/Majkey25/File-Lock/releases/latest)

Encrypt a file with a passphrase. Unlock it when you need it. Move, rename or delete the locked file normally.

```text
  FILE LOCK  /  1.0.0
  ────────────────────────────────────
  C:\Documents\notes.txt
  Unlocked

  [0] Setup    [1] Lock    [2] Unlock    [Q] Quit

  >
```

## Download and use

1. [Download the latest Windows x64 ZIP](https://github.com/Majkey25/File-Lock/releases/latest/download/File-Lock-windows-x64.zip) and extract it completely.
2. Open `File-Lock.cmd`, or run `FileLock.exe` in your terminal. No installation, administrator access, PowerShell, Python or separate age executable is required.
3. Press `0`, then paste the full path to one file. Quoted paths and Unicode names work.
4. Press `1`. Enter a new passphrase of at least 12 characters and confirm it. Prefer five or more randomly chosen words from a password manager.
5. Press `2` and enter the same passphrase to unlock. Press `Q` to quit.

The selected path is remembered in `%APPDATA%\File-Lock\target.txt`. The passphrase is never saved. Menu choices require Enter. Passphrase input is hidden.

Locking `notes.txt` creates `notes.txt.age`. The app decrypts the new encrypted copy into a hash and compares it with the source before removing the original. Unlocking authenticates the complete encrypted file before publishing restored `notes.txt`, then removes `notes.txt.age`.

If you move or rename a locked file, keep its `.age` extension. Use `0` to select its new full path. For example, `renamed.txt.age` restores to `renamed.txt` in that folder. The file does not depend on its original path or Windows account for decryption.

**Keep the passphrase and a separate backup. There is no reset, backdoor or recovery key. Losing the passphrase means losing access to the contents. Deleting the encrypted file also deletes your copy of the data.**

## Security and limits

* Uses the maintained [age library](https://github.com/FiloSottile/age) v1.3.2 and its standard binary format. Passphrase protection uses scrypt with work factor 18, approximately 256 MiB of working memory, and authenticated ChaCha20-Poly1305 encryption. No custom cryptography.
* Each operation uses fresh randomness. Existing destinations are never overwritten, including a destination created during an operation.
* The exact source file is held against writes, moves and deletion while processing. Its parent directories are held against renaming. Locks are released when the operation ends. Other readers already holding the file may prevent the operation from starting.
* Temporary files use a private same-volume directory accessible to the current Windows user and SYSTEM. Normal failures clean staging files and preserve the source. If source deletion fails after publishing, the app reports that both copies remain. Check them manually before deleting anything.
* Supports single regular files on local fixed NTFS volumes on Windows 10/11 x64. Refuses directories, hard links, symbolic links, junction paths, cloud reparse points, EFS files, alternate data streams, device paths and network shares. Move a cloud file into a normal local folder first. No folder archives or batch encryption.
* Imported files must use binary passphrase age format and a scrypt work factor no greater than 18. Files produced by the original scripts with standard age defaults are supported. Armored files, public-key recipients and `.tar.age` folder restoration are outside this app's scope.
* File names and approximate sizes remain visible. Content encryption does not hide these. Restored files get a fresh timestamp and private permissions; original ACLs, attributes and metadata are not backed up.
* Removing plaintext is ordinary file deletion, **not secure erasure**. SSDs, backups, cloud history, editor recovery files, memory and the Windows page file may retain data. Use BitLocker for disk protection. Malware or an administrator controlling your machine can read unlocked data or capture your passphrase.
* A forced termination or power loss can leave `.file-lock-*` staging directories, potentially containing partial plaintext, or both complete copies. Check the original, output and staging directory before cleanup. Do not interrupt an operation. No power-loss durability guarantee is made.
* Release executables are unsigned. Windows SmartScreen may flag an unfamiliar download. Compare the ZIP's SHA-256 with `SHA256SUMS.txt` from the same release. These checksums detect a mismatched download; they are not an independent signature.

## Recovery with age

The encrypted data is standard age, so this app is not required for recovery. With the official [age CLI](https://github.com/FiloSottile/age/releases) installed, choose an output path that does not already exist:

```powershell
age --decrypt --output recovered-notes.txt notes.txt.age
```

Enter your passphrase when age prompts. This command keeps the encrypted input. Preserve it until you have checked the recovered file.

## Build and check

Install Go 1.27.3 or newer on Windows. Dependencies are pinned in `go.mod` and `go.sum`.

```powershell
go mod verify
go vet ./...
go test -count=1 -v ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
go build -trimpath -ldflags="-s -w" -o FileLock.exe .
pwsh -NoProfile -File scripts/build.ps1
```

The packaging script creates `dist/File-Lock-windows-x64.zip` and `dist/SHA256SUMS.txt`. GitHub Actions runs the Windows tests before creating a release for a `v*` tag. Tests exercise actual NTFS files, production-strength round trips, upstream age decryption, moved vaults, wrong passwords, corrupt and truncated ciphertext, conflicts, source sharing locks, hard links, alternate streams and staging permissions.

MIT license. Bundled dependency notices are included in each release ZIP.

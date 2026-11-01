# Security and recovery

## Encryption

File Lock uses `filippo.io/age` v1.3.2 with standard binary passphrase age files. It uses the library's default scrypt work factor 18, approximately 256 MiB of working memory, and authenticated ChaCha20-Poly1305 encryption. Every encryption uses fresh randomness.

Passphrase entry is hidden. New passphrases require confirmation and at least 12 Unicode characters. Length alone does not guarantee strength; use a password manager or five or more randomly selected words. Passphrases are never written to configuration, logs or command-line arguments. Memory and the operating system's swap/page file may retain secrets.

The encrypted data does not depend on its original path or OS account. Standard age tools can decrypt it on other systems with the correct passphrase. Imported files must use binary passphrase age format with a scrypt work factor no greater than 18. Armored files, public-key recipients and folder extraction are unsupported.

## File transactions

The app writes to a private directory on the source volume. It flushes completed output before publishing, never replaces an existing destination and verifies the published file's identity. Encryption includes a full authenticated decryption and hash comparison, followed by a second source hash. Decryption authenticates all data before publishing plaintext.

Windows holds the source and published output against writes, renames and deletion during the operation. The staging directory's protected DACL grants access only to the current user and SYSTEM.

Linux and macOS cannot enforce Windows sharing semantics. They use nonblocking advisory locks, private staging with mode `0700`, output files with mode `0600`, source metadata checks and atomic no-overwrite renames. Before deletion, the source moves into private staging and its identity is checked again. If its path was replaced, the replacement is retained in staging and the operation reports its location. macOS staging also removes inherited ACLs.

Advisory locks do not stop other software from ignoring them. Close every program using the file, and do not rename parent directories while processing. Source-change checks reduce accidental races; they do not protect against malware or a hostile process running under your account. Do not use shared, network or untrusted folders.

Ordinary failures preserve the source and clean incomplete output. If removal fails after publication, both copies remain. Read the error before deleting or replacing either copy. Staging cleanup never recursively removes a quarantined source.

## Supported files and metadata

* Single regular local files only. No folders, batch operations, devices, sockets, FIFOs or symbolic/hard links.
* Windows requires local fixed NTFS. It refuses reparse points, junction paths, cloud placeholders, EFS and alternate data streams.
* Linux and macOS require files owned by the current user in trusted directories. Files with extended attributes are refused to avoid silently losing resource forks, application metadata or security labels. Linux requires filesystem support for `renameat2(RENAME_NOREPLACE)`; macOS uses `renameatx_np(RENAME_EXCL)`.
* Restored files receive fresh timestamps and private permissions. Original ACLs, extended attributes and other metadata are not backed up. Names and approximate sizes remain visible.

## Backups, interruption and secure erasure

Keep an independent backup. File Lock performs ordinary file deletion, not secure erasure. SSDs, filesystem snapshots, editor recovery, backups and cloud version history may retain plaintext. Use full-disk encryption, such as BitLocker, FileVault or LUKS, for disk protection.

A forced termination or power loss may leave `.file-lock-*` staging directories, partial plaintext, a quarantined `original` file or both complete copies. Check the source, destination and staging directory before cleanup. Recover any `original` file before removing its staging directory. No power-loss durability guarantee is made.

There is no passphrase reset or recovery key. Losing the passphrase means losing access. Deleting the encrypted file deletes that copy of the data. Malware and administrators controlling your machine can capture the passphrase or unlocked content.

## Recover with the official age CLI

Choose a new output path that does not already exist:

```sh
age --decrypt --output recovered-notes.txt notes.txt.age
```

Enter your passphrase when prompted. This command keeps the encrypted input. Verify the recovered file before deleting the encrypted copy. A decrypted `.tar.age` from an older folder script is a tar archive; this app does not extract it.

## Configuration and downloads

Only the selected file path is saved:

| System | Settings file |
| :--- | :--- |
| Windows | `%APPDATA%\File-Lock\target.txt` |
| Linux | `$XDG_CONFIG_HOME/File-Lock/target.txt`, or `~/.config/File-Lock/target.txt` |
| macOS | `~/Library/Application Support/File-Lock/target.txt` |

Release executables are unsigned. macOS builds are not notarized. Windows SmartScreen and macOS Gatekeeper may flag unfamiliar downloads. Verify the release source and SHA-256 checksum before deciding whether to run a downloaded executable. You can also build from source.

SHA-256 checksums detect mismatched downloads; they are not an independent signature. Source vulnerability scanning runs in CI. Scanning stripped executables with govulncheck can fall back to module-level advisories, including unused packages. The app does not import OpenPGP.

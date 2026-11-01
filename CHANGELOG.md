# File Lock v1.0.0

First public release for Windows 10/11 x64.

* Portable executable and terminal launcher. No runtime installation required.
* Menu: `0` setup, `1` lock, `2` unlock, `Q` quit.
* Standard age passphrase encryption, scrypt work factor 18 and authenticated ChaCha20-Poly1305.
* Hidden passphrase entry and confirmation. Saves only the selected file path.
* Streaming encryption with decryption verification before original removal.
* Complete authentication before restored plaintext is published.
* No-overwrite publishing, Windows source sharing locks, private staging and explicit conflict errors.
* Moved or renamed `.age` files remain decryptable. Files remain movable and deletable after processing.
* Windows integration tests, automated releases, download badge, dependency notices and SHA-256 checksums.

Supports regular files on local fixed NTFS volumes. See the README for unsupported file types and metadata limits. There is no passphrase recovery, secure erasure or power-loss durability guarantee. Release executables are unsigned.

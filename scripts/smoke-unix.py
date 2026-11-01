"""Exercise the shipped CLI in a real Unix pseudoterminal."""

import os
import pty
import select
import subprocess
import sys
import tempfile
import time
from pathlib import Path


def main() -> None:
    executable = Path(sys.argv[1]).resolve(strict=True)
    scratch = Path(".reference/tmp")
    scratch.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="cli-", dir=scratch) as temporary:
        root = Path(temporary).resolve()
        source = root / "notes.bin"
        original = bytes(range(256)) * 8
        source.write_bytes(original)
        environment = os.environ.copy()
        environment["HOME"] = str(root)
        environment["XDG_CONFIG_HOME"] = str(root / "config")
        master, slave = pty.openpty()
        process = subprocess.Popen(
            [str(executable)], stdin=slave, stdout=slave, stderr=slave, env=environment
        )
        os.close(slave)
        transcript = bytearray()
        pending = bytearray()

        def expect(text: str) -> None:
            marker = text.encode()
            deadline = time.monotonic() + 30
            while marker not in pending:
                remaining = deadline - time.monotonic()
                if remaining <= 0 or not select.select([master], [], [], remaining)[0]:
                    raise AssertionError(f"CLI timed out waiting for {text!r}")
                chunk = os.read(master, 4096)
                if not chunk:
                    raise AssertionError(f"CLI closed before {text!r}")
                transcript.extend(chunk)
                pending.extend(chunk)
                if len(transcript) > 131072:
                    raise AssertionError("Unexpected excessive CLI output")
            del pending[: pending.index(marker) + len(marker)]

        def send(text: str) -> None:
            os.write(master, text.encode() + b"\r")

        password = "test-only CLI passphrase 3491"
        try:
            expect("[Q] Quit")
            send("0")
            expect("Paste the full path")
            send(str(source))
            expect("Path saved")
            send("1")
            expect("Passphrase:")
            send("short")
            expect("at least 12 characters")
            send("1")
            expect("Passphrase:")
            send(password)
            expect("Confirm:")
            started = time.monotonic()
            send(password)
            expect("Done:")
            lock_seconds = time.monotonic() - started
            assert not source.exists(), "Lock left plaintext at the source path"
            vault = Path(str(source) + ".age")
            encrypted = vault.read_bytes()
            send("2")
            expect("Passphrase:")
            send("incorrect test passphrase")
            expect("wrong passphrase")
            assert vault.read_bytes() == encrypted, "Wrong passphrase changed the vault"
            assert not source.exists(), "Wrong passphrase published plaintext"
            send("2")
            expect("Passphrase:")
            started = time.monotonic()
            send(password)
            expect("Done:")
            unlock_seconds = time.monotonic() - started
            assert source.read_bytes() == original, "Round-trip byte mismatch"
            assert not vault.exists(), "Unlock left the encrypted source"
            send("q")
            assert process.wait(timeout=10) == 0, "CLI failed to quit cleanly"
            assert password.encode() not in transcript, "Passphrase was echoed"
            print(
                f"CLI PASS: setup, hidden input, lock, wrong password, unlock, quit. "
                f"Lock {lock_seconds:.2f}s; unlock {unlock_seconds:.2f}s."
            )
        finally:
            if process.poll() is None:
                process.terminate()
                process.wait(timeout=5)
            os.close(master)


if __name__ == "__main__":
    main()

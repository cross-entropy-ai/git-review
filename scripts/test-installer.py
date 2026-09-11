#!/usr/bin/env python3
"""Exercise installation and failure handling without network access."""

import hashlib
import io
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest

INSTALLER = Path(__file__).with_name("install.sh")


class InstallerTest(unittest.TestCase):
    def run_installer(self, system="Darwin", arch="arm64", failure=""):
        with tempfile.TemporaryDirectory(prefix="git-review-installer-") as directory:
            root = Path(directory)
            commands = root / "commands"
            commands.mkdir()
            destination = root / "install with spaces"
            destination.mkdir()
            binary = destination / "git-review"
            binary.write_text("previous version")
            archive = root / "archive.tar.gz"
            with tarfile.open(archive, "w:gz") as tar:
                data = b"#!/bin/sh\necho 'git-review 0.1.0'\n"
                entry = tarfile.TarInfo("git-review")
                entry.size = len(data)
                entry.mode = 0o755
                tar.addfile(entry, io.BytesIO(data))
            checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
            expected_system = "darwin" if system == "Darwin" else "linux"
            expected_arch = "arm64" if arch in ("arm64", "aarch64") else "amd64"
            name = f"git-review_{expected_system}_{expected_arch}.tar.gz"
            if failure == "checksum":
                checksum = "0" * 64
            if failure == "missing":
                name = "another-archive.tar.gz"
            (root / "checksums.txt").write_text(f"{checksum}  {name}\n")
            (commands / "uname").write_text(
                '#!/bin/sh\nif [ "$1" = -s ]; then echo "$TEST_OS"; else echo "$TEST_ARCH"; fi\n'
            )
            (commands / "curl").write_text('''#!/bin/sh
set -eu
url=''
output=''
while [ "$#" -gt 0 ]; do
    case "$1" in
        https://*) url=$1 ;;
        -o) shift; output=$1 ;;
    esac
    shift
done
case "$url" in
    */releases/latest) printf '%s' 'https://github.com/cross-entropy-ai/git-review/releases/tag/v0.1.0' ;;
    */releases/download/v0.1.0/checksums.txt) cp "$TEST_ROOT/checksums.txt" "$output" ;;
    */releases/download/v0.1.0/"$TEST_ASSET")
        [ "$TEST_FAILURE" != download ] || exit 22
        cp "$TEST_ROOT/archive.tar.gz" "$output" ;;
    *) echo "Unexpected URL: $url" >&2; exit 1 ;;
esac
''')
            for command in commands.iterdir():
                command.chmod(0o755)
            env = dict(os.environ, PATH=f"{commands}:{os.environ['PATH']}",
                       INSTALL_DIR=str(destination), TEST_OS=system, TEST_ARCH=arch,
                       TEST_ROOT=str(root), TEST_FAILURE=failure,
                       TEST_ASSET=f"git-review_{expected_system}_{expected_arch}.tar.gz")
            result = subprocess.run(["sh", str(INSTALLER)], env=env, capture_output=True, text=True)
            if failure or system == "FreeBSD" or arch == "riscv64":
                self.assertNotEqual(result.returncode, 0, result.stdout)
                self.assertEqual(binary.read_text(), "previous version")
            else:
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(subprocess.check_output([str(binary)], text=True).strip(), "git-review 0.1.0")
            self.assertEqual(list(destination.iterdir()), [binary])

    def test_platforms(self):
        for system, arch in (("Darwin", "arm64"), ("Darwin", "x86_64"),
                             ("Linux", "aarch64"), ("Linux", "x86_64")):
            with self.subTest(system=system, arch=arch):
                self.run_installer(system, arch)

    def test_failed_download_or_checksum_preserves_existing_installation(self):
        for failure in ("download", "checksum", "missing"):
            with self.subTest(failure=failure):
                self.run_installer(failure=failure)

    def test_unsupported_platforms(self):
        self.run_installer(system="FreeBSD")
        self.run_installer(arch="riscv64")


if __name__ == "__main__":
    unittest.main()

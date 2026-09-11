#!/usr/bin/env python3
"""Build portable release archives, an installer, and SHA-256 checksums."""

import argparse
import gzip
import hashlib
import io
import os
from pathlib import Path
import re
import subprocess
import tarfile

ROOT = Path(__file__).resolve().parent.parent


def build(tag: str, destination: Path) -> None:
    if not re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag):
        raise ValueError("Expected a stable version tag such as v0.1.0")
    version = tag[1:]
    destination.mkdir(parents=True, exist_ok=True)
    checksums = {}
    for system in ("darwin", "linux"):
        for arch in ("amd64", "arm64"):
            name = f"git-review_{system}_{arch}.tar.gz"
            binary = destination / f"git-review-{system}-{arch}"
            env = dict(os.environ, CGO_ENABLED="0", GOOS=system, GOARCH=arch)
            subprocess.run(
                ["go", "build", "-trimpath", "-buildvcs=false",
                 f"-ldflags=-s -w -X main.version={version}",
                 "-o", str(binary), "./cmd"],
                cwd=ROOT, env=env, check=True,
            )
            # Fixed metadata keeps archives reproducible across build hosts.
            with (destination / name).open("wb") as output:
                with gzip.GzipFile(filename="", fileobj=output, mode="wb", mtime=0) as zipped:
                    with tarfile.open(fileobj=zipped, mode="w") as archive:
                        for filename, data, mode in (
                            ("git-review", binary.read_bytes(), 0o755),
                            ("README.md", (ROOT / "README.md").read_bytes(), 0o644),
                        ):
                            entry = tarfile.TarInfo(filename)
                            entry.size = len(data)
                            entry.mode = mode
                            archive.addfile(entry, io.BytesIO(data))
            binary.unlink()
            checksums[name] = hashlib.sha256((destination / name).read_bytes()).hexdigest()
            print(f"Built {name}", flush=True)

    installer = destination / "install.sh"
    installer.write_bytes((ROOT / "scripts" / "install.sh").read_bytes())
    checksums[installer.name] = hashlib.sha256(installer.read_bytes()).hexdigest()
    (destination / "checksums.txt").write_text(
        "".join(f"{checksum}  {name}\n" for name, checksum in sorted(checksums.items()))
    )


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("tag", help="Stable version tag, e.g. v0.1.0")
    args = parser.parse_args()
    try:
        build(args.tag, ROOT / "dist" / args.tag)
    except ValueError as error:
        parser.error(str(error))

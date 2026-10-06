#!/usr/bin/env python3
"""Create and verify a complete, unambiguous SHA-256 release manifest."""
import hashlib
import pathlib
import re
import sys

ARCHIVES = {
    "momentum-server-linux-amd64.tar.gz",
    "momentum-server-linux-arm64.tar.gz",
    "momentum-server-windows-amd64.zip",
    "momentum-server-darwin-amd64.tar.gz",
    "momentum-server-darwin-arm64.tar.gz",
}


def expected(version):
    if not re.fullmatch(r"v[0-9]+\.[0-9]+\.[0-9]+", version):
        raise ValueError("official tag must be vMAJOR.MINOR.PATCH")
    ver = version[1:]
    return ARCHIVES | {
        f"momentum_{ver}_amd64.deb",
        f"momentum_{ver}_arm64.deb",
        f"momentum-{ver}-1.x86_64.rpm",
        f"momentum-{ver}-1.aarch64.rpm",
        f"momentum-{ver}-windows-amd64.msi",
    }


def assets(directory, excluded):
    paths = list(directory.iterdir())
    if any(not path.is_file() or path.is_symlink() for path in paths):
        raise ValueError("asset directory must contain only ordinary files")
    return {path.name: path for path in paths if path.name not in excluded}


def digest(path):
    sha = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            sha.update(block)
    return sha.hexdigest()


def main():
    if len(sys.argv) != 4 or sys.argv[1] not in ("create", "verify"):
        raise ValueError("usage: release-assets.py {create|verify} vX.Y.Z ASSET_DIR")
    mode, version, directory = sys.argv[1:]
    directory = pathlib.Path(directory)
    manifest = directory / "checksums.txt"
    bundle = "checksums.txt.sigstore.json"
    items = assets(directory, {manifest.name, bundle})
    required = expected(version)
    missing = required - items.keys()
    if missing:
        raise ValueError("missing required release assets: " + ", ".join(sorted(missing)))
    if mode == "create":
        if bundle in {p.name for p in directory.iterdir()}:
            raise ValueError("remove stale bundle before creating manifest")
        manifest.write_text("".join(f"{digest(items[name])}  {name}\n" for name in sorted(items)), encoding="ascii")
    else:
        if not manifest.is_file() or not (directory / bundle).is_file():
            raise ValueError("manifest or Sigstore bundle missing")
        lines = manifest.read_text(encoding="ascii").splitlines()
        entries = {}
        for line in lines:
            match = re.fullmatch(r"([0-9a-f]{64})  ([A-Za-z0-9_.-]+)", line)
            if not match or match[2] in entries:
                raise ValueError("malformed or duplicate checksum entry")
            entries[match[2]] = match[1]
        if entries.keys() != items.keys():
            raise ValueError("checksum entries do not exactly match release assets")
        for name, path in items.items():
            if entries[name] != digest(path):
                raise ValueError(f"checksum mismatch: {name}")
    print(f"{mode}: {len(items)} verified release assets")


if __name__ == "__main__":
    try:
        main()
    except (OSError, UnicodeError, ValueError) as exc:
        print(f"release asset gate: {exc}", file=sys.stderr)
        sys.exit(1)

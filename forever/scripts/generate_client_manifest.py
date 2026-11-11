#!/usr/bin/env python3
"""Build a SHA-256 download manifest for an owned WoW Forever client.

No files are copied or uploaded. Publish only files you are authorized to distribute.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
from urllib.parse import quote, urlsplit


def digest(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for block in iter(lambda: f.read(4 * 1024 * 1024), b""):
            h.update(block)
    return h.hexdigest()


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--client", type=Path, required=True, help="Path to existing game client")
    ap.add_argument("--base-url", required=True, help="Public HTTPS base URL ending in /client/")
    ap.add_argument("--launcher-manifest", type=Path, help="Existing manifest to preserve launcher update details")
    ap.add_argument("--output", type=Path, required=True, help="Where to save generated manifest")
    args = ap.parse_args()

    root = args.client.resolve(strict=True)
    if not root.is_dir():
        ap.error("--client must point to a directory")
    uri = urlsplit(args.base_url)
    if uri.scheme != "https" or not uri.hostname or uri.username or uri.password or uri.query or uri.fragment:
        ap.error("--base-url must be an HTTPS URL without credentials, query or fragment")
    base = args.base_url.rstrip("/") + "/"
    sources = [root / "Data", root / "_forever_", root / ".build.info", root / ".product.db"]
    if not (root / "_forever_" / "Wow.exe").is_file():
        ap.error("Missing _forever_/Wow.exe; verify the source client before publishing")

    launcher = None
    if args.launcher_manifest:
        with args.launcher_manifest.open("r", encoding="utf-8-sig") as f:
            launcher = json.load(f).get("launcher")
    # Never publish user-specific account settings, logs, screenshots, and cache.
    # Case-insensitive because files are ultimately installed on Windows.
    excluded_dirs = {"wtf", "cache", "logs", "screenshots", "errors", "crashes"}
    sensitive_suffixes = {".log", ".dmp", ".mdmp", ".key", ".pem", ".p12", ".pfx"}
    files = []
    seen = set()
    for item in sources:
        if item.is_symlink():
            raise RuntimeError("Refusing symlink: " + str(item))
        if item.is_file():
            paths = [item]
        elif item.is_dir():
            paths = []
            for folder, dirs, names in os.walk(item, followlinks=False):
                current = Path(folder)
                dirs[:] = sorted(d for d in dirs if not (current / d).is_symlink() and d.lower() not in excluded_dirs)
                paths.extend(current / n for n in sorted(names))
        else:
            continue
        for path in paths:
            if path.is_symlink() or not path.is_file():
                raise RuntimeError("Refusing nonregular file or symlink: " + str(path))
            rel = path.relative_to(root)
            if any(part.lower() in excluded_dirs for part in rel.parts):
                continue
            if path.suffix.lower() in sensitive_suffixes:
                continue
            if any(part in (".", "..") for part in rel.parts):
                raise RuntimeError("Unsafe path: " + str(rel))
            filename = rel.as_posix()
            if filename in seen:
                raise RuntimeError("Duplicate filename: " + filename)
            seen.add(filename)
            url = base + "/".join(quote(part, safe="") for part in rel.parts)
            size = path.stat().st_size
            print(f"Hashing {filename} ({size} bytes)", flush=True)
            files.append({"path": filename, "url": url, "sha256": digest(path), "size": size})

    files.sort(key=lambda entry: entry["path"])
    if not files or len(files) > 30000:
        raise RuntimeError(f"Invalid file count: {len(files)}")
    manifest = {"schemaVersion": 1, "launcher": launcher, "files": files}
    content = json.dumps(manifest, ensure_ascii=False, indent=2) + "\n"
    if len(content.encode("utf-8")) > 4 * 1024 * 1024:
        raise RuntimeError("Manifest exceeds launcher 4 MiB limit")
    args.output.parent.mkdir(parents=True, exist_ok=True)
    temp = args.output.with_suffix(args.output.suffix + ".tmp")
    temp.write_text(content, encoding="utf-8")
    temp.replace(args.output)
    print(f"Created {args.output} with {len(files)} files, total {sum(f['size'] for f in files):,} bytes")


if __name__ == "__main__":
    main()

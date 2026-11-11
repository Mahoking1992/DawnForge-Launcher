#!/usr/bin/env python3
"""Prepare an independent Forever connector using pinned MIT-licensed upstream.

Only the PUBLIC realm CA certificate is consumed. Never provide a private key.
Source code is obtained by the GitHub Actions workflow at an immutable commit.
"""
from __future__ import annotations
import argparse
import ssl
from pathlib import Path

DEFAULT_SERVER = "13.140.153.237"
UPSTREAM_SOURCE = "windows/forever-launcher.cpp"

def build_profile(pem: str, server: str) -> str:
    if "PRIVATE KEY" in pem or pem.count("-----BEGIN CERTIFICATE-----") != 1 or pem.count("-----END CERTIFICATE-----") != 1:
        raise ValueError("Exactly one PUBLIC CA certificate is required; never supply private keys")
    # This detects malformed PEM. The CA's subject, name constraints and chain must
    # still be verified by OpenSSL / operator before any public release.
    ssl.PEM_cert_to_DER_cert(pem)
    if "\r" in pem:
        pem = pem.replace("\r\n", "\n").replace("\r", "\n")
    lines = [line for line in pem.strip().split("\n")]
    c_pem = " \\\n".join('    "' + line.replace("\\", "\\\\").replace('"', '\\"') + '\\n"' for line in lines)
    if not (server and all(c in "0123456789." for c in server)):
        raise ValueError("Server must be an IPv4 address")
    return f'''// DawnForge Forever private realm profile. Public cert only.
// Built using pinned MIT code from MShkolenko/algalon-launcher.
// Do not publish without a live Windows login test.
#pragma once
#define REALM_SERVER      L"{server}"
#define REALM_PORTAL      L"trinity"
#define REALM_CONFIG      L"DawnForgeForever.wtf"
#define REALM_NAME        L"DawnForge Forever"
#define REALM_LONG_NAME   L"DawnForge Forever"
#define REALM_LONG_NAME_A "DawnForge Forever"
#define REALM_ID          L"dawnforge"
#define REALM_ID_A        "dawnforge"
#define REALM_MUTEX_ID    L"DawnForgeForever"

#define REALM_CA_PEM \\
{c_pem}
#define REALM_UPDATE_REPO   L""
#define REALM_UPDATE_PUBKEY L""
'''

def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--upstream", type=Path, required=True, help="checked-out pinned upstream repo")
    ap.add_argument("--ca", type=Path, required=True, help="public root CA cert")
    ap.add_argument("--server", default=DEFAULT_SERVER, help="server IPv4")
    args = ap.parse_args()
    upstream = args.upstream.resolve(strict=True)
    cert = args.ca.read_text(encoding="ascii")
    profile_dir = upstream / "windows" / "profiles" / "dawnforge"
    profile_dir.mkdir(parents=True, exist_ok=True)
    (profile_dir / "profile.h").write_text(build_profile(cert, args.server), encoding="utf-8")
    (profile_dir / "launcher.rc").write_text("// DawnForge Forever connector (test build, no icon yet)\n", encoding="ascii")
    source = upstream / UPSTREAM_SOURCE
    original = source.read_text(encoding="utf-8")
    old = 'static const wchar_t* DEF_PORTAL = REALM_PORTAL L":21843";'
    if original.count(old) != 1:
        raise RuntimeError("Pinned upstream has changed; refusing unsafe port patch")
    changed = original.replace(old, 'static const wchar_t* DEF_PORTAL = REALM_PORTAL L":1119";')
    source.write_text(changed, encoding="utf-8")
    print("DawnForge connector profile generated; port 1119, portal trinity, custom realm CA. No private keys.")
    print("IMPORTANT: test on Windows before packaging and enabling PLAY.")

if __name__ == "__main__":
    main()

#!/usr/bin/env bash
# Canonical cross-build: GCC MinGW-w64 13.2.0 (posix), binutils 2.41.90.
set -euo pipefail
cd "$(dirname "$0")"
compiler="${CXX:-x86_64-w64-mingw32-g++-posix}"
"$compiler" -std=c++17 -O2 -static -municode -mwindows \
  -DUNICODE -D_UNICODE -Wl,--no-insert-timestamp -I . \
  forever-launcher.cpp -o ../app/forever-connector.exe \
  -lshell32 -lole32 -luser32 -lcrypt32 -lws2_32 -ladvapi32 \
  -lcredui -lbcrypt -lwinhttp -lcomctl32 -lgdi32
sha256sum ../app/forever-connector.exe

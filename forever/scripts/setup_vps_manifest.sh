#!/usr/bin/env bash
# Forever-only static download manifest setup. Publish files only with authorization.
set -Eeuo pipefail

CLIENT="/opt/forever/client"
OUTPUT="/srv/forever-download/manifest.json"
CONF="/etc/nginx/sites-available/dawnforge.eu"
SOURCE_BASE="https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/forever-download-connect-20261111"
TMP="$(mktemp -d /tmp/forever-download-XXXXXX)"
trap 'rm -rf "$TMP"' EXIT

if [[ "$EUID" -ne 0 ]]; then echo "Run as root on the VPS"; exit 1; fi
for p in "$CLIENT/Data" "$CLIENT/_forever_/Wow.exe" "$CLIENT/.build.info" "$CONF"; do
  [[ -e "$p" ]] || { echo "Missing required path: $p"; exit 1; }
done

echo "=== Local HTTPS source check ==="
curl --noproxy '*' -fsS --resolve dawnforge.eu:443:127.0.0.1 \
  https://dawnforge.eu/forever-client/.build.info -o /dev/null
HTTP="$(curl --noproxy '*' -sS --resolve dawnforge.eu:443:127.0.0.1 \
  -r 0-1023 -o /dev/null -w '%{http_code}:%{size_download}' \
  https://dawnforge.eu/forever-client/_forever_/Wow.exe)"
[[ "$HTTP" == "206:1024" ]] || { echo "Range test failed: $HTTP"; exit 1; }

echo "=== Download manifest generator from Forever GitHub branch ==="
curl --retry 3 -fsSL "$SOURCE_BASE/forever/scripts/generate_client_manifest.py" \
  -o "$TMP/generate_client_manifest.py"
python3 -m py_compile "$TMP/generate_client_manifest.py"

echo "=== Hash client files with low priority; no game-data copy ==="
mkdir -p /srv/forever-download
ionice -c3 nice -n 19 python3 "$TMP/generate_client_manifest.py" \
  --client "$CLIENT" \
  --base-url "https://dawnforge.eu/forever-client/" \
  --output "$TMP/manifest.json" \
  > "$TMP/hashing.log" || {
    tail -n 30 "$TMP/hashing.log"
    echo "Manifest generation failed; Nginx remains unchanged."
    exit 1
  }
python3 - "$TMP/manifest.json" <<'PY'
import json, sys, os
p = sys.argv[1]
with open(p, encoding="utf-8") as f:
    data = json.load(f)
files = data.get("files", [])
if not files or not all(item.get("sha256") and item.get("url") for item in files):
    raise SystemExit("Manifest is empty or invalid")
print(f"Files: {len(files)}; total {sum(x.get('size',0) for x in files):,} bytes")
print(f"Manifest JSON: {os.stat(p).st_size:,} bytes")
PY
install -m 0644 "$TMP/manifest.json" "$OUTPUT"

echo "=== Scoped Nginx location for Forever manifest ==="
BACKUP="$CONF.bak-forever-manifest-$(date +%Y%m%d-%H%M%S)"
cp -a "$CONF" "$BACKUP"
python3 - "$CONF" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1])
source = p.read_text()
anchor = "    location ^~ /images/ {"
if source.count(anchor) != 1:
    raise SystemExit("Unexpected Nginx config; no changes.")
if 'location = /forever-client/manifest.json' in source:
    print("Manifest location already exists; preserving existing config.")
    raise SystemExit(0)
if 'location ^~ /forever-client/Data/' not in source or 'location ^~ /forever-client/_forever_/' not in source:
    raise SystemExit("Existing Forever client routes are missing; no changes.")
rules = '''
    # Forever-only client file manifest; this is not the game auth gateway.
    location = /forever-client/manifest.json {
        alias /srv/forever-download/manifest.json;
        default_type application/json;
        add_header Cache-Control "no-store" always;
        limit_except GET { deny all; }
    }
'''
for name in ('WTF', 'Cache', 'Logs', 'Screenshots', 'Errors', 'Crashes'):
    rules += f'''    location ^~ /forever-client/_forever_/{name}/ {{
        return 404;
    }}
'''
p.write_text(source.replace(anchor, rules + "\n" + anchor))
PY

if nginx -t && systemctl reload nginx; then
  echo "Nginx loaded."
else
  cp -a "$BACKUP" "$CONF"
  nginx -t
  systemctl reload nginx || true
  echo "Nginx rejected changes; original config restored."
  exit 1
fi

echo "=== Verify manifest at origin ==="
curl --noproxy '*' -fsS --resolve dawnforge.eu:443:127.0.0.1 \
  https://dawnforge.eu/forever-client/manifest.json \
  -o "$TMP/live.json"
python3 - "$TMP/live.json" <<'PY'
import json, sys
with open(sys.argv[1], encoding="utf-8") as f:
    obj = json.load(f)
print("LIVE manifest entries:", len(obj["files"]))
PY
echo "=== Check public hostname response ==="
curl --noproxy '*' -sS --max-time 20 -o /dev/null \
  -w 'PUBLIC HTTPS: HTTP %{http_code}\n' \
  https://dawnforge.eu/forever-client/manifest.json || true
echo "Backup: $BACKUP"
echo "Forever manifest is deployed; PLAY still requires a realm connector."

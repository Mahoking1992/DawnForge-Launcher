# Forever download endpoint on VPS

Only DawnForge Forever. Do not modify Vanilla worldserver, launcher, or existing website paths.

**Distribute the client only where you have the legal right to do so.** The launcher can also use an already installed, lawfully obtained Forever client.

## Confirmed from operator shell on 2026-11-11

- Client: /opt/forever/client (49 GB).
- Data/ and _forever_/Wow.exe exist; build 1.60.1.70862.
- 68 GB free disk; avoid creating a second 49 GB archive.
- Forever backend runs under forever-world.service and forever-bnet.service.
- Public HTTPS game-auth gateway and client helper have not been verified.

## Step 1: inspect safely, no changes

~~~bash
echo '=== HTTPS VHOST ==='
sudo nginx -T 2>/dev/null | grep -nE 'server_name|listen 443|location |alias ' | head -100
echo '=== CLIENT PERMISSIONS ==='
stat -c '%A %U:%G %n' /opt/forever/client /opt/forever/client/Data /opt/forever/client/_forever_
echo '=== GAME SOCKETS ==='
sudo ss -lntp | grep -E ':(1119|8081|8090)\b' || true
df -h /
~~~

## Step 2: add a *scoped* static location to the EXISTING HTTPS server block

Only after inspecting the vhost, file contents, distribution permissions, and Nginx read access, consider the following example (not a shell command):

~~~nginx
location ^~ /forever-client/ {
    alias /opt/forever/client/;
    autoindex off;
    disable_symlinks on;
    limit_except GET { deny all; }
    add_header X-Content-Type-Options nosniff always;
}
~~~

This publishes files under the alias: review the directory for any secret, unapproved or unrelated files first. Use a dedicated download directory containing only authorized files instead if necessary. Test nginx -t before systemctl reload nginx. Test small HTTPS file and HTTP Range (206 Partial Content) with curl. Do not expose game authentication ports for the purpose of downloading.

## Step 3: generate manifest from existing files (no second 49 GB copy)

Once the source repo and script are available on the VPS:

~~~bash
python3 forever/scripts/generate_client_manifest.py \
  --client /opt/forever/client \
  --base-url https://dawnforge.eu/forever-client/ \
  --output /srv/forever-download/manifest.json
~~~

The URL above is the Forever client URL already configured on the operator's VPS, and tested *locally* with 200 for .build.info and 206 for a 1 KiB Range request on Wow.exe. An external download must still be confirmed. The generator hashes original game files in chunks and produces a JSON list with path, SHA-256, size and HTTPS URL. It does not copy or compress game data. Publish only after confirming every selected file may be distributed and is non-sensitive. The launcher update manifest and its hashes stay separately on GitHub.

## Step 4: launcher and connection

The Forever launcher now has a first-run DOWNLOAD > PLAY flow, partial-file resume and per-file SHA-256 verification. Its **client download manifest** is https://dawnforge.eu/forever-client/manifest.json; **launcher self-updates** remain on https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/forever/update-manifest.json. The HTTPS client manifest must be explicitly exposed by an exact-match Nginx location since the existing fallback /forever-client/ location returns 404. Existing legitimate installs may be selected.

The WoW Forever 1.60 client is not redirected using the Vanilla realmlist. PLAY requires a compatible realm-specific connector and properly configured public authentication gateway, scoped CA certificate, host, and portal. Never publish CA private keys. Reference code: https://github.com/MShkolenko/algalon-launcher ; do not use its Algalon realm profile unchanged.

Test on Windows: download, interruption/resume, SHA-256, Play, login, realm selection and entry. Until these pass, do not advertise the launcher as fully working.

## Step 3A: publish the **manifest only**

Add this exact location to the existing SSL server block once the generated JSON exists and has been reviewed:

~~~nginx
location = /forever-client/manifest.json {
    alias /srv/forever-download/manifest.json;
    default_type application/json;
    add_header Cache-Control "no-store" always;
    limit_except GET { deny all; }
}
~~~

This path must not conflict with the existing fallback `location ^~ /forever-client/ { return 404; }`: exact matches take precedence. Confirm `nginx -t`, reload, and test locally with `curl --noproxy '*' --resolve dawnforge.eu:443:127.0.0.1 ...`.

Do **not** publish files containing passwords, client accounts, credentials, authentication tokens, logs, private keys or private user configuration from the client folder. The generation script enumerates all files in Data/ and _forever_/ by default, so perform a path audit and exclude sensitive paths before deployment.

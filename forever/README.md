# DawnForge Forever Launcher (separate from Vanilla)

A dedicated **Windows launcher** for the independent DawnForge Forever realm. Its files live only under `forever/` and `download/DawnForgeForeverLauncher.exe`. The original Vanilla launcher and its publishing workflows are unchanged.

## Features
- English UI; large neutral Alliance/Horde illustration; opens [dawnforge.eu](https://dawnforge.eu/) rather than showing news.
- No version number for the game client, no PlayerBots mention.
- Website URL, update-manifest URL, client folder, public gateway hostname, port and compatible connection helper can be configured in Options.
- Manifest-driven HTTPS downloads and file repair with SHA-256 verification. Self-update also requires a verified hash.
- Select an **existing legitimately acquired** Forever client; no game client files are hosted by this repository.

## Important connection prerequisite
DawnForge Forever's latest verified backend listeners were bound to `127.0.0.1:1119`, `127.0.0.1:8081` and `127.0.0.1:8090` on its VPS. External access is **not yet verified**. WoW Forever cannot be redirected with a Vanilla-style realmlist change. It needs a compatible connection helper, a proper limited-scope public CA certificate and a secured public gateway. Never distribute the CA's private key.

The [open-source Forever connector reference](https://github.com/MShkolenko/algalon-launcher) includes a Windows realm profile and signed-update design; its example cannot be used unchanged for our realm. The **Play** button intentionally refuses to launch until a compatible helper is configured. The GUI preview is not a claim of verified live game login.

## Manifest format
The file `forever/update-manifest.json` is the editable update feed. Use your own HTTPS links and SHA-256 digests before publishing a client patch:
```json
{
  "schemaVersion": 1,
  "launcher": null,
  "files": [
    {"path": "Data/example.patch", "url": "https://your-host/patch", "sha256": "<64-character sha256>"}
  ]
}
```
Do not publish proprietary client binaries, private keys or credential material.

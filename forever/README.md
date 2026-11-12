# DawnForge Forever Launcher — native Windows build

This repository has **two independent launchers**. The native Forever code is strictly in `forever/`, and the Forever EXE is `download/DawnForgeForeverLauncher.exe`. Do not modify the Vanilla executable or its release workflow.

## Native 0.3.0

The Forever launcher now uses **native Win32 API controls implemented in Go**. It displays the existing approved Alliance/Horde `forever/app/artwork.jpg`. It does **not** extract or execute PowerShell, cmd, policy bypasses, or remote scripts.

Features:
- Windows GUI with original approved artwork, folder picker, live status, website link and PLAY.
- Settings stored in `%LOCALAPPDATA%/DawnForgeForever/Launcher/settings.json` (game folder only).
- Detects a preinstalled client with `_forever_/Wow.exe` and `Data/`.
- DOWNLOAD / REPAIR reads a trusted HTTPS manifest from `https://dawnforge.eu/forever-client/manifest.json`, resumes downloads when supported, validates relative target paths, rejects symlink parents and SHA-256 mismatches.
- CHECK UPDATES uses a separate HTTPS launcher manifest and saves verified new executables under the local `Updates` folder. It does not overwrite or run an executable while the launcher is active.
- PLAY starts `DawnForgeForeverConnector.exe` only if present in the Forever application folder or selected game folder. **No Windows credential collection or game process injection is performed.**

## What is not yet complete

The Forever login/private realm gateway and a compatible connector **have not been verified or packaged**. A client folder alone is insufficient to reach the private realm. Until that dependency is addressed, PLAY reports a clear error rather than pretending the game can connect or launching the official Battle.net session.

The VPS-hosted game manifest may be empty or missing. A DOWNLOAD / REPAIR action will say so rather than offering unauthorized client files.

**The Windows Defender Trojan detection on native 0.3.0 has not yet been independently checked.** A clean build is not an antivirus clearance or a functional connection test. Do not disable Microsoft Defender or instruct users to allow the older flagged EXE.

## Build

On a Windows runner with Go 1.23:

```powershell
cd forever/app
go test ./...
go vet ./...
go build -trimpath -ldflags "-s -w -H windowsgui" -o ../../download/DawnForgeForeverLauncher.exe .
```

The `.github/workflows/build-forever-launcher.yml` workflow also verifies the expected artwork and icon hash, compiles the exe, runs available Defender scanning, uploads the artifact, and publishes only when changes are merged to `main`. The prior PowerShell script is deleted from the native source branch.

## Release checklist

1. CI build, Go tests and static validation pass.
2. Windows Defender locally scans the exact build; investigate any detection rather than suppressing warnings.
3. Launch the EXE on a real Windows desktop, check original artwork, buttons, folder selection, reopen persistence and game manifest logic.
4. Configure and verify the compatible realm connector and public gateway; perform an actual PLAY game login test before advertising the launcher as playable.
5. Only after acceptance, merge and publish the new EXE; keep Vanilla separate.

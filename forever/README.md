# DawnForge Forever launcher

The existing Windows UI, artwork, blue D icon and all buttons are preserved. PLAY starts the embedded DawnForge connector and passes the selected installation folder. No Battle.net application is required. Credentials are entered in WoW, not stored by the launcher.

The connector is vendored from [algalon-launcher](https://github.com/MShkolenko/algalon-launcher) at `8ab061fe8a55b443af4abeccff3b4c6182318670` under MIT; see `connector/UPSTREAM.md` and `connector/LICENSE`. It creates an isolated `_forever_dawnforge_` client, uses `WTF/DawnForgeForever.wtf`, and replaces the TrinityCore region key in runtime memory. It preserves the original client configuration. A first-run UAC helper adds the portal hosts entry (with backup) and the exact realm CA to LocalMachine Root.

TLS validation stays enabled. The public CA permits only `trinity.actual.battle.net` through a critical nameConstraints extension. No private keys are included. Portal: `trinity:1119`; REST: 8081; world: 8090. The worldserver configuration specifies **RealmID 70**; the client preselects `DawnForge Forever`, while the server supplies the numerical realm address.

**Live login remains unverified.** On 2026-11-11 the local BNet TLS handshake passed chain and hostname verification. All three Forever listeners bind to localhost; the candidate public gateway `play.dawnforge.eu:1119` refused the connection. The gateway address, external REST/world advertisements and running server signing-key compatibility need operator confirmation and Windows acceptance. No server services, firewall or production databases were changed.

## Build and provenance

`app/forever-connector.exe` is the checked-in, statically linked Windows x64 connector, embedded by Go together with the existing UI and artwork. The unchanged GitHub Actions workflow can therefore build the complete launcher without installing a new C++ toolchain. Rebuild the connector after every connector source/profile change:

```sh
bash forever/connector/build.sh
# MinGW-w64 GCC 13.2.0 posix; binutils 2.41.90; all linked DLLs are Windows components.
```

Alternatively use `connector/build.cmd` from an x64 Visual Studio developer prompt. MSVC and MinGW builds are not byte-identical. The canonical checked-in binary hash is in `connector/SHA256SUMS`; verify it with `sha256sum -c forever/connector/SHA256SUMS` from the repository root. The original main EXE icon still comes from the unchanged `assets/blue-icon.zip` in the existing workflow.

Existing workflow: `.github/workflows/build-forever-launcher.yml`. It builds `download/DawnForgeForeverLauncher.exe`, uploads **DawnForgeForeverLauncher-Windows**, then automatically commits the EXE and `forever/update-manifest.json`. Publishing therefore writes outside `forever/`; the strict source-only scope requires explicit resolution before triggering this workflow. No workflow or Vanilla files were edited.

Upstream Windows fixture tests are available through `connector/test.cmd`; they do not modify the actual hosts file. They require Windows and are not claimed to have run on Linux.

## Windows acceptance

Follow [WINDOWS-LOGIN-TEST.sk.md](WINDOWS-LOGIN-TEST.sk.md) for exact TCP/TLS, UAC, client, account, realm 70 and in-world acceptance steps. `scripts/test-windows-connection.ps1` tests all three public ports; `-VerifyTls` additionally checks BNet/REST using normal Windows trust validation after setup. Neither script takes credentials or writes to databases.

## Updates and downloads

The HTTPS manifest at `forever/update-manifest.json` retains schemaVersion 1 and SHA-256 file checks. Only legitimately acquired client files are supported; this repository does not distribute the game. Connector self-update is disabled in its profile; it is replaced only as part of the existing outer launcher distribution. No update trust or certificate checks were disabled.

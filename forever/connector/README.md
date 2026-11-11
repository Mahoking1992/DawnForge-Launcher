# DawnForge Forever connector (Windows test build)

This connector is **separate from** the download/install GUI and from DawnForge Vanilla.

## Server values verified 2026-11-11

- IPv4 gateway: `13.140.153.237`
- BNet TLS: `1119`
- Login REST TLS: `8081`
- World: `8090`
- Portal/REST hostname: `trinity.actual.battle.net`
- Realm ID: `70`
- Client: `1.60.1.70862`, executable under `_forever_/Wow.exe`
- CA: self-issued `Forever Local CA`, `CA:TRUE`, pathlen 0, critical Name Constraints `DNS:trinity.actual.battle.net`
- Server TLS certificate chains to the CA and verifies in OpenSSL for 1119/8081.

## Build prerequisite (pending)

The **public certificate only** must be placed at `forever/connector/forever-ca.pem` before the connector can compile. Retrieve its PEM form on the VPS:

~~~bash
openssl x509 -in /opt/forever/config/forever-ca.pem -outform PEM
~~~

This prints only the public certificate; do NOT publish private-key PEM files, DB credentials, live authentication secrets or account passwords. In addition to reading the PEM, verify the CA's critical name constraints and fingerprint against the live server. The workflow confirms that the CA includes the relevant name constraint. **Installation into Windows root store affects the computer and must be clearly disclosed to players and require consent**.

## Implementation

The build is based on public MIT-licensed `MShkolenko/algalon-launcher` pinned at commit `8ab061fe8a55b443af4abeccff3b4c6182318670`. It generates a realm-specific profile and uses port 1119, rather than that project's Algalon-specific port 21843. The build does not use Algalon's certificate, hosts name, portal or update keys. The helper uses a separate client configuration and a scoped root CA, rather than blindly changing a normal WoW realmlist.

The GitHub Action `build-forever-connector.yml` produces a Windows **test artifact only** and does not publish or run the connector automatically.

## Not yet verified

- Public CA PEM has not been supplied in this repository.
- The test connector has not been built.
- No Windows game login to realm 70 has been completed.
- The Forever GUI has no verified connector packaged inside it, so PLAY must not be advertised as working yet.

After a successful game login on Windows, the connector can be packaged into the Forever GUI and its hash confirmed. **Never silently install an unrestricted CA certificate.**

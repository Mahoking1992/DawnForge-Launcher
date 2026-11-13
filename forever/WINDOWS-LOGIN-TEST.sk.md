# Test PLAY → WoW Forever → DawnForge Forever

Stav 2026-11-11: konektor a launcher sú pripravené na Windows akceptačný test. Živé prihlásenie nie je potvrdené. Verejný kandidát `play.dawnforge.eu:1119` odmietol spojenie. BNet 1119, REST 8081 a world 8090 na VPS počúvajú iba na localhost. Firewall, služby a databázy neboli zmenené.

## Podmienky pre správcu servera

Pred testom musí existovať schválená verejná cesta na všetky tri služby. BNet/REST musia prezentovať certifikát pre `trinity.actual.battle.net` podpísaný pribaleným CA. REST odpoveď musí inzerovať adresu dostupnú z Windows a port 8081; realm 70 musí inzerovať dostupnú world adresu a port 8090. Samotné otvorenie BNet portu nestačí. `play.dawnforge.eu` je kandidát brány, ktorého správnosť pre Forever sa musí potvrdiť pred distribúciou. Pri inej adrese zmeniť `connector/profile.h` aj `$defaults.ServerHost` a znovu zostaviť konektor a launcher.

RealmID 70 bol overený v `/opt/forever/config/worldserver.conf`. Databázový záznam realmu ani externé adresy v databáze sa nečítali ani nemenili. Verejný Ed25519 kľúč konektora bol odvodením overený proti podpisovému kľúču v serverovom zdrojovom kóde; žiadny privátny kľúč sa neexportoval. Zhodu s bežiacim binárnym serverom a konkrétnym Windows klientom potvrdí až prihlásenie. `REALM_ID=dawnforge` je identifikátor súborov, nie číselné ID realmu. Klient má predvolený `realmName="DawnForge Forever"`; číselné ID poskytuje server.

## Windows 10/11 x64

1. Použiť testovací účet DawnForge Forever a vlastný kompatibilný klient obsahujúci `_forever_\Wow.exe`. Pripraviť dostatok miesta na oddelenú kópiu klienta. Zavrieť WoW aj Battle.net aplikáciu. Heslo sa nezadáva do launchera ani do skriptov.
2. Stiahnuť artefakt **DawnForgeForeverLauncher-Windows** z konkrétneho nového Actions behu. Overiť SHA-256 proti hodnote z jeho build logu: `Get-FileHash .\DawnForgeForeverLauncher.exe -Algorithm SHA256`. Nepoužiť starý verejný EXE ako dôkaz nového buildu.
3. Zo zdrojov toho istého commitu spustiť `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\forever\scripts\test-windows-connection.ps1`. Vyžadovať tri `PASS TCP`. Pri `FAIL` zastaviť test: ide o verejnú dostupnosť; nevypínať TLS ani Windows firewall.
4. Spustiť launcher, skontrolovať pôvodný obrázok, modrú ikonu D a tlačidlá HOME, WEBSITE, DOWNLOADS, UPDATES, OPTIONS, PLAY aj ovládanie okna. V OPTIONS vybrať priečinok obsahujúci `_forever_\Wow.exe` (možno vybrať aj samotný `_forever_`). Cesta s medzerami musí fungovať.
5. Stlačiť PLAY. Prvý štart môže kopírovať klienta do `_forever_dawnforge_`. UAC požiadavka konektora povoľuje iba opravu hosts záznamu portálu a pridanie konkrétneho CA do LocalMachine Root. Pri odmietnutí UAC sa musí zobraziť chyba a nesmie nasledovať neoverené prihlásenie. Potom PLAY zopakovať a povoliť UAC.
6. Overiť CA odtlačok `317A98F419C4639F4C22E6B5647C5EFD80F5382140E6B345F76BF0965A310B69` a critical nameConstraints iba `DNS:trinity.actual.battle.net`. Hosts musí obsahovať tento jediný portál smerujúci na potvrdenú bránu; záloha je pri hosts ako `.bak-dawnforge-*`.
7. Spustiť rovnaký skript s `-VerifyTls`. Vyžadovať aj dva `PASS TLS` pre BNet a REST. Skript používa bežné Windows overovanie certifikátu a mena bez validačných výnimiek.
8. V hre overiť `WTF\DawnForgeForever.wtf`: `SET portal "trinity:1119"`, správne locale a `SET realmName "DawnForge Forever"`. Proces musí byť `_forever_dawnforge_\Wow.exe -config DawnForgeForever.wtf`. Pôvodný klient a jeho `Config.wtf` musia zostať nedotknuté.
9. Prihlásiť sa vlastným testovacím účtom priamo v hre, vybrať **DawnForge Forever**, dostať sa na výber postavy a vstúpiť do sveta. Správca musí potvrdiť reláciu na worldserveri s RealmID 70, bez zápisov testovacieho skriptu do DB. Iba toto je úspešný test celého cieľa.
10. Skontrolovať `%LOCALAPPDATA%\DawnForgeForever\launcher.log` a klientsky `Logs\Connection.log`: portál `trinity.actual.battle.net`, úspešné nasadenie kľúča, žiadny prechod na EU/US portál. Logy pred zdieľaním anonymizovať. Opakované PLAY počas behu nesmie spustiť druhú prípravu. Po zavretí hry sa stav launchera musí obnoviť; ďalší PLAY má fungovať bez opakovaného UAC.

## Negatívne testy a návrat zmien

- Neplatný priečinok klienta: zrozumiteľná chyba pred spustením konektora.
- Nesprávne heslo: odmietnutie v hre, žiadne heslo v settings.json, launcher.ini ani launcher.log.
- Nedôveryhodný certifikát alebo nesprávny názov: TLS test musí zlyhať. Nepoužívať `-k`, callback prijímajúci každý certifikát ani vypnutie kontroly certifikátov.
- Guard cudzieho prihlasovacieho hosta je prevzatý z upstreamu, ale jeho reakcia pred odoslaním hesla nebola overená v živej relácii. Negatívny test robiť iba bez skutočných prihlasovacích údajov.
- Návrat: zavrieť hru a konektor; odstrániť iba záznam `trinity.actual.battle.net` označený DawnForge z hosts (alebo po kontrole použiť príslušnú zálohu) a iba CA s uvedeným odtlačkom z LocalMachine Root. Voliteľne odstrániť `_forever_dawnforge_` a `%LOCALAPPDATA%\DawnForgeForever`. Neodstraňovať oficiálny klient ani iné certifikáty.

## Zaznamenať výsledok

Zapísať commit, URL Actions behu, SHA-256 EXE, verziu Windows a klienta, TCP/TLS výsledky, správanie PLAY, výber realmu, vstup do sveta a serverové potvrdenie RealmID 70. Zatiaľ neotestované: Windows runtime/UI/UAC, upstream runtime testy, skutočné nasadenie kľúča, guard, autentifikácia, výber realmu a vstup do sveta.

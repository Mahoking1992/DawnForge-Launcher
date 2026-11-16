// Windows regression test: private Forever client must not erase player AddOns on PLAY or rebuild.
// Uses only a temporary mock game folder; never accesses the user's real game, hosts, or registry.
#include "forever-launcher.cpp"
#include <iostream>

static int failures = 0;
static void check(const char* label, bool good) {
    std::cout << (good ? "PASS " : "FAIL ") << label << "\n";
    if (!good) ++failures;
}
static bool mkdir_one(const wstr& path) {
    return CreateDirectoryW(path.c_str(), nullptr) || GetLastError() == ERROR_ALREADY_EXISTS;
}
static bool text_is(const wstr& path, const std::string& expected) {
    std::string got;
    return read_file(path, got) && got == expected;
}

int main() {
    wchar_t tmp[MAX_PATH] = {};
    if (!GetTempPathW(MAX_PATH, tmp)) return 1;
    const wstr root = wstr(tmp) + L"DF-Forever-Addon-Test-" +
        std::to_wstring(GetCurrentProcessId()) + L"-" + std::to_wstring(GetTickCount64());
    const wstr product = root + L"\\_forever_";
    const wstr official_addons = product + L"\\Interface\\AddOns";
    const wstr own = product + OWN_SUFFIX;
    const wstr user_addons = own + L"\\Interface\\AddOns";
    const wstr addon = user_addons + L"\\PlayerAddon";
    const wstr own_wtf = own + L"\\WTF";

    bool ok = mkdir_one(root) &&
        mkdir_one(product) &&
        mkdir_one(product + L"\\Interface") &&
        mkdir_one(official_addons) &&
        write_file(root + L"\\.build.info", "mock-build-70862") &&
        write_file(product + L"\\Wow.exe", "mock-wow") &&
        write_file(product + L"\\wow_loader.dll", "mock-loader") &&
        write_file(official_addons + L"\\BlizzardDefault.toc", "official");
    check("create mock official client", ok);
    if (!ok) { delete_tree_in(root, root); return 1; }

    wstr why;
    check("initial private copy succeeds",
          prepare_own_client(root, product, L"Wow.exe", why) == own);
    check("official addon included on first copy",
          text_is(user_addons + L"\\BlizzardDefault.toc", "official"));

    ok = mkdir_one(addon) &&
        write_file(addon + L"\\PlayerAddon.toc", "player version 1") &&
        write_file(own_wtf + L"\\bindings-cache.wtf", "bind A TURNLEFT");
    check("create player addon and bindings", ok);

    // Players commonly install addons in the ORIGINAL _forever_ client, which
    // is separate from the _forever_dawnforge_ folder actually launched.
    const wstr original_addon = official_addons + L"\\OriginalAddon";
    check("install addon in original Forever folder",
          mkdir_one(original_addon) &&
          write_file(original_addon + L"\\OriginalAddon.toc", "original version 1"));

    why.clear();
    check("adding player addon does not force client rebuild",
          prepare_own_client(root, product, L"Wow.exe", why) == own &&
          !exists(own + L".old"));
    check("player addon survives PLAY",
          text_is(addon + L"\\PlayerAddon.toc", "player version 1"));
    check("addon installed in original folder appears in private launched copy",
          text_is(user_addons + L"\\OriginalAddon\\OriginalAddon.toc", "original version 1"));

    // Simulate a newer addon version installed in the original folder.
    // The next PLAY must synchronize it without rebuilding the whole client.
    Sleep(25);
    check("update addon in original Forever folder",
          write_file(original_addon + L"\\OriginalAddon.toc", "original version 2"));
    why.clear();
    check("original addon update does not rebuild client",
          prepare_own_client(root, product, L"Wow.exe", why) == own &&
          !exists(own + L".old"));
    check("newer original addon version synchronized to private client",
          text_is(user_addons + L"\\OriginalAddon\\OriginalAddon.toc", "original version 2"));

    // An upstream patch legitimately changes the canonical fingerprint.
    check("simulate official client patch",
          write_file(product + L"\\PatchData.bin", "new official patch"));
    why.clear();
    check("updated private copy is built",
          prepare_own_client(root, product, L"Wow.exe", why) == own);
    check("player addon survives client rebuild",
          text_is(addon + L"\\PlayerAddon.toc", "player version 1"));
    check("bindings survive client rebuild",
          text_is(own_wtf + L"\\bindings-cache.wtf", "bind A TURNLEFT"));
    check("new client patch is installed",
          text_is(own + L"\\PatchData.bin", "new official patch"));
    check("official addon remains",
          text_is(user_addons + L"\\BlizzardDefault.toc", "official"));
    check("original player addon survives game patch",
          text_is(user_addons + L"\\OriginalAddon\\OriginalAddon.toc", "original version 2"));

    check("temporary mock client removed", delete_tree_in(root, root));
    std::cout << (failures ? "FAILED " : "ALL PASSED ") << failures << "\n";
    return failures ? 1 : 0;
}

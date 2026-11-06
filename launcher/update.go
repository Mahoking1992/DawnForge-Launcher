package main

import (
    "crypto/sha256"
    "encoding/hex"
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strconv"
    "strings"
    "time"
)

const launcherVersion = "1.0.12"
const launcherManifestURL = "https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/download/launcher-update.json"
const launcherBinaryURL = "https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/download/DawnForgeLauncher.exe"

type launcherRelease struct {
    Version string `json:"version"`
    URL     string `json:"url"`
    SHA256  string `json:"sha256"`
}

func newerVersion(remote, local string) bool {
    a, b := strings.Split(remote, "."), strings.Split(local, ".")
    if len(a) != 3 || len(b) != 3 { return false }
    for i := 0; i < 3; i++ {
        x, err1 := strconv.Atoi(a[i]); y, err2 := strconv.Atoi(b[i])
        if err1 != nil || err2 != nil || x < 0 || y < 0 { return false }
        if x != y { return x > y }
    }
    return false
}

// Installed launchers check for updates before opening the original UI.
// An untrusted or unavailable update is ignored; the installed version still starts.
func updateInstalledLauncher() bool {
    self, err := os.Executable()
    if err != nil { return false }
    local := os.Getenv("LOCALAPPDATA")
    if local == "" { return false }
    expected := filepath.Join(local, "DawnForge", "Launcher", "DawnForgeLauncher.exe")
    if !samePath(self, expected) { return false }

    client := &http.Client{Timeout: 15 * time.Second}
    req, err := http.NewRequest("GET", launcherManifestURL, nil)
    if err != nil { return false }
    req.Header.Set("Cache-Control", "no-cache")
    res, err := client.Do(req)
    if err != nil { return false }
    defer res.Body.Close()
    if res.StatusCode != http.StatusOK { return false }
    var release launcherRelease
    if err := json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&release); err != nil {
        return false
    }
    if !newerVersion(release.Version, launcherVersion) ||
       release.URL != launcherBinaryURL ||
       len(release.SHA256) != 64 { return false }
    expectedHash, err := hex.DecodeString(release.SHA256)
    if err != nil || len(expectedHash) != sha256.Size { return false }

    // Never download arbitrary URLs specified in a manifest.
    downloadClient := &http.Client{Timeout: 2 * time.Minute}
    download, err := downloadClient.Get(launcherBinaryURL)
    if err != nil { return false }
    defer download.Body.Close()
    if download.StatusCode != http.StatusOK || download.ContentLength > 64<<20 { return false }
    stage := expected + ".pending"
    output, err := os.OpenFile(stage, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0600)
    if err != nil { return false }
    hash := sha256.New()
    length, err := io.Copy(io.MultiWriter(output, hash), io.LimitReader(download.Body, (64<<20)+1))
    closeErr := output.Close()
    if err != nil || closeErr != nil || length > 64<<20 ||
       !strings.EqualFold(hex.EncodeToString(hash.Sum(nil)), release.SHA256) {
        os.Remove(stage)
        return false
    }

    script := filepath.Join(filepath.Dir(expected), "apply-update.ps1")
    // Wait for this executable to exit before replacing it. Retain the old
    // executable if every replacement attempt fails.
    body := "param([string]$Old,[string]$New,[int]$ParentPid)\r\n" +
        "try { Wait-Process -Id $ParentPid -Timeout 30 -ErrorAction SilentlyContinue } catch {}\r\n" +
        "for ($i=0; $i -lt 30; $i++) {\r\n" +
        "  try { Copy-Item -LiteralPath $New -Destination $Old -Force -ErrorAction Stop; Remove-Item -LiteralPath $New -Force -ErrorAction SilentlyContinue; Start-Process -FilePath $Old; exit 0 } catch { Start-Sleep -Seconds 1 }\r\n" +
        "}\r\n"
    if err := os.WriteFile(script, []byte(body), 0600); err != nil {
        os.Remove(stage)
        return false
    }
    cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive",
        "-ExecutionPolicy", "Bypass", "-WindowStyle", "Hidden",
        "-File", script, expected, stage, strconv.Itoa(os.Getpid()))
    if err := cmd.Start(); err != nil {
        os.Remove(stage)
        return false
    }
    return true
}

func launcherUpdateInfo() string {
    return fmt.Sprintf("DawnForge Launcher %s", launcherVersion)
}

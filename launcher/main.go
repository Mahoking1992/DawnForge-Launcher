package main

import (
    "archive/zip"
    "bytes"
    "context"
    "crypto/sha256"
    _ "embed"
    "encoding/hex"
    "encoding/json"
    "errors"
    "fmt"
    "image"
    "image/draw"
    _ "image/jpeg"
    "io"
    "net/http"
    "net/url"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "sort"
    "strconv"
    "strings"
    "sync"
    "syscall"
    "time"
    "unsafe"
)

const (
    appVersion = "1.0.10"
    appName    = "DawnForge Launcher"

    configURL     = "https://dawnforge.eu/launcher/client/config.json"
    fallbackZip   = "https://dawnforge.eu/launcher/client/DawnForge-1.12.1.zip"
    fallbackMan   = "https://dawnforge.eu/launcher/client/manifest.json"
    fallbackSite  = "https://dawnforge.eu"
    selfUpdateURL = "https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/download/launcher-update.json"

    baseW = 1458
    baseH = 972
)

//go:embed background.jpg
var backgroundPNG []byte

//go:embed dawnforge.ico
var launcherIcon []byte

var (
    user32   = syscall.NewLazyDLL("user32.dll")
    gdi32    = syscall.NewLazyDLL("gdi32.dll")
    shell32  = syscall.NewLazyDLL("shell32.dll")
    ole32    = syscall.NewLazyDLL("ole32.dll")
    kernel32 = syscall.NewLazyDLL("kernel32.dll")

    procRegisterClassExW   = user32.NewProc("RegisterClassExW")
    procCreateWindowExW    = user32.NewProc("CreateWindowExW")
    procDefWindowProcW     = user32.NewProc("DefWindowProcW")
    procShowWindow         = user32.NewProc("ShowWindow")
    procUpdateWindow       = user32.NewProc("UpdateWindow")
    procGetMessageW        = user32.NewProc("GetMessageW")
    procTranslateMessage   = user32.NewProc("TranslateMessage")
    procDispatchMessageW   = user32.NewProc("DispatchMessageW")
    procPostQuitMessage    = user32.NewProc("PostQuitMessage")
    procBeginPaint         = user32.NewProc("BeginPaint")
    procEndPaint           = user32.NewProc("EndPaint")
    procGetClientRect      = user32.NewProc("GetClientRect")
    procInvalidateRect     = user32.NewProc("InvalidateRect")
    procSetCursor          = user32.NewProc("SetCursor")
    procLoadCursorW        = user32.NewProc("LoadCursorW")
    procSendMessageW       = user32.NewProc("SendMessageW")
    procReleaseCapture     = user32.NewProc("ReleaseCapture")
    procTrackMouseEvent    = user32.NewProc("TrackMouseEvent")
    procGetSystemMetrics   = user32.NewProc("GetSystemMetrics")
    procDestroyWindow      = user32.NewProc("DestroyWindow")
    procMessageBoxW        = user32.NewProc("MessageBoxW")
    procDrawTextW          = user32.NewProc("DrawTextW")
    procFillRect           = user32.NewProc("FillRect")
    procSetProcessDPIAware = user32.NewProc("SetProcessDPIAware")

    procStretchDIBits   = gdi32.NewProc("StretchDIBits")
    procSetBkMode       = gdi32.NewProc("SetBkMode")
    procSetTextColor    = gdi32.NewProc("SetTextColor")
    procCreateFontW     = gdi32.NewProc("CreateFontW")
    procSelectObject    = gdi32.NewProc("SelectObject")
    procDeleteObject    = gdi32.NewProc("DeleteObject")
    procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")

    procShellExecuteW       = shell32.NewProc("ShellExecuteW")
    procSHBrowseForFolderW  = shell32.NewProc("SHBrowseForFolderW")
    procSHGetPathFromIDList = shell32.NewProc("SHGetPathFromIDListW")
    procCoTaskMemFree       = ole32.NewProc("CoTaskMemFree")
    procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

const (
    WS_POPUP          = 0x80000000
    WS_VISIBLE        = 0x10000000
    CS_HREDRAW        = 0x0002
    CS_VREDRAW        = 0x0001
    SW_SHOW           = 5
    SW_MINIMIZE       = 6
    WM_DESTROY        = 0x0002
    WM_PAINT          = 0x000F
    WM_CLOSE          = 0x0010
    WM_SETCURSOR      = 0x0020
    WM_LBUTTONDOWN    = 0x0201
    WM_LBUTTONUP      = 0x0202
    WM_MOUSEMOVE      = 0x0200
    WM_MOUSELEAVE     = 0x02A3
    WM_NCLBUTTONDOWN  = 0x00A1
    HTCAPTION         = 2
    IDC_ARROW         = 32512
    IDC_HAND          = 32649
    TME_LEAVE         = 0x00000002
    SM_CXSCREEN       = 0
    SM_CYSCREEN       = 1
    DIB_RGB_COLORS    = 0
    SRCCOPY           = 0x00CC0020
    BI_RGB            = 0
    TRANSPARENT       = 1
    DT_CENTER         = 0x00000001
    DT_VCENTER        = 0x00000004
    DT_SINGLELINE     = 0x00000020
    DT_LEFT           = 0x00000000
    FW_NORMAL         = 400
    FW_BOLD           = 700
    MB_OK             = 0x00000000
    MB_ICONERROR      = 0x00000010
    BIF_RETURNONLYFSDIRS = 0x00000001
    BIF_NEWDIALOGSTYLE   = 0x00000040
)

type POINT struct{ X, Y int32 }
type RECT struct{ Left, Top, Right, Bottom int32 }
type MSG struct {
    Hwnd     uintptr
    Message  uint32
    WParam   uintptr
    LParam   uintptr
    Time     uint32
    Pt       POINT
    LPrivate uint32
}
type WNDCLASSEX struct {
    CbSize        uint32
    Style         uint32
    LpfnWndProc   uintptr
    CbClsExtra    int32
    CbWndExtra    int32
    HInstance     uintptr
    HIcon         uintptr
    HCursor       uintptr
    HbrBackground uintptr
    LpszMenuName  *uint16
    LpszClassName *uint16
    HIconSm       uintptr
}
type PAINTSTRUCT struct {
    Hdc         uintptr
    FErase      int32
    RcPaint     RECT
    FRestore    int32
    FIncUpdate  int32
    RgbReserved [32]byte
}
type BITMAPINFOHEADER struct {
    Size          uint32
    Width         int32
    Height        int32
    Planes        uint16
    BitCount      uint16
    Compression   uint32
    SizeImage     uint32
    XPelsPerMeter int32
    YPelsPerMeter int32
    ClrUsed       uint32
    ClrImportant  uint32
}
type BITMAPINFO struct{ Header BITMAPINFOHEADER }
type TRACKMOUSEEVENT struct {
    CbSize      uint32
    DwFlags     uint32
    HwndTrack   uintptr
    DwHoverTime uint32
}
type BROWSEINFO struct {
    HwndOwner      uintptr
    PidlRoot       uintptr
    PszDisplayName *uint16
    LpszTitle      *uint16
    UlFlags        uint32
    Lpfn           uintptr
    LParam         uintptr
    IImage         int32
}
type hitRect struct {
    id int
    x1, y1, x2, y2 int
}

const (
    hitNone = iota
    hitSettings
    hitMin
    hitClose
    hitWebsite
    hitInstall
    hitUpdate
    hitPlay
)

var hits = []hitRect{
    {hitSettings, 1307, 8, 1358, 54},
    {hitMin,      1359, 8, 1404, 54},
    {hitClose,    1405, 8, 1457, 54},
    {hitWebsite,  1180, 632, 1427, 697},
    {hitInstall,  24, 716, 378, 822},
    {hitUpdate,   904, 716, 1152, 822},
    {hitPlay,     1164, 716, 1431, 822},
}

type settings struct {
    InstallPath string `json:"installPath"`
}
type remoteConfig struct {
    Version             string `json:"version"`
    LauncherVersion     string `json:"launcherVersion"`
    WebsiteURL          string `json:"websiteUrl"`
    BaseURL             string `json:"baseUrl"`
    ManifestURL         string `json:"manifestUrl"`
    ClientArchiveURL    string `json:"clientArchiveUrl"`
    ClientArchiveSHA256 string `json:"clientArchiveSha256"`
    RealmHost           string `json:"realmHost"`
    RealmPort           int    `json:"realmPort"`
    Realmlist           string `json:"realmlist"`
}
type manifestFile struct {
    Path   string `json:"path"`
    SHA256 string `json:"sha256"`
    Size   int64  `json:"size"`
    Delete bool   `json:"delete,omitempty"`
}
type manifest struct {
    Version string         `json:"version"`
    Files   []manifestFile `json:"files"`
}
type launcherUpdate struct {
    Version string `json:"version"`
    URL     string `json:"url"`
    SHA256  string `json:"sha256"`
}
type uiState struct {
    mu          sync.Mutex
    hwnd        uintptr
    hover       int
    pressed     int
    realmOnline bool
    realmKnown  bool
    busy        bool
    status      string
    detail      string
    progress    float64
    updateMeta  *launcherUpdate
    cfg         remoteConfig
    installPath string
}

var state uiState
var basePixels []byte
var handCursor, arrowCursor uintptr

func main() {
    runtime.LockOSThread()
    procSetProcessDPIAware.Call()

    if err := ensureInstalled(); err != nil {
        messageBox(0, "Installation failed:\n"+err.Error(), MB_OK|MB_ICONERROR)
        return
    }
    if err := loadBackground(); err != nil {
        messageBox(0, "Launcher artwork failed to load:\n"+err.Error(), MB_OK|MB_ICONERROR)
        return
    }

    loadSettings()
    if state.installPath == "" {
        state.installPath = `C:\wow`
        saveSettings()
    }
    if err := createWindow(); err != nil {
        messageBox(0, err.Error(), MB_OK|MB_ICONERROR)
        return
    }

    go statusLoop()
    go checkSelfUpdate()

    var msg MSG
    for {
        r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
        if int32(r) <= 0 {
            break
        }
        procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
        procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
    }
}

func ensureInstalled() error {
    exe, err := os.Executable()
    if err != nil { return err }
    exe, _ = filepath.Abs(exe)
    local := os.Getenv("LOCALAPPDATA")
    if local == "" { return errors.New("LOCALAPPDATA is not available") }
    dir := filepath.Join(local, "DawnForge", "Launcher")
    target := filepath.Join(dir, "DawnForgeLauncher.exe")
    if samePath(exe, target) {
        writeLauncherIcon(dir)
        createShortcuts(target)
        return nil
    }

    if err := os.MkdirAll(dir, 0755); err != nil { return err }
    src, err := os.Open(exe)
    if err != nil { return err }
    defer src.Close()

    tmp := target + ".new"
    dst, err := os.Create(tmp)
    if err != nil { return err }
    if _, err = io.Copy(dst, src); err != nil { dst.Close(); return err }
    if err = dst.Close(); err != nil { return err }
    _ = os.Remove(target)
    if err = os.Rename(tmp, target); err != nil { return err }

    writeLauncherIcon(dir)
    createShortcuts(target)
    cmd := exec.Command(target)
    cmd.Dir = dir
    if err := cmd.Start(); err != nil { return err }
    os.Exit(0)
    return nil
}

func writeLauncherIcon(dir string) {
    if len(launcherIcon) == 0 { return }
    _ = os.WriteFile(filepath.Join(dir, "DawnForge.ico"), launcherIcon, 0644)
}

func createShortcuts(target string) {
    icon := filepath.Join(filepath.Dir(target), "DawnForge.ico")
    ps := fmt.Sprintf(`$ws=New-Object -ComObject WScript.Shell; $d=[Environment]::GetFolderPath('Desktop'); $s=$ws.CreateShortcut((Join-Path $d 'DawnForge Launcher.lnk')); $s.TargetPath='%s'; $s.WorkingDirectory='%s'; $s.IconLocation='%s,0'; $s.Save(); $m=Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'; $s2=$ws.CreateShortcut((Join-Path $m 'DawnForge Launcher.lnk')); $s2.TargetPath='%s'; $s2.WorkingDirectory='%s'; $s2.IconLocation='%s,0'; $s2.Save()`, psQuote(target), psQuote(filepath.Dir(target)), psQuote(icon), psQuote(target), psQuote(filepath.Dir(target)), psQuote(icon))
    _ = exec.Command("powershell.exe", "-NoProfile", "-WindowStyle", "Hidden", "-Command", ps).Run()
}

func psQuote(s string) string { return strings.ReplaceAll(s, "'", "''") }
func samePath(a, b string) bool { return strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) }

func loadBackground() error {
    im, _, err := image.Decode(bytes.NewReader(backgroundPNG))
    if err != nil { return err }
    rgba := image.NewRGBA(image.Rect(0, 0, baseW, baseH))
    draw.Draw(rgba, rgba.Bounds(), im, im.Bounds().Min, draw.Src)
    basePixels = make([]byte, baseW*baseH*4)
    for y := 0; y < baseH; y++ {
        for x := 0; x < baseW; x++ {
            si := rgba.PixOffset(x, y)
            di := (y*baseW+x)*4
            basePixels[di+0] = rgba.Pix[si+2]
            basePixels[di+1] = rgba.Pix[si+1]
            basePixels[di+2] = rgba.Pix[si+0]
            basePixels[di+3] = 255
        }
    }
    return nil
}

func createWindow() error {
    hInst, _, _ := procGetModuleHandleW.Call(0)
    className := utf16Ptr("DawnForgeLauncherV110")
    arrowCursor, _, _ = procLoadCursorW.Call(0, IDC_ARROW)
    handCursor, _, _ = procLoadCursorW.Call(0, IDC_HAND)
    wc := WNDCLASSEX{
        CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})),
        Style: CS_HREDRAW | CS_VREDRAW,
        LpfnWndProc: syscall.NewCallback(wndProc),
        HInstance: hInst,
        HCursor: arrowCursor,
        LpszClassName: className,
    }
    if r, _, e := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
        return fmt.Errorf("RegisterClassExW failed: %v", e)
    }

    sw, _, _ := procGetSystemMetrics.Call(SM_CXSCREEN)
    sh, _, _ := procGetSystemMetrics.Call(SM_CYSCREEN)
    w, h := baseW, baseH
    maxW := int(sw) * 94 / 100
    maxH := int(sh) * 92 / 100
    if w > maxW || h > maxH {
        scaleW := float64(maxW) / float64(baseW)
        scaleH := float64(maxH) / float64(baseH)
        scale := scaleW
        if scaleH < scale { scale = scaleH }
        w = int(float64(baseW) * scale)
        h = int(float64(baseH) * scale)
    }
    x := (int(sw) - w) / 2
    y := (int(sh) - h) / 2

    title := utf16Ptr(appName)
    hwnd, _, e := procCreateWindowExW.Call(
        0,
        uintptr(unsafe.Pointer(className)),
        uintptr(unsafe.Pointer(title)),
        WS_POPUP|WS_VISIBLE,
        uintptr(x), uintptr(y), uintptr(w), uintptr(h),
        0, 0, hInst, 0,
    )
    if hwnd == 0 { return fmt.Errorf("CreateWindowExW failed: %v", e) }

    state.mu.Lock()
    state.hwnd = hwnd
    state.status = "READY TO PLAY"
    state.detail = state.installPath
    state.mu.Unlock()

    procShowWindow.Call(hwnd, SW_SHOW)
    procUpdateWindow.Call(hwnd)
    return nil
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
    switch msg {
    case WM_DESTROY:
        procPostQuitMessage.Call(0)
        return 0
    case WM_CLOSE:
        procDestroyWindow.Call(hwnd)
        return 0
    case WM_PAINT:
        paint(hwnd)
        return 0
    case WM_MOUSEMOVE:
        x, y := mouseBase(hwnd, lParam)
        id := hitAt(x, y)
        state.mu.Lock()
        old := state.hover
        state.hover = id
        state.mu.Unlock()
        if old != id { invalidate(hwnd) }
        if id != hitNone { procSetCursor.Call(handCursor) } else { procSetCursor.Call(arrowCursor) }
        tme := TRACKMOUSEEVENT{CbSize: uint32(unsafe.Sizeof(TRACKMOUSEEVENT{})), DwFlags: TME_LEAVE, HwndTrack: hwnd}
        procTrackMouseEvent.Call(uintptr(unsafe.Pointer(&tme)))
        return 0
    case WM_MOUSELEAVE:
        state.mu.Lock()
        state.hover = hitNone
        state.pressed = hitNone
        state.mu.Unlock()
        invalidate(hwnd)
        return 0
    case WM_LBUTTONDOWN:
        x, y := mouseBase(hwnd, lParam)
        id := hitAt(x, y)
        if id != hitNone {
            state.mu.Lock()
            state.pressed = id
            state.mu.Unlock()
            invalidate(hwnd)
            return 0
        }
        procReleaseCapture.Call()
        procSendMessageW.Call(hwnd, WM_NCLBUTTONDOWN, HTCAPTION, 0)
        return 0
    case WM_LBUTTONUP:
        x, y := mouseBase(hwnd, lParam)
        id := hitAt(x, y)
        state.mu.Lock()
        p := state.pressed
        state.pressed = hitNone
        state.mu.Unlock()
        invalidate(hwnd)
        if id == p && id != hitNone { handleAction(hwnd, id) }
        return 0
    case WM_SETCURSOR:
        return 0
    }
    r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
    return r
}

func handleAction(hwnd uintptr, id int) {
    switch id {
    case hitClose:
        procDestroyWindow.Call(hwnd)
    case hitMin:
        procShowWindow.Call(hwnd, SW_MINIMIZE)
    case hitSettings:
        if p, ok := browseFolder(hwnd); ok {
            state.mu.Lock()
            state.installPath = p
            state.detail = p
            state.mu.Unlock()
            saveSettings()
            invalidate(hwnd)
        }
    case hitWebsite:
        go openWebsite()
    case hitInstall:
        startTask("INSTALLING DAWNFORGE", installGame)
    case hitUpdate:
        state.mu.Lock()
        meta := state.updateMeta
        state.mu.Unlock()
        if meta != nil {
            startTask("UPDATING LAUNCHER", func() error { return performSelfUpdate(*meta) })
        } else {
            startTask("CHECKING GAME FILES", updateGame)
        }
    case hitPlay:
        go playGame()
    }
}

func startTask(label string, fn func() error) {
    state.mu.Lock()
    if state.busy {
        state.mu.Unlock()
        return
    }
    state.busy = true
    state.status = label
    state.detail = "Please wait..."
    state.progress = 0
    hwnd := state.hwnd
    state.mu.Unlock()
    invalidate(hwnd)

    go func() {
        err := fn()
        state.mu.Lock()
        state.busy = false
        if err != nil {
            state.status = "ERROR"
            state.detail = err.Error()
            state.progress = 0
        } else if state.status != "RESTARTING LAUNCHER" {
            state.status = "READY TO PLAY"
            state.detail = state.installPath
            state.progress = 1
        }
        state.mu.Unlock()
        invalidate(hwnd)
        if err != nil { messageBox(hwnd, err.Error(), MB_OK|MB_ICONERROR) }
    }()
}

func paint(hwnd uintptr) {
    var ps PAINTSTRUCT
    hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
    if hdc == 0 { return }
    defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))

    var cr RECT
    procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&cr)))
    cw, ch := int(cr.Right-cr.Left), int(cr.Bottom-cr.Top)

    state.mu.Lock()
    hover := state.hover
    pressed := state.pressed
    online := state.realmOnline
    known := state.realmKnown
    busy := state.busy
    status := state.status
    detail := state.detail
    progress := state.progress
    meta := state.updateMeta
    state.mu.Unlock()

    frame := make([]byte, len(basePixels))
    copy(frame, basePixels)

    fillBGRA(frame, 578, 617, 880, 687, 8, 9, 7, 235)
    fillBGRA(frame, 393, 724, 894, 818, 5, 8, 8, 245)
    fillBGRA(frame, 10, 936, 300, 970, 6, 6, 6, 245)

    if hover != hitNone {
        r := hitByID(hover)
        if r.id != 0 {
            if hover == hitClose {
                blendRect(frame, r.x1, r.y1, r.x2, r.y2, 190, 48, 38, 70)
                borderRect(frame, r.x1, r.y1, r.x2, r.y2, 255, 130, 90)
            } else {
                blendRect(frame, r.x1, r.y1, r.x2, r.y2, 255, 190, 65, 42)
                borderRect(frame, r.x1, r.y1, r.x2, r.y2, 255, 218, 125)
            }
        }
    }
    if pressed != hitNone {
        r := hitByID(pressed)
        if r.id != 0 { blendRect(frame, r.x1, r.y1, r.x2, r.y2, 255, 220, 120, 55) }
    }

    bmi := BITMAPINFO{Header: BITMAPINFOHEADER{
        Size: uint32(unsafe.Sizeof(BITMAPINFOHEADER{})),
        Width: baseW,
        Height: -baseH,
        Planes: 1,
        BitCount: 32,
        Compression: BI_RGB,
        SizeImage: uint32(len(frame)),
    }}
    procStretchDIBits.Call(
        hdc, 0, 0, uintptr(cw), uintptr(ch),
        0, 0, baseW, baseH,
        uintptr(unsafe.Pointer(&frame[0])),
        uintptr(unsafe.Pointer(&bmi)),
        DIB_RGB_COLORS, SRCCOPY,
    )

    sx := float64(cw) / baseW
    sy := float64(ch) / baseH

    drawText(hdc, scaleRect(578, 617, 880, 649, sx, sy), "REALM STATUS", 18, 255, 242, 215, true)
    realm := "OFFLINE"
    rr, gg, bb := 255, 92, 73
    if !known {
        realm = "CHECKING..."
        rr, gg, bb = 226, 188, 98
    }
    if online {
        realm = "●  ONLINE"
        rr, gg, bb = 74, 255, 107
    }
    drawText(hdc, scaleRect(578, 646, 880, 687, sx, sy), realm, 24, rr, gg, bb, true)

    if meta != nil && !busy {
        status = "LAUNCHER UPDATE AVAILABLE"
        detail = "Click UPDATE to install v" + meta.Version
    }
    drawText(hdc, scaleRect(400, 728, 888, 755, sx, sy), status, 16, 255, 250, 230, true)

    bar := scaleRect(411, 760, 820, 782, sx, sy)
    fillWinRect(hdc, bar, 31, 35, 34)
    if progress < 0 { progress = 0 }
    if progress > 1 { progress = 1 }
    fill := bar
    fill.Right = fill.Left + int32(float64(bar.Right-bar.Left)*progress)
    if fill.Right > fill.Left { fillWinRect(hdc, fill, 255, 185, 55) }

    drawText(hdc, scaleRect(397, 787, 890, 815, sx, sy), truncate(detail, 80), 13, 232, 218, 196, true)
    drawText(hdc, scaleRect(24, 941, 295, 970, sx, sy), "DawnForge Launcher   v"+appVersion, 13, 208, 173, 113, false)
}

func mouseBase(hwnd, lParam uintptr) (int, int) {
    x := int(int16(uint16(lParam & 0xffff)))
    y := int(int16(uint16((lParam >> 16) & 0xffff)))
    var r RECT
    procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&r)))
    cw := int(r.Right-r.Left)
    ch := int(r.Bottom-r.Top)
    if cw <= 0 || ch <= 0 { return 0, 0 }
    return x * baseW / cw, y * baseH / ch
}
func hitAt(x, y int) int {
    for _, r := range hits {
        if x >= r.x1 && x <= r.x2 && y >= r.y1 && y <= r.y2 { return r.id }
    }
    return hitNone
}
func hitByID(id int) hitRect {
    for _, r := range hits {
        if r.id == id { return r }
    }
    return hitRect{}
}
func invalidate(hwnd uintptr) { procInvalidateRect.Call(hwnd, 0, 0) }

func fillBGRA(p []byte, x1, y1, x2, y2 int, r, g, b, a uint8) {
    if x1 < 0 { x1 = 0 }
    if y1 < 0 { y1 = 0 }
    if x2 > baseW { x2 = baseW }
    if y2 > baseH { y2 = baseH }
    for y := y1; y < y2; y++ {
        for x := x1; x < x2; x++ {
            i := (y*baseW+x)*4
            p[i] = b
            p[i+1] = g
            p[i+2] = r
            p[i+3] = a
        }
    }
}
func blendRect(p []byte, x1, y1, x2, y2 int, r, g, b, a uint8) {
    aa := int(a)
    inv := 255 - aa
    if x1 < 0 { x1 = 0 }
    if y1 < 0 { y1 = 0 }
    if x2 > baseW { x2 = baseW }
    if y2 > baseH { y2 = baseH }
    for y := y1; y < y2; y++ {
        for x := x1; x < x2; x++ {
            i := (y*baseW+x)*4
            p[i] = uint8((int(p[i])*inv + int(b)*aa) / 255)
            p[i+1] = uint8((int(p[i+1])*inv + int(g)*aa) / 255)
            p[i+2] = uint8((int(p[i+2])*inv + int(r)*aa) / 255)
        }
    }
}
func borderRect(p []byte, x1, y1, x2, y2 int, r, g, b uint8) {
    for t := 0; t < 2; t++ {
        for x := x1; x < x2; x++ {
            setPix(p, x, y1+t, r, g, b)
            setPix(p, x, y2-1-t, r, g, b)
        }
        for y := y1; y < y2; y++ {
            setPix(p, x1+t, y, r, g, b)
            setPix(p, x2-1-t, y, r, g, b)
        }
    }
}
func setPix(p []byte, x, y int, r, g, b uint8) {
    if x < 0 || x >= baseW || y < 0 || y >= baseH { return }
    i := (y*baseW+x)*4
    p[i] = b
    p[i+1] = g
    p[i+2] = r
    p[i+3] = 255
}

func scaleRect(x1, y1, x2, y2 int, sx, sy float64) RECT {
    return RECT{int32(float64(x1)*sx), int32(float64(y1)*sy), int32(float64(x2)*sx), int32(float64(y2)*sy)}
}
func rgb(r, g, b int) uintptr { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }
func fillWinRect(hdc uintptr, r RECT, rr, gg, bb int) {
    br, _, _ := procCreateSolidBrush.Call(rgb(rr, gg, bb))
    if br != 0 {
        procFillRect.Call(hdc, uintptr(unsafe.Pointer(&r)), br)
        procDeleteObject.Call(br)
    }
}
func drawText(hdc uintptr, r RECT, text string, size int, rr, gg, bb int, bold bool) {
    weight := FW_NORMAL
    if bold { weight = FW_BOLD }
    face := utf16Ptr("Georgia")
    font, _, _ := procCreateFontW.Call(uintptr(int32(-size)), 0, 0, 0, uintptr(weight), 0, 0, 0, 1, 0, 0, 5, 0, uintptr(unsafe.Pointer(face)))
    old := uintptr(0)
    if font != 0 { old, _, _ = procSelectObject.Call(hdc, font) }
    procSetBkMode.Call(hdc, TRANSPARENT)
    procSetTextColor.Call(hdc, rgb(rr, gg, bb))
    t := utf16Ptr(text)
    flags := uintptr(DT_VCENTER | DT_SINGLELINE)
    if r.Right-r.Left > 100 { flags |= DT_CENTER } else { flags |= DT_LEFT }
    procDrawTextW.Call(hdc, uintptr(unsafe.Pointer(t)), uintptr(^uint32(0)), uintptr(unsafe.Pointer(&r)), flags)
    if font != 0 {
        procSelectObject.Call(hdc, old)
        procDeleteObject.Call(font)
    }
}

func settingsPath() string {
    dir := filepath.Join(os.Getenv("APPDATA"), "DawnForgeLauncher")
    _ = os.MkdirAll(dir, 0755)
    return filepath.Join(dir, "settings.json")
}
func loadSettings() {
    b, err := os.ReadFile(settingsPath())
    if err != nil { return }
    var s settings
    if json.Unmarshal(b, &s) == nil { state.installPath = s.InstallPath }
}
func saveSettings() {
    state.mu.Lock()
    p := state.installPath
    state.mu.Unlock()
    b, _ := json.MarshalIndent(settings{InstallPath: p}, "", "  ")
    _ = os.WriteFile(settingsPath(), b, 0644)
}

func browseFolder(hwnd uintptr) (string, bool) {
    title := utf16Ptr("Choose where DawnForge should be installed")
    display := make([]uint16, 260)
    bi := BROWSEINFO{
        HwndOwner: hwnd,
        PszDisplayName: &display[0],
        LpszTitle: title,
        UlFlags: BIF_RETURNONLYFSDIRS | BIF_NEWDIALOGSTYLE,
    }
    pidl, _, _ := procSHBrowseForFolderW.Call(uintptr(unsafe.Pointer(&bi)))
    if pidl == 0 { return "", false }
    defer procCoTaskMemFree.Call(pidl)

    path := make([]uint16, 32768)
    ok, _, _ := procSHGetPathFromIDList.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
    if ok == 0 { return "", false }
    return syscall.UTF16ToString(path), true
}

func openWebsite() {
    cfg, _ := fetchConfig()
    site := cfg.WebsiteURL
    if site == "" { site = fallbackSite }
    op := utf16Ptr("open")
    u := utf16Ptr(site)
    procShellExecuteW.Call(0, uintptr(unsafe.Pointer(op)), uintptr(unsafe.Pointer(u)), 0, 0, SW_SHOW)
}

func fetchConfig() (remoteConfig, error) {
    var c remoteConfig
    if err := getJSON(configURL, &c); err != nil { return c, err }
    if c.ManifestURL == "" { c.ManifestURL = fallbackMan }
    if c.ClientArchiveURL == "" { c.ClientArchiveURL = fallbackZip }
    if c.WebsiteURL == "" { c.WebsiteURL = fallbackSite }
    state.mu.Lock()
    state.cfg = c
    state.mu.Unlock()
    return c, nil
}
func getJSON(u string, v any) error {
    ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
    defer cancel()
    req, _ := http.NewRequestWithContext(ctx, "GET", u, nil)
    resp, err := http.DefaultClient.Do(req)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("server returned %s", resp.Status) }
    return json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(v)
}

func statusLoop() {
    for {
        checkRealm()
        time.Sleep(30 * time.Second)
    }
}
func checkRealm() {
    type st struct{ Status string `json:"status"` }
    var s st
    err := getJSON("https://dawnforge.eu/api/status", &s)
    online := err == nil && strings.EqualFold(s.Status, "online")
    state.mu.Lock()
    state.realmKnown = true
    state.realmOnline = online
    hwnd := state.hwnd
    state.mu.Unlock()
    invalidate(hwnd)
}

func installGame() error {
    cfg, err := fetchConfig()
    if err != nil { return fmt.Errorf("Update server unavailable: %w", err) }
    state.mu.Lock()
    dir := state.installPath
    state.mu.Unlock()
    if dir == "" { return errors.New("Choose install location first") }
    if err := os.MkdirAll(dir, 0755); err != nil { return err }

    tmp := filepath.Join(os.TempDir(), "DawnForge-1.12.1.zip")
    if err := downloadFile(cfg.ClientArchiveURL, tmp, cfg.ClientArchiveSHA256, "DOWNLOADING GAME FILES"); err != nil { return err }

    stateSet("INSTALLING GAME FILES", "Extracting Vanilla 1.12.1...", 0.92)
    if err := unzip(tmp, dir); err != nil { return err }
    _ = os.Remove(tmp)

    if cfg.Realmlist != "" { _ = writeRealmlist(dir, cfg.Realmlist) }
    if _, err := os.Stat(filepath.Join(dir, "WoW.exe")); err != nil { return errors.New("WoW.exe could not be found after installation") }
    return updateGame()
}

func updateGame() error {
    cfg, err := fetchConfig()
    if err != nil { return fmt.Errorf("Update server unavailable - game files unchanged: %w", err) }

    var man manifest
    if err := getJSON(cfg.ManifestURL, &man); err != nil { return err }

    state.mu.Lock()
    dir := state.installPath
    state.mu.Unlock()
    if dir == "" { return errors.New("Choose install location first") }
    if _, err := os.Stat(dir); err != nil { return errors.New("Game is not installed yet") }

    files := append([]manifestFile(nil), man.Files...)
    sort.SliceStable(files, func(i, j int) bool { return files[i].Path < files[j].Path })
    changed := 0

    for i, f := range files {
        rel := filepath.Clean(filepath.FromSlash(f.Path))
        if rel == "." || strings.HasPrefix(rel, "..") || filepath.IsAbs(rel) { continue }
        dst := filepath.Join(dir, rel)

        if f.Delete {
            _ = os.Remove(dst)
            continue
        }

        good := false
        if st, err := os.Stat(dst); err == nil && st.Mode().IsRegular() {
            if f.Size <= 0 || st.Size() == f.Size {
                if f.SHA256 == "" {
                    good = true
                } else if h, e := fileSHA(dst); e == nil && strings.EqualFold(h, f.SHA256) {
                    good = true
                }
            }
        }
        if good { continue }

        changed++
        stateSet("UPDATING GAME FILES", fmt.Sprintf("%s   %d/%d", f.Path, i+1, len(files)), float64(i)/float64(max(1, len(files))))

        if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil { return err }
        u := joinURL(cfg.BaseURL, f.Path)
        if u == "" { return errors.New("manifest baseUrl is missing") }

        tmp := dst + ".dfupdate"
        if err := downloadFile(u, tmp, f.SHA256, "UPDATING GAME FILES"); err != nil { return err }
        _ = os.Remove(dst)
        if err := os.Rename(tmp, dst); err != nil { return err }
    }

    if cfg.Realmlist != "" { _ = writeRealmlist(dir, cfg.Realmlist) }

    if changed == 0 {
        stateSet("GAME IS UP TO DATE", "All game files verified.", 1)
    } else {
        stateSet("UPDATE COMPLETE", fmt.Sprintf("%d file(s) updated", changed), 1)
    }
    time.Sleep(700 * time.Millisecond)
    return nil
}

func joinURL(base, p string) string {
    if base == "" { return "" }
    u, err := url.Parse(base)
    if err != nil { return "" }
    u.Path = strings.TrimRight(u.Path, "/") + "/" + strings.TrimLeft(strings.ReplaceAll(p, "\\", "/"), "/")
    return u.String()
}
func writeRealmlist(dir, s string) error {
    if !strings.Contains(strings.ToLower(s), "set realmlist") {
        s = "set realmlist " + strings.TrimSpace(s)
    }
    return os.WriteFile(filepath.Join(dir, "realmlist.wtf"), []byte(strings.TrimSpace(s)+"\r\n"), 0644)
}

func downloadFile(u, dst, want, label string) error {
    resp, err := http.Get(u)
    if err != nil { return err }
    defer resp.Body.Close()
    if resp.StatusCode < 200 || resp.StatusCode >= 300 { return fmt.Errorf("download failed: %s", resp.Status) }

    if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil { return err }
    f, err := os.Create(dst)
    if err != nil { return err }
    defer f.Close()

    h := sha256.New()
    w := io.MultiWriter(f, h)
    buf := make([]byte, 256*1024)
    var done int64
    last := time.Now()

    for {
        n, e := resp.Body.Read(buf)
        if n > 0 {
            if _, er := w.Write(buf[:n]); er != nil { return er }
            done += int64(n)
            if time.Since(last) > 180*time.Millisecond {
                p := 0.0
                if resp.ContentLength > 0 { p = float64(done) / float64(resp.ContentLength) }
                stateSet(label, fmt.Sprintf("%.2f MB", float64(done)/(1024*1024)), p*0.9)
                last = time.Now()
            }
        }
        if e == io.EOF { break }
        if e != nil { return e }
    }

    got := hex.EncodeToString(h.Sum(nil))
    if want != "" && !strings.EqualFold(strings.TrimSpace(want), got) {
        _ = os.Remove(dst)
        return errors.New("download verification failed (SHA-256 mismatch)")
    }
    return nil
}
func fileSHA(p string) (string, error) {
    f, e := os.Open(p)
    if e != nil { return "", e }
    defer f.Close()
    h := sha256.New()
    _, e = io.Copy(h, f)
    if e != nil { return "", e }
    return hex.EncodeToString(h.Sum(nil)), nil
}
func unzip(src, dst string) error {
    r, e := zip.OpenReader(src)
    if e != nil { return e }
    defer r.Close()

    root := filepath.Clean(dst) + string(os.PathSeparator)
    for _, f := range r.File {
        name := filepath.Clean(filepath.Join(dst, filepath.FromSlash(f.Name)))
        if !strings.HasPrefix(name, root) && name != filepath.Clean(dst) { return errors.New("invalid archive path") }

        if f.FileInfo().IsDir() {
            if e := os.MkdirAll(name, 0755); e != nil { return e }
            continue
        }
        if e := os.MkdirAll(filepath.Dir(name), 0755); e != nil { return e }
        rc, e := f.Open()
        if e != nil { return e }
        out, e := os.OpenFile(name, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
        if e != nil { rc.Close(); return e }
        _, e = io.Copy(out, rc)
        out.Close()
        rc.Close()
        if e != nil { return e }
    }
    return nil
}

func playGame() {
    state.mu.Lock()
    dir := state.installPath
    if state.busy {
        state.mu.Unlock()
        return
    }
    state.status = "STARTING GAME"
    state.detail = "Entering Azeroth..."
    state.progress = 1
    hwnd := state.hwnd
    state.mu.Unlock()
    invalidate(hwnd)

    wow := filepath.Join(dir, "WoW.exe")
    if _, err := os.Stat(wow); err != nil {
        messageBox(hwnd, "WoW.exe could not be found in the selected DawnForge folder.", MB_OK|MB_ICONERROR)
        stateSet("READY TO PLAY", dir, 0)
        return
    }
    cmd := exec.Command(wow)
    cmd.Dir = dir
    if err := cmd.Start(); err != nil {
        messageBox(hwnd, err.Error(), MB_OK|MB_ICONERROR)
        stateSet("READY TO PLAY", dir, 0)
        return
    }
    time.Sleep(600 * time.Millisecond)
    procDestroyWindow.Call(hwnd)
}

func checkSelfUpdate() {
    var m launcherUpdate
    if err := getJSON(selfUpdateURL, &m); err != nil { return }
    if versionGreater(m.Version, appVersion) && m.URL != "" {
        state.mu.Lock()
        state.updateMeta = &m
        hwnd := state.hwnd
        state.mu.Unlock()
        invalidate(hwnd)
    }
}
func versionGreater(a, b string) bool {
    pa := versionParts(a)
    pb := versionParts(b)
    for i := 0; i < 3; i++ {
        if pa[i] != pb[i] { return pa[i] > pb[i] }
    }
    return false
}
func versionParts(s string) [3]int {
    s = strings.TrimPrefix(strings.TrimSpace(s), "v")
    p := strings.Split(s, ".")
    var out [3]int
    for i := 0; i < len(p) && i < 3; i++ { out[i], _ = strconv.Atoi(p[i]) }
    return out
}
func performSelfUpdate(m launcherUpdate) error {
    exe, err := os.Executable()
    if err != nil { return err }

    tmp := filepath.Join(os.TempDir(), "DawnForgeLauncher_"+m.Version+".exe")
    if err := downloadFile(m.URL, tmp, m.SHA256, "DOWNLOADING LAUNCHER UPDATE"); err != nil { return err }

    bat := filepath.Join(os.TempDir(), "DawnForgeLauncher_update.cmd")
    content := fmt.Sprintf("@echo off\r\nping 127.0.0.1 -n 3 >nul\r\nmove /Y \"%s\" \"%s\" >nul\r\nstart \"\" \"%s\"\r\ndel \"%%~f0\"\r\n", tmp, exe, exe)
    if err := os.WriteFile(bat, []byte(content), 0644); err != nil { return err }

    stateSet("RESTARTING LAUNCHER", "Installing v"+m.Version+"...", 1)
    cmd := exec.Command("cmd.exe", "/C", bat)
    cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
    if err := cmd.Start(); err != nil { return err }
    time.Sleep(300 * time.Millisecond)
    os.Exit(0)
    return nil
}

func stateSet(status, detail string, p float64) {
    state.mu.Lock()
    state.status = status
    state.detail = detail
    state.progress = p
    hwnd := state.hwnd
    state.mu.Unlock()
    invalidate(hwnd)
}
func messageBox(hwnd uintptr, text string, flags uintptr) {
    t := utf16Ptr(text)
    c := utf16Ptr(appName)
    procMessageBoxW.Call(hwnd, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), flags)
}
func utf16Ptr(s string) *uint16 {
    p, _ := syscall.UTF16PtrFromString(s)
    return p
}
func truncate(s string, n int) string {
    r := []rune(s)
    if len(r) <= n { return s }
    return string(r[:n-1]) + "…"
}
func max(a, b int) int {
    if a > b { return a }
    return b
}

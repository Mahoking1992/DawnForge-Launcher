package main

import (
    _ "embed"
    "errors"
    "fmt"
    "io"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "strings"
    "syscall"
    "time"
    "unsafe"
)

const (
    appName = "DawnForge Launcher"
    baseW   = 1458
    baseH   = 972
)

//go:embed DawnForgeOriginal.exe
var originalLauncher []byte

var (
    user32   = syscall.NewLazyDLL("user32.dll")
    kernel32 = syscall.NewLazyDLL("kernel32.dll")
    gdi32    = syscall.NewLazyDLL("gdi32.dll")

    procEnumWindows                = user32.NewProc("EnumWindows")
    procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
    procIsWindowVisible            = user32.NewProc("IsWindowVisible")
    procIsIconic                   = user32.NewProc("IsIconic")
    procGetWindowRect              = user32.NewProc("GetWindowRect")
    procGetCursorPos               = user32.NewProc("GetCursorPos")
    procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
    procRegisterClassExW           = user32.NewProc("RegisterClassExW")
    procCreateWindowExW            = user32.NewProc("CreateWindowExW")
    procDefWindowProcW             = user32.NewProc("DefWindowProcW")
    procShowWindow                 = user32.NewProc("ShowWindow")
    procSetWindowPos               = user32.NewProc("SetWindowPos")
    procSetLayeredWindowAttributes = user32.NewProc("SetLayeredWindowAttributes")
    procDestroyWindow              = user32.NewProc("DestroyWindow")

    procCreateSolidBrush = gdi32.NewProc("CreateSolidBrush")
    procDeleteObject     = gdi32.NewProc("DeleteObject")
    procGetModuleHandleW = kernel32.NewProc("GetModuleHandleW")
)

const (
    WS_POPUP            = 0x80000000
    WS_EX_LAYERED       = 0x00080000
    WS_EX_TRANSPARENT   = 0x00000020
    WS_EX_TOOLWINDOW    = 0x00000080
    WS_EX_NOACTIVATE    = 0x08000000
    SW_HIDE             = 0
    SW_SHOWNOACTIVATE   = 4
    SWP_NOACTIVATE      = 0x0010
    SWP_SHOWWINDOW      = 0x0040
    LWA_ALPHA           = 0x00000002
    CS_HREDRAW          = 0x0002
    CS_VREDRAW          = 0x0001
)

var hwndTop = ^uintptr(0)

type POINT struct {
    X, Y int32
}

type RECT struct {
    Left, Top, Right, Bottom int32
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

type hitRect struct {
    x1, y1, x2, y2 int
}

var hits = []hitRect{
    {1307, 8, 1358, 54},
    {1359, 8, 1404, 54},
    {1405, 8, 1457, 54},
    {1180, 632, 1427, 697},
    {24, 716, 378, 822},
    {904, 716, 1152, 822},
    {1164, 716, 1431, 822},
}

func main() {
    runtime.LockOSThread()

    installed, core, err := ensureInstalled()
    if err != nil {
        messageBoxFallback("DawnForge Launcher: " + err.Error())
        return
    }
    if installed {
        os.Exit(0)
    }

    cmd := exec.Command(core)
    cmd.Dir = filepath.Dir(core)
    if err := cmd.Start(); err != nil {
        messageBoxFallback("DawnForge Launcher: " + err.Error())
        return
    }

    originalHwnd := waitForWindow(uint32(cmd.Process.Pid), 15*time.Second)
    if originalHwnd == 0 {
        _ = cmd.Process.Kill()
        messageBoxFallback("DawnForge Launcher window could not be found.")
        return
    }

    overlay, brush, err := createOverlay()
    if err != nil {
        _ = cmd.Wait()
        return
    }
    defer func() {
        procDestroyWindow.Call(overlay)
        if brush != 0 {
            procDeleteObject.Call(brush)
        }
    }()

    done := make(chan struct{})
    go func() {
        _ = cmd.Wait()
        close(done)
    }()

    tick := time.NewTicker(25 * time.Millisecond)
    defer tick.Stop()

    last := -1
    for {
        select {
        case <-done:
            procShowWindow.Call(overlay, SW_HIDE)
            return
        case <-tick.C:
            idx, r, ok := hoveredRect(originalHwnd)
            if !ok {
                if last != -1 {
                    procShowWindow.Call(overlay, SW_HIDE)
                    last = -1
                }
                continue
            }
            last = idx
            x, y, w, h := scaleRectToWindow(originalHwnd, r)
            if w <= 0 || h <= 0 {
                continue
            }
            procSetWindowPos.Call(
                overlay,
                hwndTop,
                uintptr(x),
                uintptr(y),
                uintptr(w),
                uintptr(h),
                SWP_NOACTIVATE|SWP_SHOWWINDOW,
            )
            procShowWindow.Call(overlay, SW_SHOWNOACTIVATE)
        }
    }
}

func ensureInstalled() (bool, string, error) {
    local := os.Getenv("LOCALAPPDATA")
    if local == "" {
        return false, "", errors.New("LOCALAPPDATA is unavailable")
    }

    dir := filepath.Join(local, "DawnForge", "Launcher")
    wrapper := filepath.Join(dir, "DawnForgeLauncher.exe")
    core := filepath.Join(dir, "DawnForgeOriginal.exe")

    self, err := os.Executable()
    if err != nil {
        return false, "", err
    }
    self, _ = filepath.Abs(self)

    if !samePath(self, wrapper) {
        if err := os.MkdirAll(dir, 0755); err != nil {
            return false, "", err
        }
        if err := copyFile(self, wrapper); err != nil {
            return false, "", err
        }
        if err := os.WriteFile(core, originalLauncher, 0755); err != nil {
            return false, "", err
        }

        createShortcuts(wrapper, core)

        c := exec.Command(wrapper)
        c.Dir = dir
        if err := c.Start(); err != nil {
            return false, "", err
        }
        return true, core, nil
    }

    st, err := os.Stat(core)
    if err != nil || st.Size() != int64(len(originalLauncher)) {
        if err := os.WriteFile(core, originalLauncher, 0755); err != nil {
            return false, "", err
        }
    }

    createShortcuts(wrapper, core)
    return false, core, nil
}

func copyFile(src, dst string) error {
    in, err := os.Open(src)
    if err != nil {
        return err
    }
    defer in.Close()

    tmp := dst + ".new"
    out, err := os.Create(tmp)
    if err != nil {
        return err
    }

    if _, err = io.Copy(out, in); err != nil {
        out.Close()
        return err
    }
    if err = out.Close(); err != nil {
        return err
    }

    _ = os.Remove(dst)
    return os.Rename(tmp, dst)
}

func createShortcuts(wrapper, iconSource string) {
    ps := fmt.Sprintf(
        `$ws=New-Object -ComObject WScript.Shell; $d=[Environment]::GetFolderPath('Desktop'); $s=$ws.CreateShortcut((Join-Path $d 'DawnForge Launcher.lnk')); $s.TargetPath='%s'; $s.WorkingDirectory='%s'; $s.IconLocation='%s,0'; $s.Save(); $m=Join-Path $env:APPDATA 'Microsoft\Windows\Start Menu\Programs'; $s2=$ws.CreateShortcut((Join-Path $m 'DawnForge Launcher.lnk')); $s2.TargetPath='%s'; $s2.WorkingDirectory='%s'; $s2.IconLocation='%s,0'; $s2.Save()`,
        psQuote(wrapper),
        psQuote(filepath.Dir(wrapper)),
        psQuote(iconSource),
        psQuote(wrapper),
        psQuote(filepath.Dir(wrapper)),
        psQuote(iconSource),
    )
    _ = exec.Command(
        "powershell.exe",
        "-NoProfile",
        "-WindowStyle",
        "Hidden",
        "-Command",
        ps,
    ).Run()
}

func psQuote(s string) string {
    return strings.ReplaceAll(s, "'", "''")
}

func samePath(a, b string) bool {
    return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

func waitForWindow(pid uint32, timeout time.Duration) uintptr {
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if h := findWindowForPID(pid); h != 0 {
            return h
        }
        time.Sleep(80 * time.Millisecond)
    }
    return 0
}

func findWindowForPID(pid uint32) uintptr {
    var found uintptr

    cb := syscall.NewCallback(func(hwnd uintptr, lParam uintptr) uintptr {
        var p uint32
        procGetWindowThreadProcessId.Call(
            hwnd,
            uintptr(unsafe.Pointer(&p)),
        )
        if p == pid {
            v, _, _ := procIsWindowVisible.Call(hwnd)
            if v != 0 {
                found = hwnd
                return 0
            }
        }
        return 1
    })

    procEnumWindows.Call(cb, 0)
    return found
}

func hoveredRect(hwnd uintptr) (int, hitRect, bool) {
    vis, _, _ := procIsWindowVisible.Call(hwnd)
    if vis == 0 {
        return -1, hitRect{}, false
    }

    iconic, _, _ := procIsIconic.Call(hwnd)
    if iconic != 0 {
        return -1, hitRect{}, false
    }

    fg, _, _ := procGetForegroundWindow.Call()
    if fg != hwnd {
        return -1, hitRect{}, false
    }

    var wr RECT
    if r, _, _ := procGetWindowRect.Call(
        hwnd,
        uintptr(unsafe.Pointer(&wr)),
    ); r == 0 {
        return -1, hitRect{}, false
    }

    var pt POINT
    if r, _, _ := procGetCursorPos.Call(
        uintptr(unsafe.Pointer(&pt)),
    ); r == 0 {
        return -1, hitRect{}, false
    }

    ww := int(wr.Right - wr.Left)
    wh := int(wr.Bottom - wr.Top)
    if ww <= 0 || wh <= 0 {
        return -1, hitRect{}, false
    }

    if pt.X < wr.Left || pt.X >= wr.Right ||
        pt.Y < wr.Top || pt.Y >= wr.Bottom {
        return -1, hitRect{}, false
    }

    bx := int(pt.X-wr.Left) * baseW / ww
    by := int(pt.Y-wr.Top) * baseH / wh

    for i, r := range hits {
        if bx >= r.x1 && bx <= r.x2 &&
            by >= r.y1 && by <= r.y2 {
            return i, r, true
        }
    }

    return -1, hitRect{}, false
}

func scaleRectToWindow(hwnd uintptr, r hitRect) (int, int, int, int) {
    var wr RECT
    if ok, _, _ := procGetWindowRect.Call(
        hwnd,
        uintptr(unsafe.Pointer(&wr)),
    ); ok == 0 {
        return 0, 0, 0, 0
    }

    ww := int(wr.Right - wr.Left)
    wh := int(wr.Bottom - wr.Top)

    x := int(wr.Left) + r.x1*ww/baseW
    y := int(wr.Top) + r.y1*wh/baseH
    w := (r.x2-r.x1)*ww/baseW
    h := (r.y2-r.y1)*wh/baseH

    return x, y, w, h
}

func createOverlay() (uintptr, uintptr, error) {
    hInst, _, _ := procGetModuleHandleW.Call(0)

    brush, _, _ := procCreateSolidBrush.Call(
        rgb(255, 196, 72),
    )
    if brush == 0 {
        return 0, 0, errors.New("cannot create hover brush")
    }

    cls := utf16Ptr("DawnForgeHoverOverlay")
    wc := WNDCLASSEX{
        CbSize:        uint32(unsafe.Sizeof(WNDCLASSEX{})),
        Style:         CS_HREDRAW | CS_VREDRAW,
        LpfnWndProc:   syscall.NewCallback(wndProc),
        HInstance:     hInst,
        HbrBackground: brush,
        LpszClassName: cls,
    }

    if r, _, e := procRegisterClassExW.Call(
        uintptr(unsafe.Pointer(&wc)),
    ); r == 0 {
        procDeleteObject.Call(brush)
        return 0, 0, fmt.Errorf("register overlay failed: %v", e)
    }

    hwnd, _, e := procCreateWindowExW.Call(
        WS_EX_LAYERED|
            WS_EX_TRANSPARENT|
            WS_EX_TOOLWINDOW|
            WS_EX_NOACTIVATE,
        uintptr(unsafe.Pointer(cls)),
        0,
        WS_POPUP,
        0, 0, 1, 1,
        0, 0, hInst, 0,
    )
    if hwnd == 0 {
        procDeleteObject.Call(brush)
        return 0, 0, fmt.Errorf("create overlay failed: %v", e)
    }

    procSetLayeredWindowAttributes.Call(
        hwnd,
        0,
        42,
        LWA_ALPHA,
    )
    procShowWindow.Call(hwnd, SW_HIDE)

    return hwnd, brush, nil
}

func wndProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
    r, _, _ := procDefWindowProcW.Call(
        hwnd,
        uintptr(msg),
        wParam,
        lParam,
    )
    return r
}

func rgb(r, g, b int) uintptr {
    return uintptr(
        uint32(r) |
            uint32(g)<<8 |
            uint32(b)<<16,
    )
}

func utf16Ptr(s string) *uint16 {
    p, _ := syscall.UTF16PtrFromString(s)
    return p
}

func messageBoxFallback(s string) {
    _ = exec.Command(
        "powershell.exe",
        "-NoProfile",
        "-Command",
        "Add-Type -AssemblyName PresentationFramework; [System.Windows.MessageBox]::Show('"+
            psQuote(s)+
            "','DawnForge Launcher') | Out-Null",
    ).Run()
}

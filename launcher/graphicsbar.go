package main

// One-row graphics/status bar, positioned over the original game's information
// strip. The original INSTALL, UPDATE and PLAY controls are left untouched.
// This is a native Win32 overlay: no embedded browser and no second launcher.

import (
    "fmt"
    "os/exec"
    "runtime"
    "sync"
    "syscall"
    "time"
    "unsafe"

    "dawnforge-launcher/internal/realm"
)

const (
    modeBarClass = "DawnForgeGraphicsStatusV2"
    wmPaint = 0x000F
    wmEraseBkgnd = 0x0014
    wmLButtonUp = 0x0202
    wmDestroy = 0x0002
    pmRemove = 0x0001
    dtSingleLine = 0x0020
    dtVCenter = 0x0004
    dtCenter = 0x0001
    dtLeft = 0x0000
    transparentBk = 1
)

type modeBarPaint struct {
    Hdc uintptr
    Erase int32
    Paint RECT
    Restore int32
    IncUpdate int32
    Reserved [32]byte
}
type winMsg struct {
    Hwnd uintptr
    Message uint32
    WParam uintptr
    LParam uintptr
    Time uint32
    Pt POINT
    Private uint32
}
type graphicsBarState struct {
    mu sync.Mutex
    hwnd uintptr
    original uintptr
    font uintptr
    snapshot realm.Snapshot
    chosen string
    message string
    busy bool
    width int
    height int
}
var graphicsBar *graphicsBarState
var (
    procBeginPaint = user32.NewProc("BeginPaint")
    procEndPaint = user32.NewProc("EndPaint")
    procFillRect = user32.NewProc("FillRect")
    procFrameRect = user32.NewProc("FrameRect")
    procDrawTextW = user32.NewProc("DrawTextW")
    procInvalidateRect = user32.NewProc("InvalidateRect")
    procPeekMessageW = user32.NewProc("PeekMessageW")
    procTranslateMessage = user32.NewProc("TranslateMessage")
    procDispatchMessageW = user32.NewProc("DispatchMessageW")
    procGetClientRect = user32.NewProc("GetClientRect")
    procSetTextColor = gdi32.NewProc("SetTextColor")
    procSetBkMode = gdi32.NewProc("SetBkMode")
    procSelectObject = gdi32.NewProc("SelectObject")
    procCreateFontW = gdi32.NewProc("CreateFontW")
)

func refreshGraphicsStatuses(bar *graphicsBarState){
    status,err:=realm.Fetch(nil)
    if err!=nil {status=realm.Unknown()}
    bar.mu.Lock()
    bar.snapshot=status
    bar.mu.Unlock()
    procInvalidateRect.Call(bar.hwnd,0,0)
}
func startGraphicsStatusPolling(bar *graphicsBarState,done <-chan struct{}) {
    go func(){
        refreshGraphicsStatuses(bar)
        ticker:=time.NewTicker(30*time.Second)
        defer ticker.Stop()
        for {
            select {
            case <-done:return
            case <-ticker.C:refreshGraphicsStatuses(bar)
            }
        }
    }()
}
func newGraphicsBar(orig uintptr)(*graphicsBarState,error){
    hInst,_,_:=procGetModuleHandleW.Call(0)
    cls:=utf16Ptr(modeBarClass)
    wc:=WNDCLASSEX{
        CbSize:uint32(unsafe.Sizeof(WNDCLASSEX{})),
        Style:CS_HREDRAW|CS_VREDRAW,
        LpfnWndProc:syscall.NewCallback(graphicsBarProc),
        HInstance:hInst,
        LpszClassName:cls,
    }
    if result,_,e:=procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)));result==0{
        // A class registered earlier in this process is also acceptable.
        if e!=syscall.Errno(1410){return nil,fmt.Errorf("graphics bar class: %v",e)}
    }
    hwnd,_,e:=procCreateWindowExW.Call(
        WS_EX_TOOLWINDOW,
        uintptr(unsafe.Pointer(cls)),
        0,WS_POPUP,
        0,0,1,1,
        orig,0,hInst,0,
    )
    if hwnd==0{return nil,fmt.Errorf("graphics status window: %v",e)}
    fontName:=utf16Ptr("Segoe UI")
    fontHeight:=int32(-17)
    font,_,_:=procCreateFontW.Call(
        uintptr(uint32(fontHeight)),0,0,0,650,0,0,0,
        1,0,0,0,0,uintptr(unsafe.Pointer(fontName)),
    )
    b:=&graphicsBarState{hwnd:hwnd,original:orig,font:font,snapshot:realm.Unknown(),chosen:"",message:"CHOOSE GRAPHICS • THEN PLAY",width:1020,height:50}
    graphicsBar=b
    return b,nil
}

func readGraphicsBar(bar *graphicsBarState)(snap realm.Snapshot,chosen,msg string,busy bool){
    bar.mu.Lock();defer bar.mu.Unlock()
    return bar.snapshot,bar.chosen,bar.message,bar.busy
}
func updateGraphicsBar(bar *graphicsBarState,chosen,msg string,busy bool) {
    bar.mu.Lock()
    if chosen!=""{bar.chosen=chosen}
    bar.message=msg
    bar.busy=busy
    bar.mu.Unlock()
    procInvalidateRect.Call(bar.hwnd,0,0)
}
func rgbBar(r,g,b byte) uintptr{return uintptr(uint32(r)|uint32(g)<<8|uint32(b)<<16)}
func fillBarRect(hdc uintptr,rc RECT,color uintptr){
    brush,_,_:=procCreateSolidBrush.Call(color)
    procFillRect.Call(hdc,uintptr(unsafe.Pointer(&rc)),brush)
    procDeleteObject.Call(brush)
}
func drawBarText(hdc uintptr,txt string,rc RECT,color uintptr,align uintptr){
    procSetTextColor.Call(hdc,color)
    u,_:=syscall.UTF16PtrFromString(txt)
    procDrawTextW.Call(hdc,uintptr(unsafe.Pointer(u)),^uintptr(0),uintptr(unsafe.Pointer(&rc)),align|dtSingleLine|dtVCenter)
}
func barRect(x,y,w,h int)RECT{return RECT{Left:int32(x),Top:int32(y),Right:int32(x+w),Bottom:int32(y+h)}}
func drawSelection(hdc uintptr,x,y,w,h int,label string,active,disabled bool){
    bg:=rgbBar(28,34,39)
    edge:=rgbBar(124,97,55)
    fg:=rgbBar(227,207,168)
    if active {bg=rgbBar(74,48,13);edge=rgbBar(237,167,58);fg=rgbBar(255,230,168)}
    if disabled {fg=rgbBar(129,131,133)}
    r:=barRect(x,y,w,h)
    fillBarRect(hdc,r,bg)
    brush,_,_:=procCreateSolidBrush.Call(edge)
    procFrameRect.Call(hdc,uintptr(unsafe.Pointer(&r)),brush)
    procDeleteObject.Call(brush)
    drawBarText(hdc,label,r,fg,dtCenter)
}
func modeBarPaintHandler(hwnd uintptr){
    if graphicsBar==nil{return}
    var ps modeBarPaint
    hdc,_,_:=procBeginPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
    if hdc==0{return}
    defer procEndPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
    var r RECT
    procGetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&r)))
    w,h:=int(r.Right),int(r.Bottom)
    bar:=graphicsBar
    bar.mu.Lock();bar.width=w;bar.height=h;bar.mu.Unlock()
    if w<1||h<1{return}
    if bar.font!=0{
        old,_,_:=procSelectObject.Call(hdc,bar.font)
        defer procSelectObject.Call(hdc,old)
    }
    procSetBkMode.Call(hdc,transparentBk)
    fillBarRect(hdc,barRect(0,0,w,h),rgbBar(15,19,20))
    sx:=func(v int)int{return v*w/1020}
    sy:=func(v int)int{return v*h/50}
    snapshot,chosen,msg,busy:=readGraphicsBar(bar)
    // Two compact selectors at LEFT. The original REALM STATUS heading,
    // website button, INSTALL/UPDATE/PLAY and bottom artwork remain visible.
    drawSelection(hdc,sx(7),sy(4),sx(205),sy(42),"ORIGINAL WOW",chosen=="ORIGINAL",busy)
    drawSelection(hdc,sx(221),sy(4),sx(252),sy(42),"MODERN HD · MODELS & TEXTURES",chosen=="MODERN",busy)
    // Centre: world state, count and uptime from the world status API.
    color:=rgbBar(221,196,158)
    if snapshot.State=="ONLINE"{color=rgbBar(123,235,146)}
    if snapshot.State=="OFFLINE"{color=rgbBar(245,132,121)}
    drawBarText(hdc,"REALM",barRect(sx(490),0,sx(68),h),rgbBar(218,189,136),dtLeft)
    drawBarText(hdc,snapshot.State,barRect(sx(557),0,sx(110),h),color,dtLeft)
    drawBarText(hdc,"PLAYERS",barRect(sx(676),0,sx(78),h),rgbBar(218,189,136),dtLeft)
    drawBarText(hdc,snapshot.Players,barRect(sx(759),0,sx(54),h),rgbBar(242,237,221),dtLeft)
    drawBarText(hdc,"UPTIME",barRect(sx(819),0,sx(76),h),rgbBar(218,189,136),dtLeft)
    drawBarText(hdc,snapshot.Uptime,barRect(sx(896),0,sx(118),h),rgbBar(242,237,221),dtLeft)
    if busy || len(msg)>=6&&msg[:6]=="ERROR:" {
        // Paint only over selectors; never obscure the realm/status values.
        fillBarRect(hdc,barRect(sx(4),sy(1),sx(470),h-sy(2)),rgbBar(15,19,20))
        drawBarText(hdc,msg,barRect(sx(10),0,sx(458),h),rgbBar(240,200,109),dtLeft)
    }
}
func graphicsBarProc(hwnd uintptr,msg uint32,wParam,lParam uintptr)uintptr{
    switch msg{
    case wmEraseBkgnd:return 1
    case wmPaint:modeBarPaintHandler(hwnd);return 0
    case wmLButtonUp:
        if graphicsBar==nil{return 0}
        bar:=graphicsBar
        x:=int(int16(uint16(lParam&0xffff)))
        bar.mu.Lock();bw,busy:=bar.width,bar.busy;bar.mu.Unlock()
        if busy||bw<=0{return 0}
        v:=x*1020/bw
        if v>=7&&v<212 {go activateGraphicsMode(bar,"ORIGINAL");return 0}
        if v>=221&&v<473 {go activateGraphicsMode(bar,"MODERN");return 0}
        return 0
    }
    result,_,_:=procDefWindowProcW.Call(hwnd,uintptr(msg),wParam,lParam)
    return result
}
func activateGraphicsMode(bar *graphicsBarState,mode string) {
    runtime.LockOSThread()
    defer runtime.UnlockOSThread()
    bar.mu.Lock()
    if bar.busy{bar.mu.Unlock();return}
    bar.busy=true
    bar.message="CHECKING "+mode+"..."
    bar.mu.Unlock()
    procInvalidateRect.Call(bar.hwnd,0,0)

    settings,err:=loadHDSettings()
    if err!=nil||settings.GameDir=="" {
        updateGraphicsBar(bar,"","ERROR: SELECT YOUR WOW FOLDER",false)
        if e:=launchHDManager();e!=nil{messageBoxFallback("DawnForge: "+e.Error())}
        return
    }
    task:="--hd-install"
    if mode=="ORIGINAL"{task="--vanilla-mode"}
    if err:=hdTask(task,settings.GameDir);err!=nil{
        updateGraphicsBar(bar,"","ERROR: "+err.Error(),false)
        messageBoxFallback("DawnForge graphics: "+err.Error())
        return
    }
    updateGraphicsBar(bar,mode,mode+" READY · CLICK PLAY",false)
}
func updateGraphicsBarPosition(bar *graphicsBarState,orig uintptr) {
    vis,_,_:=procIsWindowVisible.Call(orig)
    icon,_,_:=procIsIconic.Call(orig)
    fg,_,_:=procGetForegroundWindow.Call()
    if vis==0||icon!=0||(fg!=orig&&fg!=bar.hwnd){
        procShowWindow.Call(bar.hwnd,SW_HIDE)
        return
    }
    var wr RECT
    if ok,_,_:=procGetWindowRect.Call(orig,uintptr(unsafe.Pointer(&wr)));ok==0{return}
    ww,wh:=int(wr.Right-wr.Left),int(wr.Bottom-wr.Top)
    if ww<=0||wh<=0{return}
    x:=int(wr.Left)+24*ww/baseW
    y:=int(wr.Top)+605*wh/baseH
    w:=1020*ww/baseW
    h:=48*wh/baseH
    if h<30{h=30}
    procSetWindowPos.Call(bar.hwnd,hwndTop,uintptr(x),uintptr(y),uintptr(w),uintptr(h),SWP_NOACTIVATE|SWP_SHOWWINDOW)
}
func pollWindowsMessages(){
    // The original hover-only wrapper had no message loop.
    // Owner-drawn native graphics controls require one.
    var m winMsg
    for {
        ok,_,_:=procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)),0,0,0,pmRemove)
        if ok==0{return}
        procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
        procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
    }
}
func destroyGraphicsBar(bar *graphicsBarState){
    if bar==nil{return}
    procDestroyWindow.Call(bar.hwnd)
    if bar.font!=0{procDeleteObject.Call(bar.font)}
}

package main

// One-row graphics/status bar, positioned over the original game's information
// strip. The original INSTALL, UPDATE and PLAY controls are left untouched.
// This is a native Win32 overlay: no embedded browser and no second launcher.

import (
    "fmt"
    "net"
    "net/http"
    "os/exec"
    "runtime"
    "strings"
    "sync"
    "syscall"
    "time"
    "unsafe"
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
    web string
    login string
    cdn string
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

func probeHttp(url string) string {
    client:=&http.Client{Timeout:4*time.Second}
    req,err:=http.NewRequest("HEAD",url,nil)
    if err!=nil{return "?"}
    resp,err:=client.Do(req)
    if err!=nil{return "OFF"}
    resp.Body.Close()
    if resp.StatusCode>=200&&resp.StatusCode<400{return "ON"}
    return "OFF"
}
func probeLogin() string {
    c,err:=net.DialTimeout("tcp","dawnforge.eu:3724",3*time.Second)
    if err!=nil{return "OFF"}
    c.Close()
    return "ON"
}
func refreshGraphicsStatuses(bar *graphicsBarState){
    // Independent timeouts keep the UI responsive during a network outage.
    var wg sync.WaitGroup
    wg.Add(3)
    go func(){defer wg.Done();s:=probeHttp("https://dawnforge.eu/");bar.mu.Lock();bar.web=s;bar.mu.Unlock()}()
    go func(){defer wg.Done();s:=probeHttp("https://hd.dawnforge.eu/hd/patch-N.mpq");bar.mu.Lock();bar.cdn=s;bar.mu.Unlock()}()
    go func(){defer wg.Done();s:=probeLogin();bar.mu.Lock();bar.login=s;bar.mu.Unlock()}()
    wg.Wait()
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
    font,_,_:=procCreateFontW.Call(
        uintptr(int32(-17)),0,0,0,650,0,0,0,
        1,0,0,0,0,uintptr(unsafe.Pointer(fontName)),
    )
    b:=&graphicsBarState{hwnd:hwnd,original:orig,font:font,web:"?",login:"?",cdn:"?",chosen:"ORIGINAL",message:"CHOOSE GRAPHICS • THEN PLAY",width:1407,height:72}
    graphicsBar=b
    return b,nil
}

func readGraphicsBar(bar *graphicsBarState)(web,login,cdn,chosen,msg string,busy bool){
    bar.mu.Lock();defer bar.mu.Unlock()
    return bar.web,bar.login,bar.cdn,bar.chosen,bar.message,bar.busy
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
    oldFont:=uintptr(0)
    if bar.font!=0{oldFont,_,_=procSelectObject.Call(hdc,bar.font);defer procSelectObject.Call(hdc,oldFont)}
    procSetBkMode.Call(hdc,transparentBk)
    fillBarRect(hdc,barRect(0,0,w,h),rgbBar(15,19,20))
    fillBarRect(hdc,barRect(0,0,w,2),rgbBar(135,99,43))
    fillBarRect(hdc,barRect(0,h-2,w,2),rgbBar(135,99,43))

    // All information and controls share ONE physical row.
    // Internal coordinates are scaled from the source launcher width.
    sx:=func(v int)int{return v*w/1407}
    sy:=func(v int)int{return v*h/72}
    web,login,cdn,chosen,msg,busy:=readGraphicsBar(bar)
    onColor:=rgbBar(126,228,148)
    offColor:=rgbBar(240,123,110)
    statusColor:=func(s string)uintptr{if s=="ON"{return onColor};if s=="OFF"{return offColor};return rgbBar(206,190,153)}
    drawBarText(hdc,"LOGIN",barRect(sx(15),0,sx(63),h),rgbBar(190,184,171),dtLeft)
    drawBarText(hdc,login,barRect(sx(78),0,sx(48),h),statusColor(login),dtLeft)
    drawBarText(hdc,"WEB",barRect(sx(137),0,sx(46),h),rgbBar(190,184,171),dtLeft)
    drawBarText(hdc,web,barRect(sx(184),0,sx(48),h),statusColor(web),dtLeft)
    drawBarText(hdc,"HD",barRect(sx(245),0,sx(28),h),rgbBar(190,184,171),dtLeft)
    drawBarText(hdc,cdn,barRect(sx(273),0,sx(50),h),statusColor(cdn),dtLeft)
    // Busy messages replace only the left statuses.
    if busy {
        fillBarRect(hdc,barRect(sx(6),sy(4),sx(324),h-sy(8)),rgbBar(15,19,20))
        drawBarText(hdc,msg,barRect(sx(12),0,sx(316),h),rgbBar(240,200,109),dtLeft)
    } else if strings.HasPrefix(msg,"ERROR") {
        fillBarRect(hdc,barRect(sx(6),sy(4),sx(324),h-sy(8)),rgbBar(15,19,20))
        drawBarText(hdc,msg,barRect(sx(12),0,sx(316),h),rgbBar(240,123,110),dtLeft)
    }
    drawBarText(hdc,"GRAPHICS",barRect(sx(343),0,sx(116),h),rgbBar(234,202,143),dtLeft)
    drawSelection(hdc,sx(466),sy(10),sx(214),sy(51),"ORIGINAL WOW",chosen=="ORIGINAL",busy)
    drawSelection(hdc,sx(695),sy(10),sx(505),sy(51),"MODERN GRAPHICS · MODELS / BUILDINGS / TEXTURES",chosen=="MODERN",busy)
    drawSelection(hdc,sx(1215),sy(10),sx(179),sy(51),"WEBSITE",false,false)
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
        v:=x*1407/bw
        if v>=466&&v<680 {go activateGraphicsMode(bar,"ORIGINAL");return 0}
        if v>=695&&v<1200 {go activateGraphicsMode(bar,"MODERN");return 0}
        if v>=1215&&v<1394 {
            // Open the public web site without embedding player credentials.
            _=exec.Command("rundll32.exe","url.dll,FileProtocolHandler","https://dawnforge.eu/").Start()
            return 0
        }
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
    y:=int(wr.Top)+632*wh/baseH
    w:=1407*ww/baseW
    h:=70*wh/baseH
    if h<36{h=36}
    procSetWindowPos.Call(bar.hwnd,hwndTop,uintptr(x),uintptr(y),uintptr(w),uintptr(h),SWP_NOACTIVATE|SWP_SHOWWINDOW)
}
func pollWindowsMessages(){
    // The original hover-only wrapper had no message loop.
    // Owner-drawn native graphics controls require one.
    var m winMsg
    for {
        ok,_,_:=procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)),0,0,0,pmRemove)
        if ok==0{return}
        procTranslateMessage.Call(uintptr(unsafe.Pointer(&m))
        procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m))
    }
}
func destroyGraphicsBar(bar *graphicsBarState){
    if bar==nil{return}
    procDestroyWindow.Call(bar.hwnd)
    if bar.font!=0{procDeleteObject.Call(bar.font)}
}

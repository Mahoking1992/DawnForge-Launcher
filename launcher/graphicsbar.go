package main

// One-row graphics/status bar, positioned over the original game's information
// strip. The original INSTALL, UPDATE and PLAY controls are left untouched.
// This is a native Win32 overlay: no embedded browser and no second launcher.

import (
    "fmt"
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
    statusFont uintptr
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
    statusHeight:=int32(-23)
    bigFont,_,_:=procCreateFontW.Call(
        uintptr(uint32(statusHeight)),0,0,0,700,0,0,0,
        1,0,0,0,0,uintptr(unsafe.Pointer(fontName)),
    )
    b:=&graphicsBarState{hwnd:hwnd,original:orig,font:font,statusFont:bigFont,snapshot:realm.Unknown(),chosen:"",message:"CHOOSE GRAPHICS • THEN PLAY",width:1135,height:85}
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
    var client RECT
    procGetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&client)))
    w,h:=int(client.Right),int(client.Bottom)
    if w<=0||h<=0{return}

    bar:=graphicsBar
    bar.mu.Lock()
    bar.width=w
    bar.height=h
    bar.mu.Unlock()

    oldFont:=uintptr(0)
    if bar.font!=0 {
        oldFont,_,_=procSelectObject.Call(hdc,bar.font)
        defer procSelectObject.Call(hdc,oldFont)
    }
    procSetBkMode.Call(hdc,transparentBk)
    sx:=func(v int)int{return v*w/1135}
    sy:=func(v int)int{return v*h/85}

    // Replaces only the original GAME VERSION + REALM STATUS information row.
    // The WEBSITE button to the right and INSTALL/UPDATE/PLAY below remain intact.
    gold:=rgbBar(145,103,45)
    labelColor:=rgbBar(232,201,148)
    valueColor:=rgbBar(248,236,211)
    fillBarRect(hdc,barRect(0,0,w,h),rgbBar(13,17,19))
    fillBarRect(hdc,barRect(0,0,w,sy(2)),gold)
    fillBarRect(hdc,barRect(0,h-sy(2),w,sy(2)),gold)
    fillBarRect(hdc,barRect(0,0,sx(2),h),gold)
    fillBarRect(hdc,barRect(w-sx(2),0,sx(2),h),gold)

    status,chosen,msg,busy:=readGraphicsBar(bar)

    // Left side: match the original launcher panel shown in the reference image.
    drawBarText(hdc,"GRAPHICS",barRect(sx(12),0,sx(99),h),labelColor,dtLeft)
    drawSelection(hdc,sx(110),sy(12),sx(158),sy(59),
        "OLD VANILLA",chosen=="ORIGINAL",busy)
    drawSelection(hdc,sx(275),sy(12),sx(165),sy(59),
        "MODERN VANILLA",chosen=="MODERN",busy)

    // Columns have fixed positions in the original 1458×972 design coordinates.
    for _,divider:=range []int{445,662,884}{
        fillBarRect(hdc,barRect(sx(divider),sy(8),sx(2),sy(69)),gold)
    }

    titleRect:=func(x,width int)RECT{
        return barRect(sx(x),sy(8),sx(width),sy(29))
    }
    valueRect:=func(x,width int)RECT{
        return barRect(sx(x),sy(36),sx(width),sy(40))
    }

    drawBarText(hdc,"REALM STATUS",titleRect(452,204),labelColor,dtCenter)
    drawBarText(hdc,"PLAYERS ONLINE",titleRect(670,208),labelColor,dtCenter)
    drawBarText(hdc,"UPTIME",titleRect(892,232),labelColor,dtCenter)

    realmColor:=valueColor
    if status.State=="ONLINE" {realmColor=rgbBar(87,231,123)}
    if status.State=="OFFLINE" {realmColor=rgbBar(249,103,103)}
    if status.State=="MAINTENANCE" {realmColor=rgbBar(242,176,79)}

    if bar.statusFont!=0 {procSelectObject.Call(hdc,bar.statusFont)}
    drawBarText(hdc,status.State,valueRect(452,204),realmColor,dtCenter)
    drawBarText(hdc,status.Players,valueRect(670,208),rgbBar(87,231,123),dtCenter)
    drawBarText(hdc,status.Uptime,valueRect(892,232),valueColor,dtCenter)

    if bar.font!=0 {procSelectObject.Call(hdc,bar.font)}
    if busy || len(msg)>=6&&msg[:6]=="ERROR:" {
        fillBarRect(hdc,barRect(sx(3),sy(3),sx(440),h-sy(6)),rgbBar(13,17,19))
        color:=rgbBar(240,200,109)
        if len(msg)>=6 && msg[:6]=="ERROR:" {color=rgbBar(249,123,111)}
        drawBarText(hdc,msg,barRect(sx(12),sy(3),sx(422),h-sy(6)),color,dtCenter)
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
        v:=x*1135/bw
        if v>=110&&v<268 {go activateGraphicsMode(bar,"ORIGINAL");return 0}
        if v>=275&&v<440 {go activateGraphicsMode(bar,"MODERN");return 0}
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
    x:=int(wr.Left)+28*ww/baseW
    y:=int(wr.Top)+618*wh/baseH
    w:=1135*ww/baseW
    h:=85*wh/baseH
    if h<55{h=55}
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
    if bar.statusFont!=0{procDeleteObject.Call(bar.statusFont)}
}

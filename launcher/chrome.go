package main

// Chrome overlay: click-through native Win32 vector artwork drawn over the
// EXISTING launcher controls. Original click handlers and installer remain
// untouched. No image generation, external icon pack, or 3rd-party runtime.
import (
    "fmt"
    "syscall"
    "unsafe"
)

const (
    chromeClass = "DawnForgeChromeIconsV1"
    lwaColorKey = 0x00000001
    psNoActivate = 0x0010
)
var (
    procChromeEllipse = gdi32.NewProc("Ellipse")
    procChromeRoundRect = gdi32.NewProc("RoundRect")
    procChromeMoveToEx = gdi32.NewProc("MoveToEx")
    procChromeLineTo = gdi32.NewProc("LineTo")
    procChromePolygon = gdi32.NewProc("Polygon")
    procChromeCreatePen = gdi32.NewProc("CreatePen")
)

type launcherChrome struct {
    hwnd uintptr
    original uintptr
    width int
    height int
}
var chrome *launcherChrome

type chromeIcon struct {
    x, y int
    kind string
    tint uintptr
}
func chromeIcons()[]chromeIcon{
    return []chromeIcon{
        {x:90,y:770,kind:"download",tint:rgbBar(255,185,81)},
        {x:959,y:770,kind:"refresh",tint:rgbBar(128,205,246)},
        {x:1226,y:770,kind:"play",tint:rgbBar(255,190,84)},
        {x:1242,y:665,kind:"globe",tint:rgbBar(119,209,237)},
        {x:1329,y:32,kind:"gear",tint:rgbBar(246,192,96)},
    }
}
func createLauncherChrome(orig uintptr)(*launcherChrome,error){
    hInst,_,_:=procGetModuleHandleW.Call(0)
    cls:=utf16Ptr(chromeClass)
    wc:=WNDCLASSEX{
        CbSize:uint32(unsafe.Sizeof(WNDCLASSEX{})),
        Style:CS_HREDRAW|CS_VREDRAW,
        LpfnWndProc:syscall.NewCallback(chromeWndProc),
        HInstance:hInst,
        LpszClassName:cls,
    }
    if result,_,e:=procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)));result==0&&e!=syscall.Errno(1410){
        return nil,fmt.Errorf("register chrome overlay: %v",e)
    }
    hwnd,_,e:=procCreateWindowExW.Call(
        WS_EX_LAYERED|WS_EX_TRANSPARENT|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
        uintptr(unsafe.Pointer(cls)),0,WS_POPUP,
        0,0,1,1,orig,0,hInst,0,
    )
    if hwnd==0{return nil,fmt.Errorf("create launcher chrome: %v",e)}
    key:=rgbBar(3,4,5)
    if r,_,e:=procSetLayeredWindowAttributes.Call(hwnd,key,0,lwaColorKey);r==0{
        procDestroyWindow.Call(hwnd)
        return nil,fmt.Errorf("configure transparent chrome: %v",e)
    }
    c:=&launcherChrome{hwnd:hwnd,original:orig,width:baseW,height:baseH}
    chrome=c
    return c,nil
}
func chromeWndProc(hwnd uintptr,msg uint32,wParam,lParam uintptr)uintptr{
    switch msg {
    case wmEraseBkgnd:return 1
    case wmPaint:paintLauncherChrome(hwnd);return 0
    }
    result,_,_:=procDefWindowProcW.Call(hwnd,uintptr(msg),wParam,lParam)
    return result
}
func chromePen(hdc uintptr,color uintptr,thickness int)func(){
    pen,_,_:=procChromeCreatePen.Call(0,uintptr(thickness),color)
    old,_,_:=procSelectObject.Call(hdc,pen)
    return func(){procSelectObject.Call(hdc,old);procDeleteObject.Call(pen)}
}
func chromeLine(hdc uintptr,x1,y1,x2,y2 int){
    procChromeMoveToEx.Call(hdc,uintptr(x1),uintptr(y1),0)
    procChromeLineTo.Call(hdc,uintptr(x2),uintptr(y2))
}
func chromeDot(hdc uintptr,x,y,r int){
    procChromeEllipse.Call(hdc,uintptr(x-r),uintptr(y-r),uintptr(x+r),uintptr(y+r))
}
func drawChromeIcon(hdc uintptr,cx,cy,scale int,kind string,tint uintptr){
    if scale<1{return}
    bg:=rgbBar(18,28,36)
    // A small rounded icon well hides the old pictogram, not the old button
    // title, progress indicator or the original button's click target.
    outer:=scale*18
    off:=chromePen(hdc,tint,2*scale)
    brush,_,_:=procCreateSolidBrush.Call(bg)
    oldBrush,_,_:=procSelectObject.Call(hdc,brush)
    procChromeRoundRect.Call(hdc,
        uintptr(cx-outer),uintptr(cy-outer),
        uintptr(cx+outer),uintptr(cy+outer),
        uintptr(scale*11),uintptr(scale*11))
    procSelectObject.Call(hdc,oldBrush)
    procDeleteObject.Call(brush)
    off()

    undo:=chromePen(hdc,tint,2*scale)
    defer undo()
    switch kind {
    case "download":
        chromeLine(hdc,cx,cy-10*scale,cx,cy+6*scale)
        chromeLine(hdc,cx-7*scale,cy,cx,cy+7*scale)
        chromeLine(hdc,cx+7*scale,cy,cx,cy+7*scale)
        chromeLine(hdc,cx-10*scale,cy+11*scale,cx+10*scale,cy+11*scale)
    case "refresh":
        chromeDot(hdc,cx,cy,10*scale)
        chromeLine(hdc,cx+3*scale,cy-11*scale,cx+12*scale,cy-7*scale)
        chromeLine(hdc,cx+12*scale,cy-7*scale,cx+10*scale,cy-16*scale)
        chromeLine(hdc,cx-3*scale,cy+11*scale,cx-12*scale,cy+7*scale)
    case "play":
        pts:=[3]POINT{
            {X:int32(cx-6*scale),Y:int32(cy-11*scale)},
            {X:int32(cx+12*scale),Y:int32(cy)},
            {X:int32(cx-6*scale),Y:int32(cy+11*scale)},
        }
        penBrush,_,_:=procCreateSolidBrush.Call(tint)
        old,_,_:=procSelectObject.Call(hdc,penBrush)
        procChromePolygon.Call(hdc,uintptr(unsafe.Pointer(&pts[0])),3)
        procSelectObject.Call(hdc,old)
        procDeleteObject.Call(penBrush)
    case "globe":
        chromeDot(hdc,cx,cy,11*scale)
        chromeLine(hdc,cx-11*scale,cy,cx+11*scale,cy)
        chromeLine(hdc,cx,cy-11*scale,cx,cy+11*scale)
        chromeLine(hdc,cx-7*scale,cy-7*scale,cx-7*scale,cy+7*scale)
        chromeLine(hdc,cx+7*scale,cy-7*scale,cx+7*scale,cy+7*scale)
    case "gear":
        chromeDot(hdc,cx,cy,8*scale)
        chromeDot(hdc,cx,cy,3*scale)
        for _,d:=range [][4]int{
            {-12,0,-8,0},{8,0,12,0},{0,-12,0,-8},{0,8,0,12},
            {-9,-9,-6,-6},{6,6,9,9},{6,-6,9,-9},{-9,9,-6,6},
        }{
            chromeLine(hdc,cx+d[0]*scale,cy+d[1]*scale,cx+d[2]*scale,cy+d[3]*scale)
        }
    }
}
func paintLauncherChrome(hwnd uintptr){
    if chrome==nil{return}
    var ps modeBarPaint
    hdc,_,_:=procBeginPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
    if hdc==0{return}
    defer procEndPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))

    var client RECT
    procGetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&client)))
    w,h:=int(client.Right),int(client.Bottom)
    if w<1||h<1{return}
    key:=rgbBar(3,4,5)
    // This colour-keyed area lets the entire main artwork, real-time status,
    // and every original input remain visible/clickable.
    fillBarRect(hdc,barRect(0,0,w,h),key)
    for _,spec:=range chromeIcons(){
        // Only paint icon regions; do not add duplicate ONLINE labels.
        x:=spec.x*w/baseW
        y:=spec.y*h/baseH
        scale:=1
        drawChromeIcon(hdc,x,y,scale,spec.kind,spec.tint)
    }
}
func updateLauncherChromePosition(c *launcherChrome,orig uintptr){
    if c==nil{return}
    vis,_,_:=procIsWindowVisible.Call(orig)
    icon,_,_:=procIsIconic.Call(orig)
    fg,_,_:=procGetForegroundWindow.Call()
    if vis==0||icon!=0||(fg!=orig&&fg!=c.hwnd){
        procShowWindow.Call(c.hwnd,SW_HIDE)
        return
    }
    var rc RECT
    if ok,_,_:=procGetWindowRect.Call(orig,uintptr(unsafe.Pointer(&rc)));ok==0{return}
    w,h:=int(rc.Right-rc.Left),int(rc.Bottom-rc.Top)
    if w<=0||h<=0{return}
    procSetWindowPos.Call(c.hwnd,hwndTop,uintptr(rc.Left),uintptr(rc.Top),
        uintptr(w),uintptr(h),SWP_NOACTIVATE|SWP_SHOWWINDOW)
}
func destroyLauncherChrome(c *launcherChrome){
    if c!=nil{procDestroyWindow.Call(c.hwnd)}
}

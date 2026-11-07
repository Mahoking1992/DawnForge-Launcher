package main

// DawnForge 1.0.12 experimental dashboard.
// Replaces the old lower control area as ONE coherent native Win32 UI.
// The original launcher remains the game-install backend; non-destructive
// actions are delegated to it rather than reimplementing the download protocol.
import (
 "fmt"
 "os/exec"
 "strings"
 "sync"
 "syscall"
 "time"
 "unsafe"

 "dawnforge-launcher/internal/realm"
)

const (
 dashboardClass = "DawnForgeModernDashboardV1"
 dashboardColorKey = 0x00000001
 dashboardWMDown = 0x0201
 dashboardWMMove = 0x0200
 dashboardWMUp = 0x0202
)

type dashboardState struct {
 mu sync.RWMutex
 hwnd uintptr
 source uintptr
 server realm.Snapshot
 selected string
 activeTask string
 warning string
 busy bool
 font uintptr
 titleFont uintptr
 largeFont uintptr
 w int
 h int
}
var dashboard *dashboardState

var (
 dashRoundRect = gdi32.NewProc("RoundRect")
 dashCreatePen = gdi32.NewProc("CreatePen")
 dashSendMessageW = user32.NewProc("SendMessageW")
 dashInvalidate = user32.NewProc("InvalidateRect")
 dashSetBkMode = gdi32.NewProc("SetBkMode")
 dashSetTextColor = gdi32.NewProc("SetTextColor")
 dashSelectObject = gdi32.NewProc("SelectObject")
 dashCreateFont = gdi32.NewProc("CreateFontW")
)

func dashFont(size int32,weight uintptr)uintptr {
 u:=utf16Ptr("Segoe UI")
 signed:=uint32(-size)
 handle,_,_:=dashCreateFont.Call(uintptr(signed),0,0,0,weight,0,0,0,1,0,0,0,0,uintptr(unsafe.Pointer(u)))
 return handle
}

func startDashboard(original uintptr)(*dashboardState,error){
 hInst,_,_:=procGetModuleHandleW.Call(0)
 cls:=utf16Ptr(dashboardClass)
 wc:=WNDCLASSEX{
  CbSize:uint32(unsafe.Sizeof(WNDCLASSEX{})),
  Style:CS_HREDRAW|CS_VREDRAW,
  LpfnWndProc:syscall.NewCallback(dashboardWndProc),
  HInstance:hInst,
  LpszClassName:cls,
 }
 if res,_,e:=procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)));res==0&&e!=syscall.Errno(1410){
  return nil,fmt.Errorf("register new dashboard: %v",e)
 }
 hwnd,_,e:=procCreateWindowExW.Call(
  WS_EX_LAYERED|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
  uintptr(unsafe.Pointer(cls)),0,WS_POPUP,0,0,1,1,
  original,0,hInst,0,
 )
 if hwnd==0{return nil,fmt.Errorf("create dashboard: %v",e)}
 key:=rgbBar(2,3,4)
 if r,_,e:=procSetLayeredWindowAttributes.Call(hwnd,key,0,dashboardColorKey);r==0{
  procDestroyWindow.Call(hwnd)
  return nil,fmt.Errorf("transparent dashboard: %v",e)
 }
 d:=&dashboardState{
  hwnd:hwnd,source:original,server:realm.Unknown(),
  font:dashFont(17,600),titleFont:dashFont(16,650),
  largeFont:dashFont(27,750),w:baseW,h:baseH,
 }
 dashboard=d
 return d,nil
}
func closeDashboard(d *dashboardState){
 if d==nil{return}
 if d.hwnd!=0{procDestroyWindow.Call(d.hwnd)}
 if d.font!=0{procDeleteObject.Call(d.font)}
 if d.titleFont!=0{procDeleteObject.Call(d.titleFont)}
 if d.largeFont!=0{procDeleteObject.Call(d.largeFont)}
 if dashboard==d{dashboard=nil}
}

func drawDashboardBorder(hdc uintptr,x,y,w,h,radius int,fill,border uintptr){
 brush,_,_:=procCreateSolidBrush.Call(fill)
 oldBrush,_,_:=dashSelectObject.Call(hdc,brush)
 pen,_,_:=dashCreatePen.Call(0,2,border)
 oldPen,_,_:=dashSelectObject.Call(hdc,pen)
 dashRoundRect.Call(hdc,uintptr(x),uintptr(y),uintptr(x+w),uintptr(y+h),uintptr(radius),uintptr(radius))
 dashSelectObject.Call(hdc,oldPen)
 dashSelectObject.Call(hdc,oldBrush)
 procDeleteObject.Call(pen)
 procDeleteObject.Call(brush)
}
func dashText(hdc uintptr,txt string,r RECT,color, font uintptr,center bool){
 if font!=0{
  old,_,_:=dashSelectObject.Call(hdc,font)
  defer dashSelectObject.Call(hdc,old)
 }
 dashSetBkMode.Call(hdc,1)
 dashSetTextColor.Call(hdc,color)
 u,_:=syscall.UTF16PtrFromString(txt)
 flags:=uintptr(dtSingleLine|dtVCenter|dtCenter)
 if !center{flags=dtSingleLine|dtVCenter|dtLeft}
 procDrawTextW.Call(hdc,uintptr(unsafe.Pointer(u)),^uintptr(0),uintptr(unsafe.Pointer(&r)),flags)
}
func dashButton(hdc uintptr,d *dashboardState,x,y,w,h int,label,icon string,accent,active,disabled bool){
 bg:=rgbBar(22,29,39)
 edge:=rgbBar(77,89,101)
 color:=rgbBar(230,236,244)
 if accent {bg=rgbBar(76,35,12);edge=rgbBar(252,163,48);color=rgbBar(255,231,168)}
 if active {bg=rgbBar(15,59,98);edge=rgbBar(55,198,255);color=rgbBar(248,250,255)}
 if disabled{bg=rgbBar(27,30,35);edge=rgbBar(48,53,61);color=rgbBar(119,126,137)}
 drawDashboardBorder(hdc,x,y,w,h,12,bg,edge)
 // Pictograms use native Segoe UI glyphs; no stock icons or external DLLs.
 if icon!="" {
  dashText(hdc,icon,barRect(x+8,y+2,40,h-4),edge,d.largeFont,true)
  dashText(hdc,label,barRect(x+45,y+1,w-50,h-2),color,d.font,true)
 }else{
  dashText(hdc,label,barRect(x+6,y+1,w-12,h-2),color,d.font,true)
 }
}
func drawDashboardLine(hdc uintptr,x,y,w int){
 fillBarRect(hdc,barRect(x,y,w,1),rgbBar(93,76,55))
}
func drawDashboardScreen(hwnd uintptr){
 if dashboard==nil{return}
 var ps modeBarPaint
 hdc,_,_:=procBeginPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
 if hdc==0{return}
 defer procEndPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
 var client RECT
 procGetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&client)))
 w,h:=int(client.Right),int(client.Bottom)
 if w<1||h<1{return}
 d:=dashboard
 d.mu.Lock()
 d.w,d.h=w,h
 snap,selected,busy,note:=d.server,d.selected,d.busy,d.warning
 d.mu.Unlock()
 toX:=func(x int)int{return x*w/baseW}
 toY:=func(y int)int{return y*h/baseH}
 scaleRect:=func(x,y,ww,hh int)RECT{
  return barRect(toX(x),toY(y),toX(ww),toY(hh))
 }
 // Colour-key transparent outside the new unified interface.
 fillBarRect(hdc,barRect(0,0,w,h),rgbBar(2,3,4))

 panel:=rgbBar(10,16,23)
 edge:=rgbBar(128,91,44)
 drawDashboardBorder(hdc,toX(34),toY(503),toX(1390),toY(361),22,panel,edge)
 drawDashboardLine(hdc,toX(44),toY(571),toX(1370))

 // Row 1: actual navigation controls in the order shown on the supplied mockup.
 dashButton(hdc,d,toX(52),toY(511),toX(220),toY(54),"Play","▶",true,false,false)
 dashButton(hdc,d,toX(282),toY(511),toX(257),toY(54),"Install / Repair","⇩",false,false,false)
 dashButton(hdc,d,toX(549),toY(511),toX(171),toY(54),"Update","↻",false,false,false)
 dashButton(hdc,d,toX(730),toY(511),toX(191),toY(54),"Graphics","▣",false,false,false)
 dashButton(hdc,d,toX(931),toY(511),toX(220),toY(54),"Settings","⚙",false,false,false)
 dashButton(hdc,d,toX(1161),toY(511),toX(238),toY(54),"About","ⓘ",false,false,false)

 // Row 2: two real modes + a visibly unavailable future third mode.
 dashButton(hdc,d,toX(90),toY(585),toX(376),toY(59),"Classic (Old Vanilla)","✦",false,selected=="ORIGINAL",busy)
 dashButton(hdc,d,toX(480),toY(585),toX(407),toY(59),"Modern Vanilla","✦",false,selected=="MODERN",busy)
 dashButton(hdc,d,toX(901),toY(585),toX(448),toY(59),"Ultra HD (Not available)","HD",false,false,true)

 // Row 3: ONE status area; the core's old ONLINE region is fully hidden.
 drawDashboardBorder(hdc,toX(48),toY(658),toX(1354),toY(77),12,rgbBar(13,25,34),rgbBar(50,91,119))
 for _,v:=range []int{315,603,864,1136}{
  fillBarRect(hdc,barRect(toX(v),toY(671),2,toY(52)),rgbBar(63,88,105))
 }
 drawStatus:=func(x,y,width int,heading,value string,valueColor uintptr){
  dashText(hdc,heading,scaleRect(x,y,width,27),rgbBar(158,199,222),d.titleFont,true)
  dashText(hdc,value,scaleRect(x,y+25,width,41),valueColor,d.largeFont,true)
 }
 onlineColor:=rgbBar(231,196,129)
 if snap.State=="ONLINE"{onlineColor=rgbBar(77,236,124)}
 if snap.State=="OFFLINE"{onlineColor=rgbBar(253,112,108)}
 drawStatus(55,663,255,"SERVER STATUS",snap.State,onlineColor)
 drawStatus(318,663,281,"PLAYERS ONLINE",snap.Players,rgbBar(95,236,218))
 drawStatus(608,663,251,"UPTIME",snap.Uptime,rgbBar(242,232,216))
 drawStatus(870,663,259,"GAME VERSION","1.12.1",rgbBar(236,232,228))
 dashButton(hdc,d,toX(1152),toY(674),toX(229),toY(47),"Website","◎",false,false,false)

 // Row 4: preserve install/update/play behavior, but replace their old
 // drawn controls with coherent high-contrast buttons.
 drawDashboardLine(hdc,toX(47),toY(749),toX(1362))
 dashButton(hdc,d,toX(56),toY(760),toX(321),toY(83),"INSTALL","⇩",false,true,false)
 dashText(hdc,"GAME CLIENT",scaleRect(390,764,465,30),rgbBar(221,192,143),d.titleFont,true)
 statusText:="Use Install / Repair to check game files"
 if busy {statusText="Working on the selected graphics mode..."}
 if note!="" {statusText=note}
 dashText(hdc,statusText,scaleRect(394,796,463,45),rgbBar(216,226,233),d.font,true)
 dashButton(hdc,d,toX(881),toY(760),toX(233),toY(83),"UPDATE","↻",false,false,false)
 dashButton(hdc,d,toX(1126),toY(760),toX(274),toY(83),"PLAY","▶",true,false,false)
}
func dashboardWndProc(hwnd uintptr,msg uint32,wParam,lParam uintptr)uintptr{
 switch msg{
 case wmEraseBkgnd:return 1
 case wmPaint:drawDashboardScreen(hwnd);return 0
 case wmLButtonUp:
  if dashboard==nil{return 0}
  d:=dashboard
  x,y:=int(int16(uint16(lParam&0xffff))),int(int16(uint16((lParam>>16)&0xffff)))
  d.mu.RLock()
  w,h:=d.w,d.h
  d.mu.RUnlock()
  if w<1||h<1{return 0}
  px,py:=x*baseW/w,y*baseH/h
  go dashboardClicked(d,px,py)
  return 0
 }
 r,_,_:=procDefWindowProcW.Call(hwnd,uintptr(msg),wParam,lParam)
 return r
}
func dashboardRectHit(x,y,left,top,right,bottom int)bool{
 return x>=left&&x<right&&y>=top&&y<bottom
}
func dashboardClicked(d *dashboardState,x,y int){
 // Check actions in visual order, no invisible or overlapping hitboxes.
 if dashboardRectHit(x,y,52,511,272,565) || dashboardRectHit(x,y,1126,760,1400,843){
  dashboardForwardCore(d,1294,768,"PLAY")
  return
 }
 if dashboardRectHit(x,y,282,511,539,565) || dashboardRectHit(x,y,56,760,377,843){
  dashboardForwardCore(d,176,768,"INSTALL / REPAIR")
  return
 }
 if dashboardRectHit(x,y,549,511,720,565)||dashboardRectHit(x,y,881,760,1114,843){
  dashboardForwardCore(d,1019,769,"UPDATE")
  return
 }
 if dashboardRectHit(x,y,730,511,921,565){
  if err:=launchHDManager();err!=nil{messageBoxFallback("Graphics settings: "+err.Error())}
  return
 }
 if dashboardRectHit(x,y,931,511,1151,565){
  dashboardForwardCore(d,1329,30,"SETTINGS")
  return
 }
 if dashboardRectHit(x,y,1161,511,1399,565){
  messageBoxFallback("DawnForge Launcher v1.0.12 TEST — Vanilla 1.12.1\n\nOriginal launcher controls are retained for install, repair, updates and launching.\nModern Vanilla downloads the 13 verified optional HD patch archives. Ultra HD is not released.")
  return
 }
 if dashboardRectHit(x,y,1152,674,1381,721){
  _=exec.Command("rundll32.exe","url.dll,FileProtocolHandler","https://dawnforge.eu/").Start()
  return
 }
 if dashboardRectHit(x,y,90,585,466,644){
  dashboardApplyMode(d,"ORIGINAL")
  return
 }
 if dashboardRectHit(x,y,480,585,887,644){
  dashboardApplyMode(d,"MODERN")
  return
 }
 // Ultra HD is deliberately disabled, not an inert fake click target.
}
func dashboardForwardCore(d *dashboardState,baseX,baseY int,action string){
 // The original binary still owns installation, game patching and PLAY.
 // WM_MOUSE* messages are forwarded to its existing buttons.
 var rc RECT
 if ok,_,_:=procGetClientRect.Call(d.source,uintptr(unsafe.Pointer(&rc)));ok==0{
  messageBoxFallback("Cannot locate the running original launcher.")
  return
 }
 w,h:=int(rc.Right),int(rc.Bottom)
 if w<1||h<1{return}
 x,y:=baseX*w/baseW,baseY*h/baseH
 lparam:=uintptr((uint32(y&0xffff)<<16)|uint32(x&0xffff))
 dashSendMessageW.Call(d.source,dashboardWMMove,0,lparam)
 dashSendMessageW.Call(d.source,dashboardWMDown,1,lparam)
 dashSendMessageW.Call(d.source,dashboardWMUp,0,lparam)
 // We intentionally don't claim that the old core launched a particular
 // client path; selected graphic mode requires a separate Windows E2E check.
}
func dashboardApplyMode(d *dashboardState,mode string){
 d.mu.Lock()
 if d.busy{d.mu.Unlock();return}
 d.busy=true
 d.warning="Checking graphics files"
 d.mu.Unlock()
 dashInvalidate.Call(d.hwnd,0,0)
 defer func(){
  d.mu.Lock();d.busy=false;d.mu.Unlock()
  dashInvalidate.Call(d.hwnd,0,0)
 }()
 settings,err:=loadHDSettings()
 if err!=nil||strings.TrimSpace(settings.GameDir)==""{
  d.mu.Lock();d.warning="Choose your WoW folder";d.mu.Unlock()
  if e:=launchHDManager();e!=nil{messageBoxFallback("HD Manager: "+e.Error())}
  return
 }
 op:="--hd-install"
 if mode=="ORIGINAL"{op="--vanilla-mode"}
 err=hdTask(op,settings.GameDir)
 if err!=nil{
  d.mu.Lock();d.warning="Graphics switch failed";d.mu.Unlock()
  messageBoxFallback("DawnForge graphics: "+err.Error())
  return
 }
 d.mu.Lock()
 d.selected=mode
 if mode=="ORIGINAL"{d.warning="Classic graphics ready"}
 if mode=="MODERN"{d.warning="Modern graphics patches ready"}
 d.mu.Unlock()
}
func updateDashboardPosition(d *dashboardState,orig uintptr){
 if d==nil{return}
 visible,_,_:=procIsWindowVisible.Call(orig)
 iconic,_,_:=procIsIconic.Call(orig)
 fg,_,_:=procGetForegroundWindow.Call()
 if visible==0||iconic!=0||(fg!=orig&&fg!=d.hwnd){
  procShowWindow.Call(d.hwnd,SW_HIDE)
  return
 }
 var rect RECT
 if r,_,_:=procGetWindowRect.Call(orig,uintptr(unsafe.Pointer(&rect)));r==0{return}
 w,h:=int(rect.Right-rect.Left),int(rect.Bottom-rect.Top)
 if w<1||h<1{return}
 procSetWindowPos.Call(d.hwnd,hwndTop,uintptr(rect.Left),uintptr(rect.Top),
  uintptr(w),uintptr(h),SWP_NOACTIVATE|SWP_SHOWWINDOW)
}
func dashboardPolling(d *dashboardState,done <-chan struct{}){
 go func(){
  refresh:=func(){
   s,err:=realm.Fetch(nil)
   if err!=nil{s=realm.Unknown()}
   d.mu.Lock();d.server=s;d.mu.Unlock()
   dashInvalidate.Call(d.hwnd,0,0)
  }
  refresh()
  ticker:=time.NewTicker(30*time.Second)
  defer ticker.Stop()
  for{
   select{
   case <-done:return
   case <-ticker.C:refresh()
   }
  }
 }()
}

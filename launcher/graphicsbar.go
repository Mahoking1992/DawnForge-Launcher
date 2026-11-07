package main

// The sole visible change to the ORIGINAL DawnForge window:
// replace its GAME VERSION tile (left of REALM STATUS) with two
// functional graphics choices. Every other original control is untouched.
import (
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "syscall"
 "unsafe"
)

const (
 modeBarClass = "DawnForgeTwoGraphicsChoicesV1"
 wmPaint = 0x000F
 wmEraseBkgnd = 0x0014
 wmLButtonUp = 0x0202
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
 chosen string
 busy bool
 status string
 width int
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
func newGraphicsBar(orig uintptr)(*graphicsBarState,error){
 instance,_,_:=procGetModuleHandleW.Call(0)
 cls:=utf16Ptr(modeBarClass)
 wc:=WNDCLASSEX{
  CbSize:uint32(unsafe.Sizeof(WNDCLASSEX{})),
  Style:CS_HREDRAW|CS_VREDRAW,
  LpfnWndProc:syscall.NewCallback(graphicsBarProc),
  HInstance:instance,
  LpszClassName:cls,
 }
 if result,_,e:=procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); result==0 && e!=syscall.Errno(1410){
  return nil,fmt.Errorf("graphics selector class: %v",e)
 }
 hwnd,_,e:=procCreateWindowExW.Call(WS_EX_TOOLWINDOW,
  uintptr(unsafe.Pointer(cls)),0,WS_POPUP,0,0,1,1,
  orig,0,instance,0)
 if hwnd==0{return nil,fmt.Errorf("graphics selector window: %v",e)}
 name:=utf16Ptr("Segoe UI")
 size:=int32(-15)
 font,_,_:=procCreateFontW.Call(uintptr(uint32(size)),0,0,0,700,0,0,0,1,0,0,0,0,uintptr(unsafe.Pointer(name)))
 bar:=&graphicsBarState{hwnd:hwnd,original:orig,font:font,width:271}
 graphicsBar=bar
 return bar,nil
}
func rgbBar(r,g,b byte)uintptr {return uintptr(uint32(r)|uint32(g)<<8|uint32(b)<<16)}
func barRect(x,y,w,h int)RECT{return RECT{Left:int32(x),Top:int32(y),Right:int32(x+w),Bottom:int32(y+h)}}
func fillBarRect(hdc uintptr,r RECT,color uintptr){
 brush,_,_:=procCreateSolidBrush.Call(color)
 procFillRect.Call(hdc,uintptr(unsafe.Pointer(&r)),brush)
 procDeleteObject.Call(brush)
}
func drawBarText(hdc uintptr,txt string,r RECT,color uintptr,align uintptr){
 procSetTextColor.Call(hdc,color)
 u,_:=syscall.UTF16PtrFromString(txt)
 procDrawTextW.Call(hdc,uintptr(unsafe.Pointer(u)),^uintptr(0),uintptr(unsafe.Pointer(&r)),align|dtSingleLine|dtVCenter)
}
func drawSelection(hdc uintptr,x,y,w,h int,label string,active,disabled bool){
 bg:=rgbBar(23,26,25)
 border:=rgbBar(112,78,35)
 fg:=rgbBar(236,201,144)
 if active {bg=rgbBar(71,44,14);border=rgbBar(236,166,54)}
 if disabled{fg=rgbBar(132,124,112)}
 r:=barRect(x,y,w,h)
 fillBarRect(hdc,r,bg)
 brush,_,_:=procCreateSolidBrush.Call(border)
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
 var rect RECT
 procGetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&rect)))
 w,h:=int(rect.Right),int(rect.Bottom)
 if w<=0||h<=0{return}
 bar:=graphicsBar
 bar.mu.Lock()
 bar.width=w
 chosen,busy,status:=bar.chosen,bar.busy,bar.status
 bar.mu.Unlock()
 if bar.font!=0{
  old,_,_:=procSelectObject.Call(hdc,bar.font)
  defer procSelectObject.Call(hdc,old)
 }
 procSetBkMode.Call(hdc,transparentBk)
 // Opaque cover of GAME VERSION only. It does not touch REALM STATUS.
 fillBarRect(hdc,barRect(0,0,w,h),rgbBar(13,17,19))
 fillBarRect(hdc,barRect(0,0,w,2),rgbBar(151,105,47))
 fillBarRect(hdc,barRect(0,h-2,w,2),rgbBar(151,105,47))
 sx:=func(v int)int{return v*w/271}
 sy:=func(v int)int{return v*h/65}
 drawSelection(hdc,sx(5),sy(8),sx(125),sy(49),"OLD VANILLA",chosen=="ORIGINAL",busy)
 drawSelection(hdc,sx(140),sy(8),sx(126),sy(49),"MODERN VANILLA",chosen=="MODERN",busy)
 if busy {
  fillBarRect(hdc,barRect(0,0,w,h),rgbBar(13,17,19))
  drawBarText(hdc,"CHECKING GRAPHICS...",barRect(2,0,w-4,h),rgbBar(240,192,101),dtCenter)
 }else if status!="" {
  // Only show errors; normal selection is indicated by the active button.
  fillBarRect(hdc,barRect(0,0,w,h),rgbBar(13,17,19))
  drawBarText(hdc,"GRAPHICS ERROR",barRect(2,0,w-4,h),rgbBar(236,119,103),dtCenter)
 }
}
func graphicsBarProc(hwnd uintptr,msg uint32,wParam,lParam uintptr)uintptr{
 switch msg{
 case wmEraseBkgnd:return 1
 case wmPaint:modeBarPaintHandler(hwnd);return 0
 case wmLButtonUp:
  bar:=graphicsBar
  if bar==nil{return 0}
  x:=int(int16(uint16(lParam&0xffff)))
  bar.mu.Lock()
  width,busy:=bar.width,bar.busy
  bar.mu.Unlock()
  if busy||width<=0{return 0}
  pos:=x*271/width
  if pos>=5&&pos<130{go activateGraphicsMode(bar,"ORIGINAL")}
  if pos>=140&&pos<266{go activateGraphicsMode(bar,"MODERN")}
  return 0
 }
 result,_,_:=procDefWindowProcW.Call(hwnd,uintptr(msg),wParam,lParam)
 return result
}
func pickWoWFolder()(string,error){
 // Only shown when no valid game folder has been configured.
 ps:="$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Windows.Forms;"+
  "$f=New-Object System.Windows.Forms.FolderBrowserDialog;"+
  "$f.Description='Choose your existing WoW 1.12.1 folder (containing WoW.exe and Data)';"+
  "if($f.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){[Console]::Write($f.SelectedPath)}"
 cmd:=exec.Command("powershell.exe","-NoProfile","-STA","-Command",ps)
 bytes,err:=cmd.Output()
 if err!=nil{return "",err}
 return strings.TrimSpace(string(bytes)),nil
}
func validWoWFolder(dir string)bool{
 if dir==""{return false}
 if st,e:=os.Stat(filepath.Join(dir,"Data"));e!=nil||!st.IsDir(){return false}
 if _,e:=os.Stat(filepath.Join(dir,"WoW.exe"));e==nil{return true}
 if _,e:=os.Stat(filepath.Join(dir,"wow.exe"));e==nil{return true}
 return false
}
func activateGraphicsMode(bar *graphicsBarState,mode string){
 runtime.LockOSThread()
 defer runtime.UnlockOSThread()
 bar.mu.Lock()
 if bar.busy{bar.mu.Unlock();return}
 bar.busy=true
 bar.status=""
 bar.mu.Unlock()
 procInvalidateRect.Call(bar.hwnd,0,0)
 defer func(){
  bar.mu.Lock();bar.busy=false;bar.mu.Unlock()
  procInvalidateRect.Call(bar.hwnd,0,0)
 }()
 s,err:=loadHDSettings()
 dir:=s.GameDir
 if err!=nil || !validWoWFolder(dir){
  dir,err=pickWoWFolder()
  if err!=nil || dir==""{
   bar.mu.Lock();bar.status="folder not selected";bar.mu.Unlock()
   messageBoxFallback("DawnForge: select the folder containing WoW.exe and Data. The original INSTALL button remains available.")
   return
  }
  if !validWoWFolder(dir){
   bar.mu.Lock();bar.status="invalid WoW folder";bar.mu.Unlock()
   messageBoxFallback("DawnForge: this folder does not contain WoW.exe and Data. Use the original INSTALL button first.")
   return
  }
  if err=storeHDSettings(hdSettings{GameDir:dir});err!=nil{
   messageBoxFallback("DawnForge could not save the WoW folder: "+err.Error());return
  }
 }
 operation:="--hd-install"
 if mode=="ORIGINAL"{operation="--vanilla-mode"}
 if err=hdTask(operation,dir);err!=nil{
  bar.mu.Lock();bar.status=err.Error();bar.mu.Unlock()
  messageBoxFallback("DawnForge graphics: "+err.Error())
  return
 }
 bar.mu.Lock();bar.chosen=mode;bar.status="";bar.mu.Unlock()
}
func updateGraphicsBarPosition(bar *graphicsBarState,orig uintptr){
 vis,_,_:=procIsWindowVisible.Call(orig)
 icon,_,_:=procIsIconic.Call(orig)
 fg,_,_:=procGetForegroundWindow.Call()
 if vis==0||icon!=0||(fg!=orig&&fg!=bar.hwnd){
  procShowWindow.Call(bar.hwnd,SW_HIDE)
  return
 }
 var rect RECT
 if ok,_,_:=procGetWindowRect.Call(orig,uintptr(unsafe.Pointer(&rect)));ok==0{return}
 ww,wh:=int(rect.Right-rect.Left),int(rect.Bottom-rect.Top)
 if ww<=0||wh<=0{return}
 x:=int(rect.Left)+24*ww/baseW
 y:=int(rect.Top)+632*wh/baseH
 w:=271*ww/baseW
 h:=65*wh/baseH
 if h<32{h=32}
 procSetWindowPos.Call(bar.hwnd,hwndTop,uintptr(x),uintptr(y),uintptr(w),uintptr(h),SWP_NOACTIVATE|SWP_SHOWWINDOW)
}
func pollWindowsMessages(){
 var m winMsg
 for{
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
 graphicsBar=nil
}

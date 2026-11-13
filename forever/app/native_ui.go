//go:build windows

// Original full-bleed DawnForge Forever artwork, with native Win32 click zones.
// The graphic is intentionally NOT cut into a separate image and control panel.
// Nothing in this UI starts PowerShell or modifies the Vanilla installation.
package main

import (
 "crypto/sha256"
 "os"
 "path/filepath"
 "strings"
 "sync"
 "syscall"
 "unsafe"
)

const (
 canvasWidth int32=1536
 canvasHeight int32=1024
 wmDestroy=2; wmPaint=15; wmClose=16; wmEraseBkgnd=20
 wmLButtonDown=0x0201; wmLButtonUp=0x0202; wmMouseMove=0x0200
 wmNCLButtonDown=0x00A1; htCaption=2
 wsPopup=0x80000000; wsVisible=0x10000000
 dtCenter=1; dtVCenter=4; dtSingleLine=32; dtEndEllipsis=0x8000
 bkTransparent=1
 cmdHome=1; cmdWebsite=2; cmdDownloads=3; cmdUpdates=4
 cmdOptions=5; cmdPlay=6; cmdClose=7; cmdMinimize=8; cmdMaximize=9
)
type winRect struct{Left,Top,Right,Bottom int32}
type winPoint struct{X,Y int32}
type winPaint struct{DC uintptr; Erase int32; Bounds winRect; Restore int32; Update int32; Reserved [32]byte}
type winMsg struct{Window uintptr; Message uint32; Padding uint32; WParam,LParam uintptr; Time uint32; Position winPoint; Private uint32}
type winClass struct{Size,Style uint32; WindowProc uintptr; ClassExtra,WindowExtra int32; Instance,Icon,Cursor,Background uintptr; Menu,Name *uint16; SmallIcon uintptr}
type browseInfo struct{Owner,Root uintptr; DisplayName,Title *uint16; Flags uint32; Callback,Param uintptr; Image int32}
type gdipInput struct{Version uint32; Callback uintptr; NoBackgroundThread,NoExternalCodecs uint32}
type viewport struct{X,Y,W,H int32}
type clickZone struct{ID int; X,Y,W,H int32}
var clickZones=[]clickZone{
 {cmdHome,53,421,298,75},
 {cmdWebsite,358,421,280,75},
 {cmdDownloads,649,421,276,75},
 {cmdUpdates,929,421,279,75},
 {cmdOptions,1214,421,280,75},
 {cmdPlay,1065,773,434,125},
 {cmdClose,1482,0,54,50},
 {cmdMinimize,1355,0,55,50},
 {cmdMaximize,1422,0,55,50},
}
var (
 user32=syscall.NewLazyDLL("user32.dll")
 gdi32=syscall.NewLazyDLL("gdi32.dll")
 kernel32=syscall.NewLazyDLL("kernel32.dll")
 shell32=syscall.NewLazyDLL("shell32.dll")
 ole32=syscall.NewLazyDLL("ole32.dll")
 gdip=syscall.NewLazyDLL("gdiplus.dll")
 registerClass=user32.NewProc("RegisterClassExW")
 createWindow=user32.NewProc("CreateWindowExW")
 defWindow=user32.NewProc("DefWindowProcW")
 showWindow=user32.NewProc("ShowWindow")
 updateWindow=user32.NewProc("UpdateWindow")
 getMessage=user32.NewProc("GetMessageW")
 translateMessage=user32.NewProc("TranslateMessage")
 dispatchMessage=user32.NewProc("DispatchMessageW")
 postQuit=user32.NewProc("PostQuitMessage")
 sendMessage=user32.NewProc("SendMessageW")
 invalidateRect=user32.NewProc("InvalidateRect")
 releaseCapture=user32.NewProc("ReleaseCapture")
 getClientRect=user32.NewProc("GetClientRect")
 systemParameters=user32.NewProc("SystemParametersInfoW")
 messageBox=user32.NewProc("MessageBoxW")
 beginPaint=user32.NewProc("BeginPaint")
 endPaint=user32.NewProc("EndPaint")
 fillRect=user32.NewProc("FillRect")
 drawTextAPI=user32.NewProc("DrawTextW")
 setBkMode=gdi32.NewProc("SetBkMode")
 setTextColor=gdi32.NewProc("SetTextColor")
 createBrush=gdi32.NewProc("CreateSolidBrush")
 createPen=gdi32.NewProc("CreatePen")
 roundRect=gdi32.NewProc("RoundRect")
 deleteObject=gdi32.NewProc("DeleteObject")
 createFont=gdi32.NewProc("CreateFontW")
 selectObject=gdi32.NewProc("SelectObject")
 getModule=kernel32.NewProc("GetModuleHandleW")
 loadCursor=user32.NewProc("LoadCursorW")
 setCursor=user32.NewProc("SetCursor")
 browseFolderAPI=shell32.NewProc("SHBrowseForFolderW")
 getPathAPI=shell32.NewProc("SHGetPathFromIDListW")
 shellExecute=shell32.NewProc("ShellExecuteW")
 coFree=ole32.NewProc("CoTaskMemFree")
 coInitialize=ole32.NewProc("CoInitializeEx")
 gdipStartup=gdip.NewProc("GdiplusStartup")
 gdipShutdown=gdip.NewProc("GdiplusShutdown")
 gdipLoad=gdip.NewProc("GdipCreateBitmapFromFile")
 gdipDispose=gdip.NewProc("GdipDisposeImage")
 gdipGraphics=gdip.NewProc("GdipCreateFromHDC")
 gdipDeleteGraphics=gdip.NewProc("GdipDeleteGraphics")
 gdipDraw=gdip.NewProc("GdipDrawImageRectI")
 windowHandle,artHandle,gdipToken uintptr
 uiMu sync.RWMutex
 uiStatus="Ready. DOWNLOAD downloads the game; PLAY launches Forever."
 uiFolder="No Forever game folder selected."
 uiRealm="Checking Forever realm status..."
 uiProgress=-1
)
func utf16ptr(s string)*uint16{return syscall.StringToUTF16Ptr(s)}
func strPtr(s string)uintptr{return uintptr(unsafe.Pointer(utf16ptr(s)))}
func color(r,g,b byte)uintptr{return uintptr(r)|uintptr(g)<<8|uintptr(b)<<16}
func makeBrush(c uintptr)uintptr{r,_,_:=createBrush.Call(c);return r}
func refreshUI(){if windowHandle!=0{invalidateRect.Call(windowHandle,0,0)}}
func setStatus(s string){uiMu.Lock();uiStatus=s;uiMu.Unlock();refreshUI()}
func setFolder(s string){uiMu.Lock();uiFolder=s;uiMu.Unlock();refreshUI()}
func setRealm(s string){uiMu.Lock();uiRealm=s;uiMu.Unlock();refreshUI()}
func setProgress(i int){uiMu.Lock();uiProgress=i;uiMu.Unlock();refreshUI()}
func showWarning(s string){messageBox.Call(windowHandle,strPtr(s),strPtr("DawnForge Forever"),0x30)}
func showInfo(s string){messageBox.Call(windowHandle,strPtr(s),strPtr("DawnForge Forever"),0x40)}

func artworkViewport(width,height int32)viewport{
 if width<=0||height<=0{return viewport{}}
 if int64(width)*int64(canvasHeight)<=int64(height)*int64(canvasWidth){
  h:=int32(int64(width)*int64(canvasHeight)/int64(canvasWidth))
  return viewport{0,(height-h)/2,width,h}
 }
 w:=int32(int64(height)*int64(canvasWidth)/int64(canvasHeight))
 return viewport{(width-w)/2,0,w,height}
}
func hitArtwork(x,y int32)int{
 for _,zone:=range clickZones{
  if x>=zone.X&&x<zone.X+zone.W&&y>=zone.Y&&y<zone.Y+zone.H{return zone.ID}
 }
 return 0
}
func artPoint(clientX,clientY int32,view viewport)(int32,int32,bool){
 if view.W<=0||view.H<=0||clientX<view.X||clientY<view.Y||clientX>=view.X+view.W||clientY>=view.Y+view.H{return 0,0,false}
 return int32(int64(clientX-view.X)*int64(canvasWidth)/int64(view.W)),
  int32(int64(clientY-view.Y)*int64(canvasHeight)/int64(view.H)),true
}
func getViewport(w uintptr)viewport{
 var r winRect
 getClientRect.Call(w,uintptr(unsafe.Pointer(&r)))
 return artworkViewport(r.Right-r.Left,r.Bottom-r.Top)
}
func clickAt(w uintptr,lp uintptr)int{
 x:=int32(int16(uint16(lp)))
 y:=int32(int16(uint16(lp>>16)))
 ax,ay,ok:=artPoint(x,y,getViewport(w))
 if !ok{return 0}
 return hitArtwork(ax,ay)
}
func scaledRect(v viewport,x,y,w,h int32)winRect{
 left:=v.X+int32(int64(x)*int64(v.W)/int64(canvasWidth))
 top:=v.Y+int32(int64(y)*int64(v.H)/int64(canvasHeight))
 right:=v.X+int32(int64(x+w)*int64(v.W)/int64(canvasWidth))
 bottom:=v.Y+int32(int64(y+h)*int64(v.H)/int64(canvasHeight))
 return winRect{left,top,right,bottom}
}
func fillCanvas(dc uintptr,v viewport,x,y,w,h int32,ink uintptr){
 area:=scaledRect(v,x,y,w,h)
 b:=makeBrush(ink)
 fillRect.Call(dc,uintptr(unsafe.Pointer(&area)),b)
 deleteObject.Call(b)
}
func textCanvas(dc uintptr,v viewport,s string,x,y,w,h,size int32,bold bool,ink uintptr){
 area:=scaledRect(v,x,y,w,h)
 px:=int32(int64(size)*int64(v.H)/int64(canvasHeight))
 if px<11{px=11}
 weight:=uintptr(400);if bold{weight=700}
 font,_,_:=createFont.Call(uintptr(-px),0,0,0,weight,0,0,0,0,0,0,0,0,strPtr("Segoe UI"))
 if font==0{return}
 defer deleteObject.Call(font)
 previous,_,_:=selectObject.Call(dc,font)
 setBkMode.Call(dc,bkTransparent)
 setTextColor.Call(dc,ink)
 drawTextAPI.Call(dc,strPtr(s),^uintptr(0),uintptr(unsafe.Pointer(&area)),dtVCenter|dtSingleLine|dtEndEllipsis)
 selectObject.Call(dc,previous)
}
func panelCanvas(dc uintptr,v viewport,x,y,w,h int32){
 area:=scaledRect(v,x,y,w,h)
 back:=makeBrush(color(7,19,36))
 stroke,_,_:=createPen.Call(0,2,color(170,133,71))
 oldBrush,_,_:=selectObject.Call(dc,back)
 oldPen,_,_:=selectObject.Call(dc,stroke)
 roundRect.Call(dc,uintptr(area.Left),uintptr(area.Top),uintptr(area.Right),uintptr(area.Bottom),14,14)
 selectObject.Call(dc,oldBrush);selectObject.Call(dc,oldPen)
 deleteObject.Call(back);deleteObject.Call(stroke)
}
func drawScreen(w uintptr){
 var ps winPaint
 dc,_,_:=beginPaint.Call(w,uintptr(unsafe.Pointer(&ps)))
 defer endPaint.Call(w,uintptr(unsafe.Pointer(&ps)))
 var client winRect
 getClientRect.Call(w,uintptr(unsafe.Pointer(&client)))
 bg:=makeBrush(color(5,13,27))
 fillRect.Call(dc,uintptr(unsafe.Pointer(&client)),bg)
 deleteObject.Call(bg)
 v:=artworkViewport(client.Right-client.Left,client.Bottom-client.Top)
 if artHandle!=0{
  var graphics uintptr
  if result,_,_:=gdipGraphics.Call(dc,uintptr(unsafe.Pointer(&graphics)));result==0{
   gdipDraw.Call(graphics,artHandle,uintptr(v.X),uintptr(v.Y),uintptr(v.W),uintptr(v.H))
   gdipDeleteGraphics.Call(graphics)
  }
 }
 // The complete approved artwork is visible. Only the same lower-left status
 // area used by the original WPF UI is overlaid, with dynamic download progress.
 panelCanvas(dc,v,35,765,995,150)
 uiMu.RLock()
 status,folder,realm,progress:=uiStatus,uiFolder,uiRealm,uiProgress
 uiMu.RUnlock()
 textCanvas(dc,v,realm,58,780,940,31,21,true,color(231,195,128))
 textCanvas(dc,v,folder,58,811,940,28,19,false,color(218,232,243))
 textCanvas(dc,v,status,58,841,940,35,20,false,color(238,220,181))
 if progress>=0{
  fillCanvas(dc,v,58,887,940,12,color(23,47,69))
  if progress>100{progress=100}
  if progress>0{fillCanvas(dc,v,58,887,int32(progress)*940/100,12,color(63,194,106))}
 }
}
func pickFolder(){
 path:=openFolderDialog()
 if path==""{return}
 if err:=chooseGameDir(path);err!=nil{
  showWarning(err.Error())
 }else{
  setFolder("Game folder: "+gameDir())
  setStatus("Game folder saved. Click DOWNLOADS to repair or PLAY to start.")
 }
}
func beginDownload(){
 if gameDir()==""{
  showInfo("Choose an EMPTY Forever installation folder, or your existing Forever client folder. Downloaded files will stay in that location. Vanilla is not modified.")
  if path:=openFolderDialog();path==""{return}else if err:=chooseGameDir(path);err!=nil{showWarning(err.Error());return}
  setFolder("Game folder: "+gameDir())
 }
 runTask(syncClientFiles)
}
func handleClick(id int){
 switch id{
 case cmdHome:
  setStatus("DawnForge Forever launcher is ready.")
  go refreshStatus()
 case cmdWebsite:
  shellExecute.Call(windowHandle,strPtr("open"),strPtr(websiteURL),0,0,1)
 case cmdDownloads:beginDownload()
 case cmdUpdates:runTask(checkLauncherUpdates)
 case cmdOptions:pickFolder()
 case cmdPlay:runTask(playGame)
 case cmdClose:sendMessage.Call(windowHandle,wmClose,0,0)
 case cmdMinimize:showWindow.Call(windowHandle,6)
 case cmdMaximize:
  // Keep the original 3:2 visual composition: use standard restore/maximize,
  // with aspect-ratio-safe letterboxing on wide monitors.
  // SW_MAXIMIZE=3, SW_RESTORE=9, IsZoomed is queried separately.
  isZoomed:=user32.NewProc("IsZoomed")
  zoomed,_,_:=isZoomed.Call(windowHandle)
  if zoomed!=0{showWindow.Call(windowHandle,9)}else{showWindow.Call(windowHandle,3)}
 }
}
func nativeWndProc(w uintptr,code uint32,wp,lp uintptr)uintptr{
 switch code{
 case wmEraseBkgnd:return 1
 case wmPaint:drawScreen(w);return 0
 case wmLButtonUp:
  handleClick(clickAt(w,lp))
  return 0
 case wmLButtonDown:
  // Drag by empty space in the top decorative header, not by the control icons.
  if id:=clickAt(w,lp);id==0{
   y:=int32(int16(uint16(lp>>16)))
   v:=getViewport(w)
   if y>=v.Y&&y<v.Y+int32(int64(60)*int64(v.H)/int64(canvasHeight)){
    releaseCapture.Call()
    sendMessage.Call(w,wmNCLButtonDown,htCaption,0)
   }
  }
  return 0
 case wmMouseMove:
  cursor:=uintptr(32512)
  if clickAt(w,lp)!=0{cursor=32649}
  c,_,_:=loadCursor.Call(0,cursor)
  setCursor.Call(c)
  return 0
 case wmDestroy:
  if artHandle!=0{gdipDispose.Call(artHandle)}
  if gdipToken!=0{gdipShutdown.Call(gdipToken)}
  postQuit.Call(0)
  return 0
 }
 result,_,_:=defWindow.Call(w,uintptr(code),wp,lp)
 return result
}
func openFolderDialog()string{
 var text [260]uint16
 prompt:=browseInfo{Owner:windowHandle,DisplayName:&text[0],Title:utf16ptr("Choose Forever client folder (or a new empty download folder)"),Flags:1|0x40}
 pidl,_,_:=browseFolderAPI.Call(uintptr(unsafe.Pointer(&prompt)))
 if pidl==0{return ""}
 defer coFree.Call(pidl)
 var out [32768]uint16
 ok,_,_:=getPathAPI.Call(pidl,uintptr(unsafe.Pointer(&out[0])))
 if ok==0{return ""}
 return syscall.UTF16ToString(out[:])
}
func startWindow(){
 imagePath:=filepath.Join(appFolder,"artwork.jpg")
 if previous,err:=os.ReadFile(imagePath);err!=nil||sha256.Sum256(previous)!=sha256.Sum256(artwork){
  _=os.WriteFile(imagePath,artwork,0600)
 }
 input:=gdipInput{Version:1}
 gdipStartup.Call(uintptr(unsafe.Pointer(&gdipToken)),uintptr(unsafe.Pointer(&input)),0)
 if gdipToken!=0{gdipLoad.Call(strPtr(imagePath),uintptr(unsafe.Pointer(&artHandle)))}
 coInitialize.Call(0,2)
 instance,_,_:=getModule.Call(0)
 cursor,_,_:=loadCursor.Call(0,32512)
 className:=utf16ptr("DawnForgeForeverFullArtworkWindow")
 wc:=winClass{Size:uint32(unsafe.Sizeof(winClass{})),Style:3,WindowProc:syscall.NewCallback(nativeWndProc),Instance:instance,Cursor:cursor,Name:className}
 if registered,_,_:=registerClass.Call(uintptr(unsafe.Pointer(&wc)));registered==0{return}
 var work winRect
 if ok,_,_:=systemParameters.Call(48,0,uintptr(unsafe.Pointer(&work)),0);ok==0{
  work=winRect{0,0,1440,900}
 }
 availableW:=work.Right-work.Left-32
 availableH:=work.Bottom-work.Top-32
 width:=int32(1229)
 if width>availableW{width=availableW}
 height:=int32(int64(width)*int64(canvasHeight)/int64(canvasWidth))
 if height>availableH{
  height=availableH
  width=int32(int64(height)*int64(canvasWidth)/int64(canvasHeight))
 }
 if width<600||height<400{return}
 x:=work.Left+(work.Right-work.Left-width)/2
 y:=work.Top+(work.Bottom-work.Top-height)/2
 hwnd,_,_:=createWindow.Call(0,uintptr(unsafe.Pointer(className)),strPtr("DawnForge Forever Launcher "+launcherVersion),
  wsPopup|wsVisible,uintptr(x),uintptr(y),uintptr(width),uintptr(height),0,0,instance,0)
 if hwnd==0{return}
 windowHandle=hwnd
 showWindow.Call(hwnd,1)
 updateWindow.Call(hwnd)
 if cfg.GameDir!=""{setFolder("Game folder: "+cfg.GameDir)}
 go refreshStatus()
 var message winMsg
 for{
  result,_,_:=getMessage.Call(uintptr(unsafe.Pointer(&message)),0,0,0)
  if result==0||result==^uintptr(0){break}
  translateMessage.Call(uintptr(unsafe.Pointer(&message)))
  dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
 }
}

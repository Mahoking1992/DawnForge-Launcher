//go:build windows

// Native Win32 user interface; no PowerShell, cmd.exe or hidden scripts.
package main

import (
 "crypto/sha256"
 "os"
 "path/filepath"
 "syscall"
 "unsafe"
)

const (
 wmCreate=1; wmDestroy=2; wmPaint=15; wmCommand=273; wmDrawItem=43
 wmColorStatic=312; wmSetFont=48
 wsChild=0x40000000; wsVisible=0x10000000; wsTabStop=0x10000
 wsCaption=0xc00000; wsSysMenu=0x80000; wsMinimizeBox=0x20000
 bsOwnerDraw=11; transparent=1
 dtCenter=1; dtVCenter=4; dtSingleLine=32
 webID=101; folderID=102; updatesID=103; repairID=104; playID=105
)
type winRect struct {Left,Top,Right,Bottom int32}
type winPoint struct {X,Y int32}
type winPaint struct {DC uintptr; Erase int32; Bounds winRect; Restore int32; Update int32; Reserved [32]byte}
type winMsg struct {Window uintptr; Message uint32; Padding uint32; WParam,LParam uintptr; Time uint32; Position winPoint; Private uint32}
type winClass struct {Size,Style uint32; WindowProc uintptr; ClassExtra,WindowExtra int32; Instance,Icon,Cursor,Background uintptr; Menu,Name *uint16; SmallIcon uintptr}
type drawInfo struct {ControlType,ControlID,ItemID,Action,State uint32; Window,DC uintptr; Bounds winRect; Data uintptr}
type browseInfo struct {Owner,Root uintptr; DisplayName,Title *uint16; Flags uint32; Callback,Param uintptr; Image int32}
type gdipInput struct {Version uint32; Callback uintptr; NoBackgroundThread,NoExternalCodecs uint32}
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
 setWindowText=user32.NewProc("SetWindowTextW")
 messageBox=user32.NewProc("MessageBoxW")
 beginPaint=user32.NewProc("BeginPaint")
 endPaint=user32.NewProc("EndPaint")
 getClientRect=user32.NewProc("GetClientRect")
 fillRect=user32.NewProc("FillRect")
 drawTextAPI=user32.NewProc("DrawTextW")
 setBkMode=gdi32.NewProc("SetBkMode")
 setTextColor=gdi32.NewProc("SetTextColor")
 createBrush=gdi32.NewProc("CreateSolidBrush")
 deleteObject=gdi32.NewProc("DeleteObject")
 createFont=gdi32.NewProc("CreateFontW")
 selectObject=gdi32.NewProc("SelectObject")
 getModule=kernel32.NewProc("GetModuleHandleW")
 loadCursor=user32.NewProc("LoadCursorW")
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

 windowHandle,statusHandle,folderHandle,realmHandle uintptr
 headlineFont,buttonFont,bgBrush uintptr
 artHandle,gdipToken uintptr
)
func utf16ptr(s string)*uint16{return syscall.StringToUTF16Ptr(s)}
func strPtr(s string)uintptr{return uintptr(unsafe.Pointer(utf16ptr(s)))}
func color(r,g,b byte)uintptr{return uintptr(r)|uintptr(g)<<8|uintptr(b)<<16}
func makeBrush(c uintptr)uintptr{r,_,_:=createBrush.Call(c);return r}
func setStatus(s string){if statusHandle!=0{setWindowText.Call(statusHandle,strPtr(s))}}
func setFolder(s string){if folderHandle!=0{setWindowText.Call(folderHandle,strPtr(s))}}
func setRealm(s string){if realmHandle!=0{setWindowText.Call(realmHandle,strPtr(s))}}
func showWarning(s string){messageBox.Call(windowHandle,strPtr(s),strPtr("DawnForge Forever"),0x30)}
func showInfo(s string){messageBox.Call(windowHandle,strPtr(s),strPtr("DawnForge Forever"),0x40)}
func createButton(id int,x,y,w,h int32,label string){
 hBtn,_,_:=createWindow.Call(0,strPtr("BUTTON"),strPtr(label),wsChild|wsVisible|wsTabStop|bsOwnerDraw,uintptr(x),uintptr(y),uintptr(w),uintptr(h),windowHandle,uintptr(id),0,0)
 sendMessage.Call(hBtn,wmSetFont,buttonFont,1)
}
func createLabel(x,y,w,h int32,label string)uintptr{
 hLabel,_,_:=createWindow.Call(0,strPtr("STATIC"),strPtr(label),wsChild|wsVisible,uintptr(x),uintptr(y),uintptr(w),uintptr(h),windowHandle,0,0,0)
 sendMessage.Call(hLabel,wmSetFont,buttonFont,1)
 return hLabel
}
func drawLabel(dc uintptr,s string,x,y,w,h int32,ink uintptr,font uintptr,flags uintptr){
 r:=winRect{x,y,x+w,y+h}
 setBkMode.Call(dc,transparent)
 setTextColor.Call(dc,ink)
 previous,_,_:=selectObject.Call(dc,font)
 drawTextAPI.Call(dc,strPtr(s),^uintptr(0),uintptr(unsafe.Pointer(&r)),flags|dtSingleLine|dtVCenter)
 selectObject.Call(dc,previous)
}
func drawScreen(w uintptr){
 var ps winPaint
 dc,_,_:=beginPaint.Call(w,uintptr(unsafe.Pointer(&ps)))
 defer endPaint.Call(w,uintptr(unsafe.Pointer(&ps)))
 var client winRect
 getClientRect.Call(w,uintptr(unsafe.Pointer(&client)))
 if artHandle!=0 {
  var graphics uintptr
  if result,_,_:=gdipGraphics.Call(dc,uintptr(unsafe.Pointer(&graphics)));result==0{
   gdipDraw.Call(graphics,artHandle,0,0,uintptr(client.Right),485)
   gdipDeleteGraphics.Call(graphics)
  }
 }else{
  r:=winRect{0,0,client.Right,485};fillRect.Call(dc,uintptr(unsafe.Pointer(&r)),bgBrush)
 }
 overlay:=winRect{0,0,495,485}
 fillRect.Call(dc,uintptr(unsafe.Pointer(&overlay)),bgBrush)
 bottom:=winRect{0,485,client.Right,client.Bottom}
 fillRect.Call(dc,uintptr(unsafe.Pointer(&bottom)),bgBrush)
 drawLabel(dc,"DAWNFORGE",36,26,440,70,color(234,183,106),headlineFont,0)
 drawLabel(dc,"FOREVER",36,82,440,70,color(230,240,251),headlineFont,0)
 drawLabel(dc,"An independent world awaits.",38,165,420,30,color(181,201,225),buttonFont,0)
 drawLabel(dc,"Select your existing client and enter Azeroth.",38,208,442,30,color(181,201,225),buttonFont,0)
 drawLabel(dc,"NATIVE LAUNCHER   "+launcherVersion,38,247,442,30,color(216,185,125),buttonFont,0)
 drawLabel(dc,"REALM STATUS",38,513,260,28,color(227,181,109),buttonFont,0)
 drawLabel(dc,"YOUR GAME FOLDER",38,603,340,29,color(227,181,109),buttonFont,0)
}
func paintButton(data uintptr){
 info:=(*drawInfo)(unsafe.Pointer(data))
 bounds:=info.Bounds
 base:=color(25,54,82)
 if info.ControlID==playID{base=color(26,95,130)}
 if info.State&1!=0{base=color(47,85,113)}
 b:=makeBrush(base)
 fillRect.Call(info.DC,uintptr(unsafe.Pointer(&bounds)),b)
 deleteObject.Call(b)
 labels:=map[uint32]string{
  webID:"WEBSITE",folderID:"CHOOSE GAME FOLDER",updatesID:"CHECK UPDATES",
  repairID:"DOWNLOAD / REPAIR",playID:"PLAY",
 }
 drawLabel(info.DC,labels[info.ControlID],bounds.Left,bounds.Top,bounds.Right-bounds.Left,bounds.Bottom-bounds.Top,color(244,221,169),buttonFont,dtCenter)
}
func nativeWndProc(w uintptr,code uint32,wp,lp uintptr)uintptr{
 switch code{
 case wmCreate:
  windowHandle=w
  bgBrush=makeBrush(color(9,18,33))
  headlineFont,_,_=createFont.Call(42,0,0,0,700,0,0,0,0,0,0,0,0,strPtr("Segoe UI"))
  buttonFont,_,_=createFont.Call(17,0,0,0,600,0,0,0,0,0,0,0,0,strPtr("Segoe UI"))
  createButton(webID,38,318,199,54,"WEBSITE")
  createButton(folderID,251,318,219,54,"CHOOSE GAME FOLDER")
  createButton(updatesID,38,399,199,54,"CHECK UPDATES")
  createButton(repairID,251,399,219,54,"DOWNLOAD / REPAIR")
  realmHandle=createLabel(38,554,825,34,"Checking Forever server...")
  folderHandle=createLabel(38,646,900,32,"No game directory selected.")
  statusHandle=createLabel(38,700,900,43,"Choose an existing Forever client folder.")
  createButton(playID,921,615,218,102,"PLAY")
  return 0
 case wmDrawItem:
  if lp!=0{paintButton(lp);return 1}
 case wmColorStatic:
  setTextColor.Call(wp,color(228,239,248));setBkMode.Call(wp,transparent)
  return bgBrush
 case wmPaint:
  drawScreen(w);return 0
 case wmCommand:
  switch int(wp&0xffff){
  case webID:
   shellExecute.Call(windowHandle,strPtr("open"),strPtr(websiteURL),0,0,1)
  case folderID:
   if path:=openFolderDialog();path!=""{
    if err:=chooseGameDir(path);err!=nil{showWarning(err.Error())}else{
     setFolder(gameDir());setStatus("Folder saved. Click PLAY or DOWNLOAD / REPAIR.")
    }
   }
  case updatesID:runTask(checkLauncherUpdates)
  case repairID:runTask(syncClientFiles)
  case playID:runTask(playGame)
  }
  return 0
 case wmDestroy:
  if artHandle!=0{gdipDispose.Call(artHandle)}
  if gdipToken!=0{gdipShutdown.Call(gdipToken)}
  for _,obj:=range []uintptr{headlineFont,buttonFont,bgBrush}{if obj!=0{deleteObject.Call(obj)}}
  postQuit.Call(0);return 0
 }
 v,_,_:=defWindow.Call(w,uintptr(code),wp,lp)
 return v
}
func openFolderDialog()string{
 var name [260]uint16
 info:=browseInfo{Owner:windowHandle,DisplayName:&name[0],Title:utf16ptr("Choose the existing WoW Forever game folder"),Flags:1|0x40}
 pidl,_,_:=browseFolderAPI.Call(uintptr(unsafe.Pointer(&info)))
 if pidl==0{return ""}
 defer coFree.Call(pidl)
 var out [32768]uint16
 result,_,_:=getPathAPI.Call(pidl,uintptr(unsafe.Pointer(&out[0])))
 if result==0{return ""}
 return syscall.UTF16ToString(out[:])
}
func startWindow(){
 imagePath:=filepath.Join(appFolder,"artwork.jpg")
 if existing,err:=os.ReadFile(imagePath);err!=nil||sha256.Sum256(existing)!=sha256.Sum256(artwork){
  _=os.WriteFile(imagePath,artwork,0600)
 }
 gdipInit:=gdipInput{Version:1}
 gdipStartup.Call(uintptr(unsafe.Pointer(&gdipToken)),uintptr(unsafe.Pointer(&gdipInit)),0)
 if gdipToken!=0{gdipLoad.Call(strPtr(imagePath),uintptr(unsafe.Pointer(&artHandle)))}
 coInitialize.Call(0,2)
 instance,_,_:=getModule.Call(0)
 cursor,_,_:=loadCursor.Call(0,32512)
 className:=utf16ptr("DawnForgeForeverNativeLauncher")
 wc:=winClass{Size:uint32(unsafe.Sizeof(winClass{})),Style:3,WindowProc:syscall.NewCallback(nativeWndProc),Instance:instance,Cursor:cursor,Name:className}
 if ok,_,_:=registerClass.Call(uintptr(unsafe.Pointer(&wc)));ok==0{return}
 h,_,_:=createWindow.Call(0,uintptr(unsafe.Pointer(className)),strPtr("DawnForge Forever Launcher "+launcherVersion),wsCaption|wsSysMenu|wsMinimizeBox|wsVisible,120,85,1190,810,0,0,instance,0)
 if h==0{return}
 showWindow.Call(h,1)
 updateWindow.Call(h)
 if cfg.GameDir!=""{setFolder(cfg.GameDir)}
 go refreshStatus()
 var message winMsg
 for{
  result,_,_:=getMessage.Call(uintptr(unsafe.Pointer(&message)),0,0,0)
  if result==0||result==^uintptr(0){break}
  translateMessage.Call(uintptr(unsafe.Pointer(&message)))
  dispatchMessage.Call(uintptr(unsafe.Pointer(&message)))
 }
}

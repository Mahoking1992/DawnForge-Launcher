package main

// Test-only enhancement for the existing DawnForge window.
// It does not change the original launcher executable, its buttons, or game files.
import (
 "encoding/json"
 "fmt"
 "net/http"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "unsafe"
)

const (
 dfWSVisible = 0x10000000
 dfWSChild = 0x40000000
 dfWMCommand = 0x0111
 dfWMLButtonUp = 0x0202
 dfPMRemove = 0x0001
 dfWMSetFont = 0x0030
 dfSWShow = 4
)
var (
 dfCreateWindow = user32.NewProc("CreateWindowExW")
 dfSetWindowText = user32.NewProc("SetWindowTextW")
 dfGetStockObject = gdi32.NewProc("GetStockObject")
 dfPeekMessage = user32.NewProc("PeekMessageW")
 dfTranslateMessage = user32.NewProc("TranslateMessage")
 dfDispatchMessage = user32.NewProc("DispatchMessageW")
 dfGetAncestor = user32.NewProc("GetAncestor")
)

type dfWinMsg struct {
 hwnd uintptr
 message uint32
 _ uint32
 wparam uintptr
 lparam uintptr
 time uint32
 point POINT
 private uint32
}
type dfOptions struct {
 Mode string `json:"mode"`
}
type dfPanel struct {
 hwnd uintptr
 status uintptr
 classic uintptr
 ultra uintptr
 playOverlay uintptr
 parent uintptr
 clicks chan int
 options dfOptions
 updates chan string
 stop chan struct{}
}
var currentDF *dfPanel

func dfWindowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
 if msg == dfWMLButtonUp && currentDF != nil && hwnd == currentDF.playOverlay {
  select { case currentDF.clicks <- 103: default: }
  return 0
 }
 if msg == dfWMCommand && currentDF != nil && hwnd == currentDF.hwnd {
  choice := int(wp & 0xffff)
  if choice == 101 || choice == 102 {
   if choice == 101 { currentDF.options.Mode = "classic" } else {currentDF.options.Mode = "hd"}
   if err := dfSaveOptions(currentDF.options); err != nil {
    dfSetText(currentDF.status,"SETTINGS ERROR: "+err.Error())
   }
   currentDF.setSelected()
   return 0
  }
 }
 r,_,_ := procDefWindowProcW.Call(hwnd,uintptr(msg),wp,lp)
 return r
}

func dfSetText(hwnd uintptr, value string) {
 if hwnd != 0 { dfSetWindowText.Call(hwnd,uintptr(unsafe.Pointer(utf16Ptr(value)))) }
}
func dfChild(parent uintptr, kind, label string, x,y,w,h,id int) uintptr {
 hwnd,_,_:=dfCreateWindow.Call(0,uintptr(unsafe.Pointer(utf16Ptr(kind))),
  uintptr(unsafe.Pointer(utf16Ptr(label))),dfWSChild|dfWSVisible,
  uintptr(x),uintptr(y),uintptr(w),uintptr(h),parent,uintptr(id),0,0)
 if hwnd != 0 {
  font,_,_:=dfGetStockObject.Call(17)
  dfSend:=user32.NewProc("SendMessageW")
  dfSend.Call(hwnd,dfWMSetFont,font,1)
 }
 return hwnd
}
func dfSettingsPath() string {
 dir:=os.Getenv("LOCALAPPDATA")
 if dir=="" {dir=os.TempDir()}
 return filepath.Join(dir,"DawnForge","Launcher","graphics-choice.json")
}
func dfReadOptions() dfOptions {
 o:=dfOptions{Mode:"classic"}
 data,err:=os.ReadFile(dfSettingsPath())
 if err==nil {_=json.Unmarshal(data,&o)}
 if !strings.EqualFold(o.Mode,"hd") {o.Mode="classic"} else {o.Mode="hd"}
 return o
}
func dfSaveOptions(o dfOptions) error {
 path:=dfSettingsPath()
 if err:=os.MkdirAll(filepath.Dir(path),0700);err!=nil{return err}
 b,err:=json.MarshalIndent(o,"","  ")
 if err!=nil {return err}
 temp:=path+".tmp"
 if err:=os.WriteFile(temp,b,0600);err!=nil{return err}
 if err:=os.Rename(temp,path);err!=nil {
  _=os.Remove(path)
  return os.Rename(temp,path)
 }
 return nil
}
func (p *dfPanel) setSelected() {
 if p.options.Mode=="hd" {
  dfSetText(p.classic,"CLASSIC")
  dfSetText(p.ultra,"[ULTRA HD]")
 } else {
  dfSetText(p.classic,"[CLASSIC]")
  dfSetText(p.ultra,"ULTRA HD")
 }
}

func dfCreatePanel(parent uintptr) (*dfPanel,error) {
 p:=&dfPanel{parent:parent,options:dfReadOptions(),updates:make(chan string,1),clicks:make(chan int,4),stop:make(chan struct{})}
 inst,_,_:=procGetModuleHandleW.Call(0)
 cls:=utf16Ptr("DawnForgeTextStatusGraphicsV1")
 wc:=WNDCLASSEX{CbSize:uint32(unsafe.Sizeof(WNDCLASSEX{})),
  LpfnWndProc:syscall.NewCallback(dfWindowProc),
  HInstance:inst,LpszClassName:cls}
 if ok,_,e:=procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)));ok==0 {
  return nil,fmt.Errorf("register status panel: %v",e)
 }
 currentDF=p
 p.hwnd,_,_ = dfCreateWindow.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
  uintptr(unsafe.Pointer(cls)),uintptr(unsafe.Pointer(utf16Ptr("DawnForge"))),
  WS_POPUP,0,0,510,100,0,0,inst,0)
 if p.hwnd==0 {currentDF=nil;return nil,fmt.Errorf("create status panel failed")}
 p.status=dfChild(p.hwnd,"STATIC","PLAYERS ONLINE: --       UPTIME: --",12,10,490,23,0)
 _=dfChild(p.hwnd,"STATIC","GRAPHICS:",12,48,92,24,0)
 p.classic=dfChild(p.hwnd,"BUTTON","CLASSIC",108,43,170,32,101)
 p.ultra=dfChild(p.hwnd,"BUTTON","ULTRA HD",283,43,195,32,102)
 p.setSelected()
 p.playOverlay,_,_=dfCreateWindow.Call(WS_EX_LAYERED|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,uintptr(unsafe.Pointer(cls)),0,WS_POPUP,0,0,1,1,0,0,inst,0)
 if p.playOverlay!=0 {procSetLayeredWindowAttributes.Call(p.playOverlay,0,1,LWA_ALPHA);procShowWindow.Call(p.playOverlay,SW_HIDE)}
 procShowWindow.Call(p.hwnd,SW_HIDE)
 go p.poll()
 return p,nil
}

func dfPump() {
 var msg dfWinMsg
 for {
  ok,_,_:=dfPeekMessage.Call(uintptr(unsafe.Pointer(&msg)),0,0,0,dfPMRemove)
  if ok==0 {break}
  dfTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
  dfDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
 }
}

func (p *dfPanel) tick() {
 select {
 case event:=<-p.clicks:
  if event==103 {
   go func(){if err:=dfLaunchHD();err!=nil {dfSetText(p.status,"HD TEST ERROR: "+err.Error())}}()
  }
 default:
 }
 select {case s:= <-p.updates: dfSetText(p.status,s);default:}
 var r RECT
 if ok,_,_:=procGetWindowRect.Call(p.parent,uintptr(unsafe.Pointer(&r)));ok==0 {return}
 vis,_,_:=procIsWindowVisible.Call(p.parent)
 ico,_,_:=procIsIconic.Call(p.parent)
 fg,_,_:=procGetForegroundWindow.Call()
 // A click on the panel itself must not immediately hide it.
 if vis==0 || ico!=0 || (fg!=p.parent && fg!=p.hwnd) {
  procShowWindow.Call(p.hwnd,SW_HIDE)
  if p.playOverlay!=0 {procShowWindow.Call(p.playOverlay,SW_HIDE)}
  return
 }
 w:=int(r.Right-r.Left)
 h:=int(r.Bottom-r.Top)
 if w<=0 || h<=0 {return}
 x:=int(r.Left)+392*w/baseW
 y:=int(r.Top)+713*h/baseH
 pw:=510*w/baseW
 ph:=100*h/baseH
 procSetWindowPos.Call(p.hwnd,hwndTop,uintptr(x),uintptr(y),uintptr(pw),uintptr(ph),
  SWP_NOACTIVATE|SWP_SHOWWINDOW)
 procShowWindow.Call(p.hwnd,SW_SHOWNOACTIVATE)
 if p.playOverlay!=0 {
  if p.options.Mode=="hd" {
   play:=hits[len(hits)-1]
   px:=int(r.Left)+play.x1*w/baseW
   py:=int(r.Top)+play.y1*h/baseH
   pw:=(play.x2-play.x1)*w/baseW
   ph:=(play.y2-play.y1)*h/baseH
   procSetWindowPos.Call(p.playOverlay,hwndTop,uintptr(px),uintptr(py),uintptr(pw),uintptr(ph),SWP_NOACTIVATE|SWP_SHOWWINDOW)
   procShowWindow.Call(p.playOverlay,SW_SHOWNOACTIVATE)
  } else {procShowWindow.Call(p.playOverlay,SW_HIDE)}
 }
}

func (p *dfPanel) close() {
 close(p.stop)
 if p.playOverlay!=0 {procDestroyWindow.Call(p.playOverlay)}
 procDestroyWindow.Call(p.hwnd)
 currentDF=nil
}

func (p *dfPanel) poll() {
 client:=&http.Client{Timeout:6*time.Second}
 ticker:=time.NewTicker(30*time.Second)
 defer ticker.Stop()
 for {
  line:="PLAYERS ONLINE: --       UPTIME: --"
  req,err:=http.NewRequest(http.MethodGet,"https://dawnforge.eu/api/status",nil)
  if err==nil {
   resp,err:=client.Do(req)
   if err==nil {
    if resp.StatusCode==http.StatusOK {
     var raw map[string]json.RawMessage
     if json.NewDecoder(resp.Body).Decode(&raw)==nil {
      players,okp:=dfNumber(raw,"players","playersOnline","onlinePlayers")
      uptime,oku:=dfNumber(raw,"uptimeSeconds","worldUptimeSeconds")
      if okp || oku {
       pv,uv:="--","--"
       if okp && players>=0 {pv=fmt.Sprint(players)}
       if oku && uptime>=0 {uv=fmt.Sprintf("%dh %dm",uptime/3600,(uptime%3600)/60)}
       line=fmt.Sprintf("PLAYERS ONLINE: %s       UPTIME: %s",pv,uv)
      }
     }
    }
    resp.Body.Close()
   }
  }
  select {case p.updates<-line:default:}
  select {case <-p.stop:return;case <-ticker.C:}
 }
}

func dfNumber(raw map[string]json.RawMessage, keys ...string) (int64,bool) {
 for _,key:=range keys {
  data,ok:=raw[key]
  if !ok {continue}
  var value int64
  if json.Unmarshal(data,&value)==nil {return value,true}
 }
 return 0,false
}

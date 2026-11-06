package main

// Text-only, GDI-drawn addition to the existing DawnForge launcher.
// This file deliberately uses no stock Windows buttons or white STATIC controls.
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
    dfWMPaint = 0x000F
    dfWMErase = 0x0014
    dfWMLButtonUp = 0x0202
    dfPMRemove = 0x0001
    dfDTSingleLine = 0x0020
    dfDTVCenter = 0x0004
    dfDTCenter = 0x0001
    dfDTEndEllipsis = 0x8000
)
var (
    dfCreateWindow = user32.NewProc("CreateWindowExW")
    dfBeginPaint = user32.NewProc("BeginPaint")
    dfEndPaint = user32.NewProc("EndPaint")
    dfInvalidateRect = user32.NewProc("InvalidateRect")
    dfGetClientRect = user32.NewProc("GetClientRect")
    dfFillRect = user32.NewProc("FillRect")
    dfDrawText = user32.NewProc("DrawTextW")
    dfPeekMessage = user32.NewProc("PeekMessageW")
    dfTranslateMessage = user32.NewProc("TranslateMessage")
    dfDispatchMessage = user32.NewProc("DispatchMessageW")
    dfSetTextColor = gdi32.NewProc("SetTextColor")
    dfSetBkMode = gdi32.NewProc("SetBkMode")
    dfSelectObject = gdi32.NewProc("SelectObject")
    dfCreateFont = gdi32.NewProc("CreateFontW")
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
type dfPaintData struct {
    hdc uintptr
    erase int32
    rcPaint RECT
    restore int32
    incUpdate int32
    reserved [32]byte
}
type dfOptions struct { Mode string }
type dfStatus struct { players, uptime string }
type dfPanel struct {
    hwnd uintptr
    playOverlay uintptr
    parent uintptr
    options dfOptions
    updates chan dfStatus
    errors chan string
    stop chan struct{}
    players string
    uptime string
    message string
}
var currentDF *dfPanel

func dfWindowProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
    p := currentDF
    if p != nil && hwnd == p.hwnd {
        switch msg {
        case dfWMErase:
            return 1
        case dfWMPaint:
            var ps dfPaintData
            dc,_,_ := dfBeginPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps)))
            if dc != 0 { p.paint(dc); dfEndPaint.Call(hwnd,uintptr(unsafe.Pointer(&ps))) }
            return 0
        case dfWMLButtonUp:
            var rc RECT
            dfGetClientRect.Call(hwnd,uintptr(unsafe.Pointer(&rc)))
            w := int(rc.Right)
            h := int(rc.Bottom)
            if w > 0 && h > 0 {
                x := int(uint16(lp & 0xffff))*510/w
                y := int(uint16((lp >> 16) & 0xffff))*96/h
                if y >= 47 && y <= 91 {
                    if x >= 112 && x < 297 {p.options.Mode="classic"}
                    if x >= 299 && x <= 494 {p.options.Mode="hd"}
                    if err:=dfSaveOptions(p.options); err!=nil {p.message="Settings: "+err.Error()}
                    p.invalidate()
                }
            }
            return 0
        }
    }
    if p != nil && hwnd == p.playOverlay && msg == dfWMLButtonUp {
        go func(){
            if err:=dfLaunchHD(); err!=nil {
                select {case p.errors<-err.Error():default:}
            }
        }()
        return 0
    }
    r,_,_ := procDefWindowProcW.Call(hwnd,uintptr(msg),wp,lp)
    return r
}

func dfRect(dc uintptr,x,y,w,h int,color uintptr) {
    if w<=0 || h<=0 {return}
    r:=RECT{Left:int32(x),Top:int32(y),Right:int32(x+w),Bottom:int32(y+h)}
    brush,_,_:=procCreateSolidBrush.Call(color)
    if brush!=0 {
        dfFillRect.Call(dc,uintptr(unsafe.Pointer(&r)),brush)
        procDeleteObject.Call(brush)
    }
}
func dfText(dc uintptr,s string,x,y,w,h,size,weight int,color uintptr,center bool) {
    if s=="" || size<5 || w<1 || h<1 {return}
    font,_,_:=dfCreateFont.Call(uintptr(int32(-size)),0,0,0,uintptr(weight),
        0,0,0,1,0,0,5,0,uintptr(unsafe.Pointer(utf16Ptr("Georgia"))))
    if font==0 {return}
    old,_,_:=dfSelectObject.Call(dc,font)
    dfSetBkMode.Call(dc,1) // TRANSPARENT
    dfSetTextColor.Call(dc,color)
    r:=RECT{Left:int32(x),Top:int32(y),Right:int32(x+w),Bottom:int32(y+h)}
    flags:=uintptr(dfDTSingleLine|dfDTVCenter|dfDTEndEllipsis)
    if center {flags|=dfDTCenter}
    dfDrawText.Call(dc,uintptr(unsafe.Pointer(utf16Ptr(s))),^uintptr(0),
        uintptr(unsafe.Pointer(&r)),flags)
    dfSelectObject.Call(dc,old)
    procDeleteObject.Call(font)
}
func (p *dfPanel) paint(dc uintptr) {
    var rc RECT
    dfGetClientRect.Call(p.hwnd,uintptr(unsafe.Pointer(&rc)))
    w,h:=int(rc.Right),int(rc.Bottom)
    if w<1 || h<1 {return}
    sx:=func(n int)int{return n*w/510}
    sy:=func(n int)int{return n*h/96}
    sf:=func(n int)int {v:=n*h/96;if v<9 {return 9};return v}
    bg:=rgb(17,17,20)
    edge:=rgb(94,69,40)
    gold:=rgb(223,176,87)
    pale:=rgb(218,195,154)
    muted:=rgb(154,137,111)
    green:=rgb(81,212,115)

    // Flat dark stone strip, bronze inset, no stock white controls.
    dfRect(dc,0,0,w,h,bg)
    dfRect(dc,0,0,w,sy(2),edge)
    dfRect(dc,0,h-sy(2),w,sy(2),edge)
    dfRect(dc,0,0,sx(2),h,edge)
    dfRect(dc,w-sx(2),0,sx(2),h,edge)
    dfRect(dc,sx(14),sy(43),sx(482),sy(1),edge)

    online:="PLAYERS ONLINE"
    value:=p.players
    if value=="" {value="--"}
    up:=p.uptime
    if up=="" {up="--"}
    if p.message!="" {
        dfText(dc,p.message,sx(19),sy(7),sx(471),sy(32),sf(13),600,gold,false)
    } else {
        dfText(dc,online,sx(18),sy(8),sx(150),sy(28),sf(12),700,muted,false)
        dfText(dc,value,sx(173),sy(8),sx(70),sy(28),sf(17),700,green,false)
        dfText(dc,"UPTIME",sx(262),sy(8),sx(69),sy(28),sf(12),700,muted,false)
        dfText(dc,up,sx(336),sy(8),sx(154),sy(28),sf(14),700,pale,false)
    }

    dfText(dc,"GRAPHICS",sx(18),sy(55),sx(92),sy(27),sf(12),700,pale,false)
    dfModeButton:=func(x,bw int,label string,selected bool) {
        border:=edge
        fill:=rgb(36,33,32)
        ink:=muted
        if selected {
            border=gold
            fill=rgb(69,47,24)
            ink=gold
        }
        dfRect(dc,sx(x),sy(52),sx(bw),sy(35),border)
        dfRect(dc,sx(x+2),sy(54),sx(bw-4),sy(31),fill)
        if selected {
            dfRect(dc,sx(x+9),sy(84),sx(bw-18),sy(2),gold)
        }
        dfText(dc,label,sx(x+5),sy(54),sx(bw-10),sy(30),sf(14),700,ink,true)
    }
    dfModeButton(113,177,"CLASSIC",p.options.Mode!="hd")
    dfModeButton(298,197,"ULTRA HD",p.options.Mode=="hd")
}
func (p *dfPanel) invalidate() {
    if p!=nil && p.hwnd!=0 {dfInvalidateRect.Call(p.hwnd,0,0)}
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
    tmp:=path+".tmp"
    if err:=os.WriteFile(tmp,b,0600);err!=nil{return err}
    if err:=os.Rename(tmp,path);err!=nil {
        _=os.Remove(path)
        return os.Rename(tmp,path)
    }
    return nil
}
func dfCreatePanel(parent uintptr) (*dfPanel,error) {
    p:=&dfPanel{
        parent:parent,options:dfReadOptions(),players:"--",uptime:"--",
        updates:make(chan dfStatus,2),errors:make(chan string,2),stop:make(chan struct{}),
    }
    inst,_,_:=procGetModuleHandleW.Call(0)
    class:=utf16Ptr("DawnForgeStyledStatusPanelV2")
    wc:=WNDCLASSEX{CbSize:uint32(unsafe.Sizeof(WNDCLASSEX{})),
        LpfnWndProc:syscall.NewCallback(dfWindowProc),
        HInstance:inst,LpszClassName:class}
    if ok,_,e:=procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)));ok==0 {
        return nil,fmt.Errorf("register styled status panel: %v",e)
    }
    currentDF=p
    p.hwnd,_,_=dfCreateWindow.Call(WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
        uintptr(unsafe.Pointer(class)),0,WS_POPUP,0,0,510,96,0,0,inst,0)
    if p.hwnd==0 {currentDF=nil;return nil,fmt.Errorf("create styled status panel failed")}
    p.playOverlay,_,_=dfCreateWindow.Call(WS_EX_LAYERED|WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE,
        uintptr(unsafe.Pointer(class)),0,WS_POPUP,0,0,1,1,0,0,inst,0)
    if p.playOverlay!=0 {
        procSetLayeredWindowAttributes.Call(p.playOverlay,0,1,LWA_ALPHA)
        procShowWindow.Call(p.playOverlay,SW_HIDE)
    }
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
    case stat:=<-p.updates:
        p.players,p.uptime=stat.players,stat.uptime
        p.message=""
        p.invalidate()
    default:
    }
    select {
    case e:=<-p.errors:
        p.message="HD START ERROR: "+e
        p.invalidate()
    default:
    }
    var r RECT
    if ok,_,_:=procGetWindowRect.Call(p.parent,uintptr(unsafe.Pointer(&r)));ok==0 {return}
    vis,_,_:=procIsWindowVisible.Call(p.parent)
    ico,_,_:=procIsIconic.Call(p.parent)
    fg,_,_:=procGetForegroundWindow.Call()
    if vis==0 || ico!=0 || (fg!=p.parent && fg!=p.hwnd) {
        procShowWindow.Call(p.hwnd,SW_HIDE)
        if p.playOverlay!=0 {procShowWindow.Call(p.playOverlay,SW_HIDE)}
        return
    }
    w,h:=int(r.Right-r.Left),int(r.Bottom-r.Top)
    if w<=0 || h<=0{return}
    // Precisely the original center install/update information strip.
    x:=int(r.Left)+388*w/baseW
    y:=int(r.Top)+711*h/baseH
    pw:=510*w/baseW
    ph:=96*h/baseH
    procSetWindowPos.Call(p.hwnd,hwndTop,uintptr(x),uintptr(y),
        uintptr(pw),uintptr(ph),SWP_NOACTIVATE|SWP_SHOWWINDOW)
    procShowWindow.Call(p.hwnd,SW_SHOWNOACTIVATE)
    if p.playOverlay!=0 {
        if p.options.Mode=="hd" {
            play:=hits[len(hits)-1]
            px:=int(r.Left)+play.x1*w/baseW
            py:=int(r.Top)+play.y1*h/baseH
            pww:=(play.x2-play.x1)*w/baseW
            phh:=(play.y2-play.y1)*h/baseH
            procSetWindowPos.Call(p.playOverlay,hwndTop,
                uintptr(px),uintptr(py),uintptr(pww),uintptr(phh),
                SWP_NOACTIVATE|SWP_SHOWWINDOW)
            procShowWindow.Call(p.playOverlay,SW_SHOWNOACTIVATE)
        } else {
            procShowWindow.Call(p.playOverlay,SW_HIDE)
        }
    }
}
func (p *dfPanel) close() {
    close(p.stop)
    if p.playOverlay!=0 {procDestroyWindow.Call(p.playOverlay)}
    if p.hwnd!=0 {procDestroyWindow.Call(p.hwnd)}
    currentDF=nil
}
func (p *dfPanel) poll() {
    client:=&http.Client{Timeout:6*time.Second}
    ticker:=time.NewTicker(30*time.Second)
    defer ticker.Stop()
    for {
        online,up:="--","--"
        req,err:=http.NewRequest(http.MethodGet,"https://dawnforge.eu/api/status",nil)
        if err==nil {
            resp,err:=client.Do(req)
            if err==nil {
                if resp.StatusCode==http.StatusOK {
                    var raw map[string]json.RawMessage
                    if json.NewDecoder(resp.Body).Decode(&raw)==nil {
                        players,okp:=dfNumber(raw,"players","playersOnline","onlinePlayers")
                        uptime,oku:=dfNumber(raw,"uptimeSeconds","worldUptimeSeconds")
                        if okp && players>=0 {online=fmt.Sprint(players)}
                        if oku && uptime>=0 {up=fmt.Sprintf("%dh %02dm",uptime/3600,(uptime%3600)/60)}
                    }
                }
                resp.Body.Close()
            }
        }
        select {case p.updates<-dfStatus{players:online,uptime:up}:default:}
        select {case <-p.stop:return;case <-ticker.C:}
    }
}
func dfNumber(raw map[string]json.RawMessage,keys ...string) (int64,bool) {
    for _,key:=range keys {
        data,ok:=raw[key]
        if !ok {continue}
        var value int64
        if json.Unmarshal(data,&value)==nil {return value,true}
    }
    return 0,false
}

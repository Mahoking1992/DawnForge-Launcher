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
type dfStatus struct { realm, players, uptime string }
type dfPanel struct {
    hwnd uintptr
    playOverlay uintptr
    parent uintptr
    options dfOptions
    updates chan dfStatus
    errors chan string
    stop chan struct{}
    realm string
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
                // One horizontal top info row, no controls in UPDATE's strip.
                x := int(uint16(lp & 0xffff))*1140/w
                y := int(uint16((lp >> 16) & 0xffff))*90/h
                choice := ""
                if y >= 19 && y <= 73 {
                    if x >= 112 && x <= 264 { choice = "classic" }
                    if x >= 271 && x <= 428 { choice = "hd" }
                }
                if choice != "" {
                    p.options.Mode=choice
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
    // New one-row info bar ABOVE the original INSTALL / UPDATE / PLAY strip.
    // Only the previous GAME VERSION and REALM STATUS cards are covered.
    // The WEBSITE button and the original animated UPDATE progress remain
    // untouched in the original launcher.
    sx:=func(n int)int{return n*w/1140}
    sy:=func(n int)int{return n*h/90}
    sf:=func(n int)int{v:=n*h/90;if v<9{return 9};return v}
    edge:=rgb(101,72,37)
    gold:=rgb(230,182,90)
    pale:=rgb(220,201,166)
    muted:=rgb(169,149,116)
    green:=rgb(69,220,103)
    bg:=rgb(15,17,20)

    // Four dark, bronze-framed segments on the existing information row.
    dfRect(dc,0,0,w,h,edge)
    dfRect(dc,sx(3),sy(3),w-sx(6),h-sy(6),bg)
    for _,pos:=range []int{444,666,898} {
        dfRect(dc,sx(pos),sy(10),sx(2),sy(70),edge)
    }

    // Replaces the old GAME VERSION card at the left.
    dfText(dc,"GRAPHICS",sx(18),sy(20),sx(94),sy(44),sf(15),700,pale,false)
    modeButton:=func(x,bw int,label string,selected bool) {
        border,fill,ink:=edge,rgb(32,31,30),muted
        if selected {border,fill,ink=gold,rgb(72,48,26),gold}
        dfRect(dc,sx(x),sy(19),sx(bw),sy(54),border)
        dfRect(dc,sx(x+2),sy(21),sx(bw-4),sy(50),fill)
        if selected {dfRect(dc,sx(x+7),sy(68),sx(bw-14),sy(2),gold)}
        dfText(dc,label,sx(x+4),sy(23),sx(bw-8),sy(42),sf(15),700,ink,true)
    }
    modeButton(112,152,"CLASSIC",p.options.Mode!="hd")
    modeButton(271,157,"ULTRA HD",p.options.Mode=="hd")

    // Realm status, live players and uptime, all in this same horizontal row.
    realm:=p.realm
    if realm=="" {realm="--"}
    dfText(dc,"REALM STATUS",sx(454),sy(8),sx(202),sy(30),sf(15),700,pale,true)
    realmColor:=muted
    if realm=="ONLINE"{realmColor=green}
    dfText(dc,realm,sx(454),sy(36),sx(202),sy(44),sf(23),700,realmColor,true)

    if p.message!="" {
        dfText(dc,p.message,sx(678),sy(14),sx(442),sy(63),sf(14),700,gold,false)
    } else {
        players:=p.players
        if players==""{players="--"}
        uptime:=p.uptime
        if uptime==""{uptime="--"}
        dfText(dc,"PLAYERS ONLINE",sx(678),sy(8),sx(208),sy(30),sf(14),700,pale,true)
        dfText(dc,players,sx(678),sy(36),sx(208),sy(44),sf(23),700,green,true)
        dfText(dc,"UPTIME",sx(910),sy(8),sx(218),sy(30),sf(14),700,pale,true)
        dfText(dc,uptime,sx(910),sy(36),sx(218),sy(44),sf(20),700,pale,true)
    }
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
        uintptr(unsafe.Pointer(class)),0,WS_POPUP,0,0,1140,90,0,0,inst,0)
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
        p.realm,p.players,p.uptime=stat.realm,stat.players,stat.uptime
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
    // Reuse ONLY the upper information row (GAME VERSION + REALM STATUS).
    // Original update progress strip begins at y=711; it stays fully visible.
    // Leave the WEBSITE button at x>=1175 uncovered and clickable.
    x:=int(r.Left)+26*w/baseW
    y:=int(r.Top)+615*h/baseH
    pw:=1140*w/baseW
    ph:=90*h/baseH
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
        realm,online,up:="--","--","--"
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
                        // Live data is a stronger signal than a static label.
                        // Unknown is not mislabeled OFFLINE when API data is missing.
                        if okp && oku && uptime>=0 {realm="ONLINE"}
                    }
                }
                resp.Body.Close()
            }
        }
        select {case p.updates<-dfStatus{realm:realm,players:online,uptime:up}:default:}
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

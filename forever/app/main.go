//go:build windows

package main

import (
 "context"
 _ "embed"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "runtime"
 "strings"
 "sync"
 "time"
 "net/http"
 "io"
)

//go:embed artwork.jpg
var artwork []byte

const (
 launcherVersion="0.3.0"
 websiteURL="https://dawnforge.eu/forever"
 statusURL="https://dawnforge.eu/forever/api/status"
 updateManifestURL="https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/forever/update-manifest.json"
 clientManifestURL="https://dawnforge.eu/forever-client/manifest.json"
)
type settings struct {GameDir string}
type realmStatus struct {World *bool; Bnet *bool; Players *int}
var appFolder string
var cfg settings
var cfgLock sync.Mutex
var taskLock sync.Mutex
var taskBusy bool
var webClient=&http.Client{
 Timeout:40*time.Second,
 CheckRedirect:func(req *http.Request,via []*http.Request)error{
  if req.URL.Scheme!="https"||len(via)>5{return errors.New("Unsafe HTTP redirect")}
  return nil
 },
}
func gameDir()string{cfgLock.Lock();defer cfgLock.Unlock();return cfg.GameDir}
func normalizeDir(s string)string{
 s=filepath.Clean(s)
 if strings.EqualFold(filepath.Base(s),"_forever_"){
  if st,err:=os.Stat(filepath.Join(s,"Wow.exe"));err==nil&&!st.IsDir(){return filepath.Dir(s)}
 }
 return s
}
func chooseGameDir(path string)error{
 path=normalizeDir(path)
 st,err:=os.Stat(path);if err!=nil||!st.IsDir(){return errors.New("Folder not found")}
 cfgLock.Lock();defer cfgLock.Unlock()
 cfg.GameDir=path
 data,err:=json.MarshalIndent(cfg,"","  ");if err!=nil{return err}
 tmp:=filepath.Join(appFolder,"settings.json.tmp")
 if err=os.WriteFile(tmp,data,0600);err!=nil{return err}
 target:=filepath.Join(appFolder,"settings.json")
 _=os.Remove(target)
 return os.Rename(tmp,target)
}
func clientReady(dir string)bool{
 if dir==""{return false}
 if _,err:=os.Stat(filepath.Join(dir,".dawnforge-forever-download-pending"));err==nil{return false}
 if st,err:=os.Stat(filepath.Join(dir,"_forever_","Wow.exe"));err!=nil||st.IsDir(){return false}
 if st,err:=os.Stat(filepath.Join(dir,"Data"));err!=nil||!st.IsDir(){return false}
 return true
}
func playGame()error{
 dir:=gameDir()
 if !clientReady(dir){return errors.New("Forever client not found. Select the folder containing _forever_\\Wow.exe and Data.")}
 paths:=[]string{
  filepath.Join(appFolder,"DawnForgeForeverConnector.exe"),
  filepath.Join(dir,"DawnForgeForeverConnector.exe"),
 }
 for _,connector:=range paths{
  st,err:=os.Stat(connector)
  if err==nil&&!st.IsDir(){
   cmd:=exec.Command(connector)
   cmd.Dir=filepath.Dir(connector)
   if err:=cmd.Start();err!=nil{return err}
   setStatus("Compatible Forever connector started.")
   return nil
  }
 }
 setStatus("Game client found; realm connector is not configured.")
 return errors.New("Your client was detected, but the Forever private-realm connection helper is not configured. Starting Wow.exe alone would not connect to this private realm. PLAY cannot connect yet.")
}
func runTask(action func()error){
 taskLock.Lock()
 if taskBusy{taskLock.Unlock();return}
 taskBusy=true;taskLock.Unlock()
 go func(){
  defer func(){taskLock.Lock();taskBusy=false;taskLock.Unlock()}()
  if err:=action();err!=nil{setStatus("Action failed: "+err.Error());showWarning(err.Error())}
 }()
}
func refreshStatus(){
 ctx,cancel:=context.WithTimeout(context.Background(),12*time.Second);defer cancel()
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,statusURL,nil);if err!=nil{return}
 res,err:=webClient.Do(req);if err!=nil{setRealm("Live server status unavailable");return}
 defer res.Body.Close()
 if res.StatusCode!=200{setRealm("Live server status unavailable");return}
 var state realmStatus
 if err=json.NewDecoder(io.LimitReader(res.Body,1<<20)).Decode(&state);err!=nil{setRealm("Live server status unavailable");return}
 formatBool:=func(b *bool)string{if b==nil{return "UNKNOWN"};if *b{return "ONLINE"};return "OFFLINE"}
 p:="-";if state.Players!=nil{p=fmt.Sprint(*state.Players)}
 setRealm("World: "+formatBool(state.World)+"     Login: "+formatBool(state.Bnet)+"     Players: "+p)
}
func main(){
 runtime.LockOSThread()
 base:=os.Getenv("LOCALAPPDATA")
 if base==""{base=os.TempDir()}
 appFolder=filepath.Join(base,"DawnForgeForever","Launcher")
 if err:=os.MkdirAll(appFolder,0700);err!=nil{return}
 if data,err:=os.ReadFile(filepath.Join(appFolder,"settings.json"));err==nil{_ =json.Unmarshal(data,&cfg)}
 if cfg.GameDir!=""{cfg.GameDir=normalizeDir(cfg.GameDir)}
 startWindow()
}

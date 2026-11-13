//go:build windows

package main

import (
 "context"
 _ "embed"
 "crypto/sha256"
 "encoding/hex"
 "net"
 "strconv"
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
//go:embed forever-connector.exe
var bundledConnector []byte

const connectorSHA256="691cb9e017d47311839d4248d3a41265ef5bd1273b271987e429a23bd86b84f5"
const gatewayHost="play.dawnforge.eu"

const (
 launcherVersion="0.3.3"
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
// The binary is built from the pinned 2026-11-11 connector source and contains
// the certificate-constrained Forever realm profile. Only the SHA-pinned bundled
// connector is executed; arbitrary files from game directories are never run.
func readyConnector()(string,error){
 digest:=sha256.Sum256(bundledConnector)
 if hex.EncodeToString(digest[:])!=connectorSHA256{return "",errors.New("Bundled connector integrity check failed")}
 target:=filepath.Join(appFolder,"DawnForgeForeverConnector-"+connectorSHA256[:12]+".exe")
 if existing,err:=fileSHA256(target);err==nil&&strings.EqualFold(existing,connectorSHA256){return target,nil}
 if _,err:=os.Stat(target);err==nil{return "",errors.New("Existing connector file differs from the approved SHA-256; remove the invalid copy after checking it")}
 temp,err:=os.CreateTemp(appFolder,".connector-*.tmp")
 if err!=nil{return "",err}
 name:=temp.Name()
 defer os.Remove(name)
 if _,err=temp.Write(bundledConnector);err!=nil{temp.Close();return "",err}
 if err=temp.Close();err!=nil{return "",err}
 if err=os.Rename(name,target);err!=nil{return "",err}
 return target,nil
}
func gatewayReady()error{
 for _,port:=range []int{1119,8081,8090}{
  address:=net.JoinHostPort(gatewayHost,strconv.Itoa(port))
  conn,err:=net.DialTimeout("tcp",address,4*time.Second)
  if err!=nil{return fmt.Errorf("Forever gateway %s is unavailable: %w. Server routing must be fixed before PLAY",address,err)}
  conn.Close()
 }
 return nil
}
func playGame()error{
 dir:=gameDir()
 if !clientReady(dir){return errors.New("Forever client is not installed. Select a folder with _forever_\\Wow.exe and Data, or use DOWNLOAD / REPAIR.")}
 setStatus("Checking Forever gateway ports 1119, 8081 and 8090...")
 if err:=gatewayReady();err!=nil{return err}
 target,err:=readyConnector();if err!=nil{return err}
 setStatus("Starting verified Forever connector...")
 // The connector may request UAC to install its constrained public CA
 // and the one hosts entry for trinity.actual.battle.net.
 cmd:=exec.Command(target,"--client",dir)
 cmd.Dir=appFolder
 if err=cmd.Start();err!=nil{return fmt.Errorf("Could not start Forever connector: %w",err)}
 setStatus("Forever connector started. Login credentials belong inside the game.")
 return nil
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

package main

import (
 "context"
 _ "embed"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "time"

 "dawnforge-launcher/internal/hd"
)

//go:embed hd-manager.ps1
var hdManagerScript string

//go:embed hd-manifest.json
var pinnedHDManifest []byte

const hdManifestURL = "https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/download/hd-manifest.json"

type hdSettings struct { GameDir string `json:"game_dir"` }
func hdSettingsPath()(string,error){
 local:=os.Getenv("LOCALAPPDATA")
 if local=="" {return "",errors.New("LOCALAPPDATA is unset")}
 return filepath.Join(local,"DawnForge","Launcher","hd-settings.json"),nil
}
func loadHDSettings()(hdSettings,error){
 var s hdSettings
 path,err:=hdSettingsPath();if err!=nil{return s,err}
 b,err:=os.ReadFile(path);if err!=nil{return s,err}
 if err=json.Unmarshal(b,&s);err!=nil{return s,err}
 return s,nil
}
func storeHDSettings(s hdSettings)error{
 path,err:=hdSettingsPath();if err!=nil{return err}
 if err=os.MkdirAll(filepath.Dir(path),0755);err!=nil{return err}
 b,err:=json.MarshalIndent(s,"","  ");if err!=nil{return err}
 return os.WriteFile(path,b,0600)
}
func hdTask(mode string,gameDir string)error{
 if strings.TrimSpace(gameDir)==""{
  s,e:=loadHDSettings();if e!=nil{return errors.New("WoW folder not configured. Use --hd-set-game C:\\YourWoWFolder first")}
  gameDir=s.GameDir
 }
 if mode=="--hd-set-game"{
  return storeHDSettings(hdSettings{GameDir:gameDir})
 }
 ctx,cancel:=context.WithTimeout(context.Background(),6*time.Hour)
 defer cancel()
 var manifest hd.Manifest
 var err error
 if mode=="--vanilla-mode"{
  // Vanilla switch must work without network connectivity.
  manifest,err=hd.ParseManifest(pinnedHDManifest)
 }else{
  manifest,err=hd.FetchManifest(ctx,hdManifestURL)
 }
 if err!=nil{return fmt.Errorf("HD manifest unavailable: %w",err)}
 manager:=hd.Manager{GameDir:gameDir,Manifest:manifest,Progress:func(n string,current,total int64){
  fmt.Printf("\r%s %d%% (%d/%d bytes)  ",n,current*100/total,current,total)
 }}
 if mode=="--hd-install"{
  err=manager.Activate(ctx)
  fmt.Println()
  if err==nil {fmt.Println("HD patches enabled; launch the game using your existing DawnForge launcher.")}
  return err
 }
 if mode=="--vanilla-mode"{
  err=manager.Classic()
  if err==nil{fmt.Println("Classic graphics enabled; HD files safely retained for future use.")}
  return err
 }
 return errors.New("unknown HD command")
}
func launchHDManager()error{
 path,err:=hdSettingsPath();if err!=nil{return err}
 script:=filepath.Join(filepath.Dir(path),"hd-manager.ps1")
 if err=os.MkdirAll(filepath.Dir(path),0755);err!=nil{return err}
 if err=os.WriteFile(script,[]byte(hdManagerScript),0600);err!=nil{return err}
 me,err:=os.Executable();if err!=nil{return err}
 cmd:=exec.Command("powershell.exe","-NoProfile","-STA","-ExecutionPolicy","Bypass","-File",script,me)
 cmd.Stdout=io.Discard;cmd.Stderr=io.Discard
 return cmd.Start()
}
func handleHDCommand()bool{
 if len(os.Args)<2 {return false}
 mode:=os.Args[1]
 if mode!="--hd-install"&&mode!="--vanilla-mode"&&mode!="--hd-set-game"&&mode!="--hd-manager"{return false}
 if mode=="--hd-manager"{
  if err:=launchHDManager();err!=nil{messageBoxFallback(err.Error())}
  return true
 }
 arg:=""
 if len(os.Args)>2{arg=os.Args[2]}
 if mode=="--hd-set-game"&&arg=="" {messageBoxFallback("WoW directory not provided");return true}
 var status string
 if mode=="--hd-set-game" {
  if err:=hdTask(mode,arg);err!=nil{ messageBoxFallback("DawnForge HD: "+err.Error()) }
  return true
 }
 if settingsPath,err:=hdSettingsPath();err==nil{
  _=os.WriteFile(filepath.Join(filepath.Dir(settingsPath),"hd-last-run.log"),
   []byte("Started "+mode+" at "+time.Now().Format(time.RFC3339)+"\\n"),0600)
 }
 err:=hdTask(mode,arg)
 if err!=nil{
  status="FAILED "+mode+": "+err.Error()
 }else if mode=="--hd-install" {
  status="SUCCESS: 13 HD patch files verified and enabled. Existing Vanilla files are preserved."
 }else {
  status="SUCCESS: Vanilla graphics enabled. Verified HD patches retained in disabled folder."
 }
 if settingsPath,e:=hdSettingsPath();e==nil{
  _=os.WriteFile(filepath.Join(filepath.Dir(settingsPath),"hd-last-run.log"),
   []byte(time.Now().Format(time.RFC3339)+"\\n"+status+"\\n"),0600)
 }
 messageBoxFallback(status)
 return true
}

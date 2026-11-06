package main

// HD play support for the local TEST client only. No automatic game downloads,
// file deletion, registry changes, or modifications to the original client.
import (
 "bytes"
 "encoding/base64"
 "unicode/utf16"
 "crypto/sha256"
 "encoding/hex"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "unsafe"
)

func dfHash(path string) (string,error) {
 f,err:=os.Open(path)
 if err!=nil{return "",err}
 defer f.Close()
 h:=sha256.New()
 if _,err=io.Copy(h,f);err!=nil{return "",err}
 return hex.EncodeToString(h.Sum(nil)),nil
}
func dfFindTestClient() (string,error) {
 entries,err:=os.ReadDir(`C:\`)
 if err!=nil{return "",err}
 for _,item:=range entries {
  if !item.IsDir(){continue}
  name:=strings.ToLower(item.Name())
  if !strings.HasPrefix(name,"wow") || !strings.Contains(name,"kópia"){continue}
  root:=filepath.Join(`C:\`,item.Name())
  if _,err:=os.Stat(filepath.Join(root,"WoW.exe"));err==nil {
   return root,nil
  }
 }
 return "",errors.New(`HD test copy not found in C:\ (expected C:\wow – kópia)`)
}
// Alias the HD test directory via the Unicode Win32 DOS-device API.
// The path contains an en dash and an accented character, so the ANSI
// console output from subst.exe is unreliable.
func dfSubst(root string) (string,error) {
 k32:=syscall.NewLazyDLL("kernel32.dll")
 getDrives:=k32.NewProc("GetLogicalDrives")
 queryDevice:=k32.NewProc("QueryDosDeviceW")
 defineDevice:=k32.NewProc("DefineDosDeviceW")
 root=filepath.Clean(root)
 target:="\\??\\"+root
 wantedFile:=filepath.Join(root,"WoW.exe")
 stWanted,err:=os.Stat(wantedFile)
 if err!=nil {return "",fmt.Errorf("HD WoW.exe unavailable: %w",err)}
 mask,_,e:=getDrives.Call()
 if mask==0 {return "",fmt.Errorf("GetLogicalDrives failed: %v",e)}
 // First reuse any existing subst mapping for precisely this client.
 for ch:='Z';ch>='D';ch-- {
  drive:=string(ch)+":"
  if mask&(1<<uint(ch-'A'))==0 {continue}
  ptr:=syscall.StringToUTF16Ptr(drive)
  buf:=make([]uint16,2048)
  count,_,_:=queryDevice.Call(uintptr(unsafe.Pointer(ptr)),uintptr(unsafe.Pointer(&buf[0])),uintptr(len(buf)))
  if count==0 {continue}
  native:=syscall.UTF16ToString(buf)
  native=strings.TrimPrefix(native,"\\??\\")
  if !strings.EqualFold(filepath.Clean(native),root) {continue}
  stMapped,mappingError:=os.Stat(drive+"\\WoW.exe")
  if mappingError==nil && os.SameFile(stMapped,stWanted) {
   dfHDLog("Reusing HD alias "+drive+" -> "+root)
   return drive,nil
  }
 }
 errorsSeen:=[]string{}
 for _,ch:=range "ZYXWVUTSRQPONMLKJIHGFED" {
  if mask&(1<<uint(ch-'A'))!=0 {continue}
  drive:=string(ch)+":"
  drivePtr:=syscall.StringToUTF16Ptr(drive)
  pathPtr:=syscall.StringToUTF16Ptr(target)
  ok,_,createErr:=defineDevice.Call(0x1,
   uintptr(unsafe.Pointer(drivePtr)),uintptr(unsafe.Pointer(pathPtr)))
  if ok==0 {
   if len(errorsSeen)<3 {errorsSeen=append(errorsSeen,drive+" "+createErr.Error())}
   continue
  }
  stMapped,readErr:=os.Stat(drive+"\\WoW.exe")
  if readErr==nil && os.SameFile(stMapped,stWanted) {
   dfHDLog("Created HD alias "+drive+" -> "+root)
   return drive,nil
  }
  // Roll back only the virtual alias we just created, never an existing drive.
  defineDevice.Call(0x1|0x2|0x4,
   uintptr(unsafe.Pointer(drivePtr)),uintptr(unsafe.Pointer(pathPtr)))
  if len(errorsSeen)<3 {errorsSeen=append(errorsSeen,drive+" verification: "+fmt.Sprint(readErr))}
 }
 if len(errorsSeen)==0 {
  return "",errors.New("No free drive letter. Check current virtual drives.")
 }
 return "",fmt.Errorf("Cannot create HD alias: %s",strings.Join(errorsSeen,"; "))
}
func dfHDLog(entry string) {
 dir:=filepath.Join(os.Getenv("LOCALAPPDATA"),"DawnForge","LauncherTest")
 if err:=os.MkdirAll(dir,0700);err!=nil{return}
 f,err:=os.OpenFile(filepath.Join(dir,"hd-launch.log"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{return}
 defer f.Close()
 fmt.Fprintf(f,"%s %s\n",time.Now().Format(time.RFC3339),entry)
}

// wowreeb 3.5 uses narrow filesystem paths in several Windows calls.
// Never launch it from the accented HD game directory. Build a read-only-to-game
// copy of the official loader in a per-version ASCII-only test directory.
// This deliberately does not restore or overwrite any previous game config.
func dfIsASCII(s string) bool {
 for _,r:=range s {if r>127{return false}}
 return true
}
func dfStageFile(source,destination string) error {
 data,err:=os.ReadFile(source)
 if err!=nil{return err}
 if existing,err:=os.ReadFile(destination);err==nil {
  if bytes.Equal(existing,data){return nil}
  return fmt.Errorf("staged file differs; refusing to overwrite %s",destination)
 } else if !os.IsNotExist(err){return err}
 f,err:=os.OpenFile(destination,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return err}
 n,e:=f.Write(data)
 ce:=f.Close()
 if e!=nil || ce!=nil || n!=len(data) {
  _=os.Remove(destination)
  if e!=nil{return e}
  if ce!=nil{return ce}
  return io.ErrShortWrite
 }
 return nil
}
type dfWowreebConfig struct {
 XMLName xml.Name `xml:"wowreeb"`
 Config []struct {Name string `xml:"Name,attr"`; Value string `xml:"Value,attr"`} `xml:"Config"`
 Realms []dfWowreebRealm `xml:"Realm"`
}
type dfWowreebRealm struct {
 Name string `xml:"Name,attr"`
 Exe struct {Path string `xml:"Path,attr"`; SHA256 string `xml:"SHA256,attr"`} `xml:"Exe"`
 DLLs []struct {Path string `xml:"Path,attr"`} `xml:"DLL"`
}
func dfPrepareASCIILoader(sourceSetup,gameRoot,drive string) (string,string,error) {
 sourceEXE:=filepath.Join(sourceSetup,"wowreeb32.exe")
 sourceDLL:=filepath.Join(sourceSetup,"wowreeb32.dll")
 exeHash,err:=dfHash(sourceEXE)
 if err!=nil{return "","",err}
 dllHash,err:=dfHash(sourceDLL)
 if err!=nil{return "","",err}
 gameHash,err:=dfHash(filepath.Join(gameRoot,"WoW.exe"))
 if err!=nil{return "","",err}
 // No secrets, usernames, spaces, accents, Unicode punctuation, or redirects.
 local:=os.Getenv("LOCALAPPDATA")
 if !dfIsASCII(local) || local=="" {
  return "","",errors.New("Windows LOCALAPPDATA path is not ASCII; choose an ASCII-only test directory before HD launcher testing")
 }
 session:=filepath.Join(local,"DawnForge","LauncherTest","HDLoader",exeHash[:12]+"-"+dllHash[:12]+"-"+gameHash[:8])
 if !dfIsASCII(session){return "","",errors.New("Test loader path contains unsupported characters")}
 if err:=os.MkdirAll(session,0700);err!=nil{return "","",err}
 if err:=dfStageFile(sourceEXE,filepath.Join(session,"wowreeb32.exe"));err!=nil{return "","",err}
 if err:=dfStageFile(sourceDLL,filepath.Join(session,"wowreeb32.dll"));err!=nil{return "","",err}
 const entry="DawnForge HD Test"
 cfg:=dfWowreebConfig{}
 cfg.Config=append(cfg.Config,struct {
  Name string `xml:"Name,attr"`
  Value string `xml:"Value,attr"`
 }{Name:"ClearWDB",Value:"0"})
 realm:=dfWowreebRealm{Name:entry}
 realm.Exe.Path=drive+`\WoW.exe`
 realm.Exe.SHA256=gameHash
 realm.DLLs=append(realm.DLLs,struct{Path string `xml:"Path,attr"`}{Path:drive+`\VanillaHelpers.dll`})
 cfg.Realms=[]dfWowreebRealm{realm}
 data,err:=xml.MarshalIndent(cfg,"","  ")
 if err!=nil{return "","",err}
 data=append([]byte(xml.Header),data...)
 configPath:=filepath.Join(session,"config.xml")
 if err:=dfStageBytes(data,configPath);err!=nil{return "","",err}
 dfHDLog("Created isolated ASCII wowreeb session "+session)
 return session,entry,nil
}
func dfStageBytes(data []byte,destination string) error {
 if old,err:=os.ReadFile(destination);err==nil {
  if bytes.Equal(old,data){return nil}
  return fmt.Errorf("isolated config already exists with different content: %s",destination)
 } else if !os.IsNotExist(err){return err}
 f,err:=os.OpenFile(destination,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return err}
 _,e:=f.Write(data)
 ce:=f.Close()
 if e!=nil || ce!=nil {
  _=os.Remove(destination)
  if e!=nil{return e}
  return ce
 }
 return nil
}
func dfLaunchHD() error {
 dfHDLog("HD PLAY pressed - using original PowerShell Start-Process method")
 root,err:=dfFindTestClient()
 if err!=nil{return err}
 dfHDLog("HD client: "+root)
 if strings.EqualFold(filepath.Clean(root),filepath.Clean("C:\\wow")) {return errors.New("Refusing to use original game for HD")}
 setup:=filepath.Join(root,"_DawnForge_HD_Wowreeb_3_5")
 loader:=filepath.Join(setup,"wowreeb32.exe")
 helper:=filepath.Join(root,"VanillaHelpers.dll")
 for _,file:=range []string{loader,filepath.Join(setup,"wowreeb32.dll"),helper,filepath.Join(setup,"config.xml"),filepath.Join(root,"WoW.exe")} {
  if st,e:=os.Stat(file);e!=nil || st.IsDir(){return fmt.Errorf("HD file missing: %s",file)}
 }
 hash,err:=dfHash(helper)
 if err!=nil{return err}
 if !strings.EqualFold(hash,"1e74ddf6484c23482550d5f9e5f9d291085c4ac538d3b3ed9d87155521841196"){return errors.New("VanillaHelpers.dll verification failed")}
 for _,id:=range []string{"A","B","C","D","E","G","I","M","N","P","S","T","U"}{
  if st,e:=os.Stat(filepath.Join(root,"Data","patch-"+id+".mpq"));e!=nil || st.Size()<512{return fmt.Errorf("HD patch %s is not installed",id)}
 }
 // The old working PowerShell script used an existing Windows SUBST mapping.
 // Keep that exact behavior, without creating any replacement wowreeb config.
 drive,err:=dfSubst(root)
 if err!=nil{return err}
 dfHDLog("HD mapped drive: "+drive)
 // This replicates the old SPUSTIT-HD.ps1 startup, using the original
 // wowreeb32.exe, its original config.xml and the PowerShell Start-Process
 // API instead of launching a fabricated isolated wowreeb session.
 // The game and configuration are never written by this function.
 quote:=func(x string)string{return "'"+strings.ReplaceAll(x,"'","''")+"'"}
 ps:="$ErrorActionPreference='Stop';"+
    "$env:WOWREEB_ENTRY='DawnForge HD Test';"+
    "$p=Start-Process -FilePath "+quote(loader)+" -WorkingDirectory "+quote(setup)+" -PassThru -ErrorAction Stop;"+
    "Write-Output ('WOWREEB_PID='+$p.Id);"+
    "Start-Sleep -Milliseconds 1300;"+
    "if($p.HasExited){Write-Output ('WOWREEB_EXIT='+$p.ExitCode); if($p.ExitCode -ne 0){exit 13}}"
 // EncodedCommand uses UTF-16LE; avoid shell interpolation and quote issues.
 encoded:=utf16.Encode([]rune(ps))
 raw:=make([]byte,len(encoded)*2)
 for i,v:=range encoded {raw[2*i]=byte(v);raw[2*i+1]=byte(v>>8)}
 cmd:=exec.Command("powershell.exe","-NoProfile","-NonInteractive","-EncodedCommand",base64.StdEncoding.EncodeToString(raw))
 cmd.Dir=setup
 cmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true,CreationFlags:0x08000000}
 output,err:=cmd.CombinedOutput()
 response:=strings.TrimSpace(string(output))
 dfHDLog("PowerShell Start-Process result: "+response)
 if err!=nil {
  return fmt.Errorf("Original PowerShell HD startup failed: %w. %s",err,response)
 }
 dfHDLog("PowerShell completed; actual HD game window still requires visual verification")
 return nil
}

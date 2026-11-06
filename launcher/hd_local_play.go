package main

// HD play support for the local TEST client only. No automatic game downloads,
// file deletion, registry changes, or modifications to the original client.
import (
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

type dfGameConfig struct {
 XMLName xml.Name `xml:"wowreeb"`
 Realms []struct {
  Name string `xml:"Name,attr"`
  Exe struct {Path string `xml:"Path,attr"`} `xml:"Exe"`
  DLLs []struct {Path string `xml:"Path,attr"`} `xml:"DLL"`
  AuthServer struct {Host string `xml:"Host,attr"`} `xml:"AuthServer"`
 } `xml:"Realm"`
}

func dfLocalHDConfig(setup,root string) (string,error) {
 configPath:=filepath.Join(setup,"config.xml")
 raw,err:=os.ReadFile(configPath)
 if err!=nil{return "",fmt.Errorf("Cannot read wowreeb config: %w",err)}
 var parsed dfGameConfig
 if err:=xml.Unmarshal(raw,&parsed);err!=nil{return "",fmt.Errorf("Invalid wowreeb config.xml: %w",err)}
 if parsed.XMLName.Local!="wowreeb" || len(parsed.Realms)==0 {
  return "",errors.New("wowreeb config.xml has no valid Realm entries")
 }
 gameFile,err:=os.Stat(filepath.Join(root,"WoW.exe"))
 if err!=nil{return "",err}
 for _,realm:=range parsed.Realms {
  if realm.Name=="" || realm.Exe.Path=="" {continue}
  configured,err:=os.Stat(realm.Exe.Path)
  if err!=nil || !os.SameFile(configured,gameFile) {continue}
  // Never use the incomplete config generated by the earlier broken test
  // launcher: it omitted AuthServer and caused immediate wowreeb crashes.
  if realm.AuthServer.Host=="" {
   return "",fmt.Errorf("wowreeb entry %q has no AuthServer; the test-generated configuration is incomplete. Restore the previously working wowreeb config.xml before testing",realm.Name)
  }
  helperFound:=false
  for _,dll:=range realm.DLLs {
   if strings.EqualFold(filepath.Base(strings.ReplaceAll(dll.Path,"\\","/")),"VanillaHelpers.dll") {
    dllStat,err:=os.Stat(dll.Path)
    expected,err2:=os.Stat(filepath.Join(root,"VanillaHelpers.dll"))
    if err==nil && err2==nil && os.SameFile(dllStat,expected) {helperFound=true;break}
   }
  }
  if !helperFound {
   return "",fmt.Errorf("wowreeb entry %q does not load the verified HD VanillaHelpers.dll. Restore the working config instead of regenerating it",realm.Name)
  }
  return realm.Name,nil
 }
 return "",errors.New("No wowreeb Realm points to the HD test client's WoW.exe. The original config was not modified")
}
func dfLaunchHD() error {
 dfHDLog("HD PLAY pressed")
 root,err:=dfFindTestClient()
 if err!=nil{dfHDLog("HD test folder: "+err.Error());return err}
 dfHDLog("HD test client: "+root)
 if strings.EqualFold(filepath.Clean(root),filepath.Clean("C:\\wow")) {
  return errors.New("Refusing to change or launch the original C:\\wow as HD test")
 }
 setup:=filepath.Join(root,"_DawnForge_HD_Wowreeb_3_5")
 loader:=filepath.Join(setup,"wowreeb32.exe")
 dll:=filepath.Join(setup,"wowreeb32.dll")
 helper:=filepath.Join(root,"VanillaHelpers.dll")
 for _,f:=range []string{loader,dll,helper,filepath.Join(root,"WoW.exe")} {
  if st,e:=os.Stat(f);e!=nil || st.IsDir() {
   return fmt.Errorf("Required HD test file missing: %s",f)
  }
 }
 h,err:=dfHash(helper)
 if err!=nil{return err}
 const trusted="1e74ddf6484c23482550d5f9e5f9d291085c4ac538d3b3ed9d87155521841196"
 if !strings.EqualFold(h,trusted) {
  return errors.New("VanillaHelpers.dll hash mismatch; refusing to load it")
 }
 // The full HD profile must already be installed. Never download or move MPQs
 // during PLAY and never touch the production client's Data folder.
 for _,id:=range []string{"A","B","C","D","E","G","I","M","N","P","S","T","U"} {
  mpq:=filepath.Join(root,"Data","patch-"+id+".mpq")
  if st,e:=os.Stat(mpq);e!=nil || st.Size()<512 {
   return fmt.Errorf("HD patch %s is not active in the TEST copy",id)
  }
 }
 drive,err:=dfSubst(root)
 if err!=nil {dfHDLog("Virtual drive: "+err.Error());return err}
 dfHDLog("HD virtual drive: "+drive)
 entry,err:=dfLocalHDConfig(setup,root)
 if err!=nil{dfHDLog("Preflight: "+err.Error());return err}
 dfHDLog("Using original, unmodified wowreeb config and entry: "+entry)
 cmd:=exec.Command(loader)
 cmd.Dir=setup
 cmd.Env=append(os.Environ(),"WOWREEB_ENTRY="+entry)
 cmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true}
 if err:=cmd.Start();err!=nil {dfHDLog("Start failed: "+err.Error());return err}
 dfHDLog(fmt.Sprintf("wowreeb started PID %d",cmd.Process.Pid))
 err=cmd.Wait()
 if err!=nil {
  dfHDLog("wowreeb terminated with error: "+err.Error())
  return fmt.Errorf("wowreeb crashed: %w. View Windows Event Viewer, Application log, event 1000 for fault module",err)
 }
 dfHDLog("wowreeb process returned success")
 return nil
}

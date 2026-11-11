package main

import (
 _ "embed"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "unsafe"
)

//go:embed original-setup.exe
var embeddedInstaller []byte

var user32=syscall.NewLazyDLL("user32.dll")
var messageBox=user32.NewProc("MessageBoxW")
func alert(s string,flags uintptr) {
 msg,_:=syscall.UTF16PtrFromString(s)
 title,_:=syscall.UTF16PtrFromString("DawnForge Vanilla")
 messageBox.Call(0,uintptr(unsafe.Pointer(msg)),uintptr(unsafe.Pointer(title)),flags)
}
func chooseFolder()(string,error){
 script:="Add-Type -AssemblyName System.Windows.Forms; $d=New-Object System.Windows.Forms.FolderBrowserDialog; $d.Description='Choose the DawnForge Vanilla game folder. Select the SAME destination in the installer.'; $d.ShowNewFolderButton=$true; if($d.ShowDialog() -eq 'OK') { [Console]::Write($d.SelectedPath) }"
 b,err:=exec.Command("powershell.exe","-NoProfile","-STA","-Command",script).Output()
 return strings.TrimSpace(string(b)),err
}
func repair(dir string)(bool,error){
 if st,e:=os.Stat(filepath.Join(dir,"Wow.exe"));e!=nil||st.IsDir(){return false,nil}
 target:=filepath.Join(dir,"realmlist.wtf")
 b,e:=os.ReadFile(target)
 if os.IsNotExist(e){return false,nil}
 if e!=nil{return false,e}
 if strings.TrimSpace(string(b))=="set realmlist play.dawnforge.eu"{return true,nil}
 backup:=target+".dawnforge-backup"
 if _,e=os.Stat(backup);os.IsNotExist(e){if e=os.WriteFile(backup,b,0644);e!=nil{return false,e}}
 tmp:=target+".dawnforge-tmp"
 if e=os.WriteFile(tmp,[]byte("set realmlist play.dawnforge.eu\r\n"),0644);e!=nil{return false,e}
 if e=os.Rename(tmp,target);e!=nil{_ =os.Remove(tmp);return false,e}
 return true,nil
}
func run()error{
 gameDir:=os.Getenv("DAWNFORGE_WRAPPER_TEST_DIR")
 test:=gameDir!=""
 if !test {var e error;gameDir,e=chooseFolder();if e!=nil{return e};if gameDir==""{return nil}}
 gameDir,e:=filepath.Abs(gameDir);if e!=nil{return e}
 if !test {alert("Install the game into this SAME folder in the original installer:\r\n"+gameDir+"\r\n\r\nRealmlist will be corrected automatically.",0x40)}
 exe:=os.Getenv("DAWNFORGE_WRAPPER_TEST_INSTALLER")
 if !test{
  base:=os.Getenv("LOCALAPPDATA")
  if base==""{return errors.New("LOCALAPPDATA unavailable")}
  exe=filepath.Join(base,"DawnForge","VanillaInstaller","DawnForgeOriginalSetup.exe")
  if e=os.MkdirAll(filepath.Dir(exe),0755);e!=nil{return e}
  if e=os.WriteFile(exe,embeddedInstaller,0755);e!=nil{return e}
 }
 if exe==""{return errors.New("Launcher executable missing")}
 proc:=exec.Command(exe)
 proc.Dir=filepath.Dir(exe)
 if e=proc.Start();e!=nil{return fmt.Errorf("Cannot start original Vanilla installer: %w",e)}
 done:=make(chan struct{})
 go func(){_ =proc.Wait();close(done)}()
 ticker:=time.NewTicker(200*time.Millisecond);defer ticker.Stop()
 limit:=time.NewTimer(12*time.Hour);defer limit.Stop()
 verified:=false
 for{
  select{
  case <-ticker.C:
   ok,err:=repair(gameDir);if err!=nil{return err};if ok{verified=true}
  case <-done:
   for i:=0;i<15;i++{ok,err:=repair(gameDir);if err!=nil{return err};if ok{verified=true};time.Sleep(200*time.Millisecond)}
   if !verified{
    if test{return errors.New("test failed: client folder not repaired")}
    alert("The selected game folder was not found:\r\n"+gameDir+"\r\nUse the same destination in the installer, then run DawnForge again.",0x30)
   }
   return nil
  case <-limit.C:
   return errors.New("Installer monitoring timed out")
  }
 }
}
func main(){if e:=run();e!=nil{alert(e.Error(),0x10);os.Exit(1)}}

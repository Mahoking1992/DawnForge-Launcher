package main

import (
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "path/filepath"
 "strings"
 "syscall"
 "unsafe"
)

var user32=syscall.NewLazyDLL("user32.dll")
var msgBox=user32.NewProc("MessageBoxW")
func notify(message string) {p,_:=syscall.UTF16PtrFromString(message);title,_:=syscall.UTF16PtrFromString("DawnForge Vanilla - Realmlist Fix");msgBox.Call(0,uintptr(unsafe.Pointer(p)),uintptr(unsafe.Pointer(title)),0x40)}
func fail(err error){p,_:=syscall.UTF16PtrFromString(err.Error());title,_:=syscall.UTF16PtrFromString("DawnForge Vanilla - Realmlist Fix");msgBox.Call(0,uintptr(unsafe.Pointer(p)),uintptr(unsafe.Pointer(title)),0x10)}
func fix(folder string) error {
 folder,err:=filepath.Abs(folder);if err!=nil{return err}
 wow:=filepath.Join(folder,"Wow.exe")
 st,err:=os.Stat(wow);if err!=nil || st.IsDir(){return errors.New("Selected folder does not contain Wow.exe. No changes were made.")}
 target:=filepath.Join(folder,"realmlist.wtf")
 old,err:=os.ReadFile(target);if err!=nil{return fmt.Errorf("realmlist.wtf not found next to Wow.exe: %w",err)}
 if strings.TrimSpace(string(old))=="set realmlist play.dawnforge.eu" {return nil}
 if err:=os.WriteFile(target+".dawnforge-backup",old,0644);err!=nil{return err}
 tmp:=target+".dawnforge-tmp"
 if err:=os.WriteFile(tmp,[]byte("set realmlist play.dawnforge.eu\r\n"),0644);err!=nil{return err}
 if err:=os.Rename(tmp,target);err!=nil {os.Remove(tmp);return err}
 return nil
}
func main(){
 args:=os.Args[1:]
 if len(args)!=1 { fail(errors.New("Provide the installed Vanilla client folder as the only argument (the folder containing Wow.exe and realmlist.wtf). No changes were made."));return}
 folder:=args[0]
 if err:=fix(folder);err!=nil{fail(err);return}
 notify("Vanilla realmlist is now play.dawnforge.eu. Original file was backed up.")
 _=json.Valid
}

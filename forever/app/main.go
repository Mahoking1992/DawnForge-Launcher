//go:build windows

package main

import (
    _ "embed"
    "crypto/sha256"
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "syscall"
)

//go:embed launcher.ps1
var script []byte
//go:embed artwork.jpg
var art []byte

func main() {
    base := os.Getenv("LOCALAPPDATA")
    if base == "" {return}
    folder := filepath.Join(base,"DawnForgeForever","Launcher")
    if err:=os.MkdirAll(folder,0700);err!=nil{return}
    files := map[string][]byte{"launcher.ps1":script,"artwork.jpg":art}
    for name,data := range files {
        dest:=filepath.Join(folder,name)
        old,err:=os.ReadFile(dest)
        if err==nil && sha256.Sum256(old)==sha256.Sum256(data){continue}
        if err:=os.WriteFile(dest,data,0600);err!=nil{return}
    }
    exe,_:=os.Executable()
    cmd:=exec.Command("powershell.exe","-NoProfile","-STA","-ExecutionPolicy","Bypass","-WindowStyle","Hidden","-File",filepath.Join(folder,"launcher.ps1"),"-AppDirectory",folder,"-LauncherExe",exe)
    cmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true}
    if out,err:=cmd.CombinedOutput();err!=nil{
        _=os.WriteFile(filepath.Join(folder,"error.log"),[]byte(fmt.Sprintf("%v\n%s",err,out)),0600)
    }
}

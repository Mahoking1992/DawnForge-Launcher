package main

// HD play support for the local TEST client only. No automatic game downloads,
// file deletion, registry changes, or modifications to the original client.
import (
 "bytes"
 "crypto/sha256"
 "encoding/hex"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "regexp"
 "strings"
 "sort"
 "time"
 "syscall"
)

type dfHDXML struct {
 XMLName xml.Name `xml:"wowreeb"`
 Config []dfHDConfig `xml:"Config"`
 Realm []dfHDRealm `xml:"Realm"`
}
type dfHDConfig struct {
 Name string `xml:"Name,attr"`
 Value string `xml:"Value,attr"`
}
type dfHDRealm struct {
 Name string `xml:"Name,attr"`
 Exe dfHDExe `xml:"Exe"`
 DLL dfHDDLL `xml:"DLL"`
}
type dfHDExe struct {
 Path string `xml:"Path,attr"`
 SHA256 string `xml:"SHA256,attr"`
}
type dfHDDLL struct {Path string `xml:"Path,attr"`}

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
func dfSubst(root string) (string,error) {
 listCmd:=exec.Command("subst.exe")
 listCmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true,CreationFlags:0x08000000}
 content,_:=listCmd.CombinedOutput()
 re:=regexp.MustCompile(`(?m)^([A-Za-z]):\\: => (.*?)\r?$`)
 for _,match:=range re.FindAllStringSubmatch(string(content),-1) {
  if strings.EqualFold(strings.TrimRight(match[2],`\`),strings.TrimRight(root,`\`)) {
   return strings.ToUpper(match[1])+":",nil
  }
 }
 for _,letter:=range "ZYXWVUTSR" {
  drive:=string(letter)+":"
  if _,err:=os.Stat(drive+`\`);err==nil {continue}
  if err:=func()error{cmd:=exec.Command("subst.exe",drive,root);cmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true,CreationFlags:0x08000000};return cmd.Run()}();err==nil{return drive,nil}
 }
 return "",errors.New("No available Windows drive letter for HD test")
}
func dfHDLog(entry string) {
 dir:=filepath.Join(os.Getenv("LOCALAPPDATA"),"DawnForge","LauncherTest")
 if err:=os.MkdirAll(dir,0700);err!=nil{return}
 f,err:=os.OpenFile(filepath.Join(dir,"hd-launch.log"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{return}
 defer f.Close()
 fmt.Fprintf(f,"%s %s\n",time.Now().Format(time.RFC3339),entry)
}

// Reuse the verified local wowreeb config instead of overwriting it.
// The earliest pre-test backup is preferred because previous test builds
// could have replaced the original working config.
func dfExistingConfig(setup,drive string) ([]byte,string,string,error) {
 names:=[]string{}
 entries,err:=os.ReadDir(setup)
 if err!=nil{return nil,"","",err}
 for _,entry:=range entries {
  name:=entry.Name()
  if strings.HasPrefix(name,"config-before-launcher-test-") && strings.HasSuffix(strings.ToLower(name),".xml") {
   names=append(names,filepath.Join(setup,name))
  }
 }
 sort.Strings(names)
 names=append(names,filepath.Join(setup,"config.xml"))
 for _,file:=range names {
  raw,err:=os.ReadFile(file)
  if err!=nil {continue}
  config,entry,err:=dfRebaseConfig(raw,drive)
  if err==nil && entry!="" {return config,file,entry,nil}
 }
 return nil,"","",errors.New("No valid existing wowreeb config or backup found. The original HD config is untouched.")
}

// Token-preserving XML rewrite changes only the two paths that depend
// on the local ASCII subst drive, and keeps all other verified settings.
func dfRebaseConfig(raw []byte,drive string) ([]byte,string,error) {
 dec:=xml.NewDecoder(bytes.NewReader(raw))
 var buffer bytes.Buffer
 enc:=xml.NewEncoder(&buffer)
 entry:=""
 realmCount:=0
 exeCount:=0
 for {
  token,err:=dec.Token()
  if err==io.EOF {break}
  if err!=nil{return nil,"",err}
  if el,ok:=token.(xml.StartElement);ok {
   switch el.Name.Local {
   case "Realm":
    realmCount++
    for _,a:=range el.Attr {if a.Name.Local=="Name" && entry=="" {entry=a.Value}}
   case "Exe":
    exeCount++
    for i,a:=range el.Attr {if a.Name.Local=="Path" {el.Attr[i].Value=drive+string([]byte{92})+"WoW.exe"}}
   case "DLL":
    for i,a:=range el.Attr {
     if a.Name.Local=="Path" && strings.EqualFold(filepath.Base(strings.ReplaceAll(a.Value,string([]byte{92}),"/")),"VanillaHelpers.dll") {
      el.Attr[i].Value=drive+string([]byte{92})+"VanillaHelpers.dll"
     }
    }
   }
   token=el
  }
  if err:=enc.EncodeToken(token);err!=nil{return nil,"",err}
 }
 if err:=enc.Flush();err!=nil{return nil,"",err}
 if realmCount==0||exeCount==0||entry=="" {return nil,"",errors.New("Missing wowreeb Realm or Exe Path")}
 return buffer.Bytes(),entry,nil
}
func dfLaunchHD() error {
 dfHDLog("HD PLAY pressed")
 root,err:=dfFindTestClient()
 if err!=nil{dfHDLog("No test client: "+err.Error());return err}
 dfHDLog("HD test client: "+root)
 if strings.EqualFold(filepath.Clean(root),filepath.Clean(`C:\wow`)) {
  return errors.New(`HD test refuses to alter original C:\wow`)
 }
 setup:=filepath.Join(root,"_DawnForge_HD_Wowreeb_3_5")
 loader:=filepath.Join(setup,"wowreeb32.exe")
 dll:=filepath.Join(setup,"wowreeb32.dll")
 helper:=filepath.Join(root,"VanillaHelpers.dll")
 exe:=filepath.Join(root,"WoW.exe")
 for _,file:=range []string{loader,dll,helper,exe} {
  if st,e:=os.Stat(file);e!=nil || st.IsDir() {
   return fmt.Errorf("HD test dependency missing: %s",file)
  }
 }
 hash,err:=dfHash(helper)
 if err!=nil{return err}
 const knownHelper="1e74ddf6484c23482550d5f9e5f9d291085c4ac538d3b3ed9d87155521841196"
 if !strings.EqualFold(hash,knownHelper) {
  return errors.New("VanillaHelpers.dll SHA-256 does not match the official v1.1.2 release")
 }
 for _,id:=range []string{"A","B","C","D","E","G","I","M","N","P","S","T","U"} {
  name:=filepath.Join(root,"Data","patch-"+id+".mpq")
  if st,e:=os.Stat(name);e!=nil || st.Size()<512 {
   return fmt.Errorf("HD module %s is not installed: %s",id,name)
  }
 }
 drive,err:=dfSubst(root)
 if err!=nil{dfHDLog("Subst error: "+err.Error());return err}
 dfHDLog("Subst drive: "+drive)
 // Preserve the working wowreeb configuration. The old launcher broke HD
 // by replacing it with a freshly generated minimal config.xml.
 config,source,entry,err:=dfExistingConfig(setup,drive)
 if err!=nil{dfHDLog("Config selection: "+err.Error());return err}
 dfHDLog("Using existing config: "+source+", entry: "+entry)
 session:=filepath.Join(os.Getenv("LOCALAPPDATA"),"DawnForge","LauncherTest","HDSession")
 if err:=os.MkdirAll(session,0700);err!=nil{return err}
 if err:=os.WriteFile(filepath.Join(session,"config.xml"),config,0600);err!=nil{return err}
 dfHDLog("Private HD session config ready; the existing game config was not modified")
 cmd:=exec.Command(loader)
 cmd.Dir=session
 cmd.Env=append(os.Environ(),"WOWREEB_ENTRY="+entry)
 cmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true}
 if err:=cmd.Start();err!=nil{dfHDLog("Wowreeb launch: "+err.Error());return err}
 dfHDLog(fmt.Sprintf("Wowreeb started, PID %d",cmd.Process.Pid))
 go func(){if err:=cmd.Wait();err!=nil{dfHDLog("Wowreeb exit error: "+err.Error())}else{dfHDLog("Wowreeb exited normally")}}()
 return nil
}

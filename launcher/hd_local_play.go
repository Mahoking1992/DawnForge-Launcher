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
 "regexp"
 "strings"
 "time"
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
 entries,err:=os.ReadDir("C:\")
 if err!=nil{return "",err}
 for _,item:=range entries {
  if !item.IsDir(){continue}
  name:=strings.ToLower(item.Name())
  if !strings.HasPrefix(name,"wow") || !strings.Contains(name,"kópia"){continue}
  root:=filepath.Join("C:\",item.Name())
  if _,err:=os.Stat(filepath.Join(root,"WoW.exe"));err==nil {
   return root,nil
  }
 }
 return "",errors.New("HD test copy not found in C:\ (expected C:\wow – kópia)")
}
func dfSubst(root string) (string,error) {
 content,_:=exec.Command("subst.exe").CombinedOutput()
 re:=regexp.MustCompile(`(?m)^([A-Za-z]):\: => (.*?)?$`)
 for _,match:=range re.FindAllStringSubmatch(string(content),-1) {
  if strings.EqualFold(strings.TrimRight(match[2],"\"),strings.TrimRight(root,"\")) {
   return strings.ToUpper(match[1])+":",nil
  }
 }
 for _,letter:=range "ZYXWVUTSR" {
  drive:=string(letter)+":"
  if _,err:=os.Stat(drive+"\");err==nil {continue}
  if err:=exec.Command("subst.exe",drive,root).Run();err==nil{return drive,nil}
 }
 return "",errors.New("No available Windows drive letter for HD test")
}
func dfLaunchHD() error {
 root,err:=dfFindTestClient()
 if err!=nil{return err}
 if strings.EqualFold(filepath.Clean(root),filepath.Clean("C:\wow")) {
  return errors.New("HD test refuses to alter original C:\wow")
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
 if err!=nil{return err}
 exeHash,err:=dfHash(exe)
 if err!=nil{return err}
 x:=dfHDXML{
  Config:[]dfHDConfig{{Name:"ClearWDB",Value:"0"}},
  Realm:[]dfHDRealm{{
   Name:"DawnForge HD Test",
   Exe:dfHDExe{Path:drive+"\WoW.exe",SHA256:exeHash},
   DLL:dfHDDLL{Path:drive+"\VanillaHelpers.dll"},
  }},
 }
 data,err:=xml.MarshalIndent(x,"","  ")
 if err!=nil{return err}
 data=append([]byte(xml.Header),data...)
 config:=filepath.Join(setup,"config.xml")
 if old,e:=os.ReadFile(config);e==nil && string(old)!=string(data) {
  backup:=filepath.Join(setup,fmt.Sprintf("config-before-launcher-test-%d.xml",time.Now().UnixNano()))
  if err:=os.WriteFile(backup,old,0600);err!=nil{return err}
 }
 if err:=os.WriteFile(config,data,0600);err!=nil{return err}
 cmd:=exec.Command(loader)
 cmd.Dir=setup
 cmd.Env=append(os.Environ(),"WOWREEB_ENTRY=DawnForge HD Test")
 if err:=cmd.Start();err!=nil{return err}
 return cmd.Process.Release()
}

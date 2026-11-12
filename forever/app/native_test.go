//go:build windows

package main

import (
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestVersionCompare(t *testing.T){
 if compareVersions("0.3.0","0.2.4")<=0{t.Fatal("new version not newer")}
 if compareVersions("0.3.0","0.3.0")!=0{t.Fatal("same version differs")}
 if compareVersions("0.2.4","0.3.0")>=0{t.Fatal("old version not older")}
}
func TestSafeFileTarget(t *testing.T){
 root:=t.TempDir()
 good,err:=safeFileTarget(root,"Data/example.mpq")
 if err!=nil||!strings.HasPrefix(strings.ToLower(good),strings.ToLower(root)){t.Fatalf("valid path rejected %q %v",good,err)}
 for _,bad:=range []string{"../wow.exe","Data/../../outside.exe","C:/Windows/System32/bad.dll","/outside","\\server\\share","", "Data/../.."}{
  if result,err:=safeFileTarget(root,bad);err==nil{t.Errorf("unsafe path accepted %q => %q",bad,result)}
 }
}
func TestRejectSymlinkPath(t *testing.T){
 root:=t.TempDir();target:=t.TempDir()
 if err:=os.Symlink(target,filepath.Join(root,"Data"));err!=nil{t.Skipf("symlinks not available: %v",err)}
 if _,err:=safeFileTarget(root,"Data/patch.bin");err==nil{t.Fatal("symlink parent accepted")}
}
func TestRequireHTTPS(t *testing.T){
 if !secureURL("https://dawnforge.eu/forever-client/manifest.json"){t.Fatal("HTTPS rejected")}
 for _,bad:=range []string{"http://example.com","file:///C:/tmp/x","https://ok/\r\nbad", ""}{
  if secureURL(bad){t.Errorf("insecure URL accepted: %q",bad)}
 }
}
func TestClientReady(t *testing.T){
 root:=t.TempDir();if clientReady(root){t.Fatal("empty folder is ready")}
 if err:=os.MkdirAll(filepath.Join(root,"_forever_"),0700);err!=nil{t.Fatal(err)}
 if err:=os.MkdirAll(filepath.Join(root,"Data"),0700);err!=nil{t.Fatal(err)}
 if err:=os.WriteFile(filepath.Join(root,"_forever_","Wow.exe"),[]byte("test"),0600);err!=nil{t.Fatal(err)}
 if !clientReady(root){t.Fatal("client folders not recognized")}
 if err:=os.WriteFile(filepath.Join(root,".dawnforge-forever-download-pending"),[]byte("pending"),0600);err!=nil{t.Fatal(err)}
 if clientReady(root){t.Fatal("partial client accepted")}
}

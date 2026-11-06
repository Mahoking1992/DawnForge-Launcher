package hd

import (
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func fixture() Manifest {
 fs:=[]File{}
 for _,name:=range PatchNames {
  b:=sha256.Sum256([]byte(name))
  fs=append(fs,File{Name:name,Size:20,SHA256:hex.EncodeToString(b[:])})
 }
 return Manifest{Version:"1",BaseURL:"https://pub-example.r2.dev",Files:fs}
}

func TestManifestValidation(t *testing.T) {
 m:=fixture()
 if err:=m.Validate();err!=nil{t.Fatal(err)}
 m.Files[1]=m.Files[0]
 if err:=m.Validate();err==nil{t.Fatal("duplicate must be rejected")}
 m=fixture();m.BaseURL="https://account.r2.cloudflarestorage.com"
 if err:=m.Validate();err==nil{t.Fatal("secret S3 API endpoint must be rejected")}
 m=fixture();m.Files[0].SHA256="nope"
 if err:=m.Validate();err==nil{t.Fatal("invalid hash must be rejected")}
 m=fixture();m.Files=m.Files[:12]
 if err:=m.Validate();err==nil{t.Fatal("missing patch must be rejected")}
}

func TestReadManifest(t *testing.T) {
 m:=fixture()
 b,e:=json.Marshal(m);if e!=nil{t.Fatal(e)}
 parsed,e:=ParseManifest(b);if e!=nil||len(parsed.Files)!=13{t.Fatalf("unexpected parsed manifest: %v",e)}
}

func TestClassicKeepsUnknownPatches(t *testing.T) {
 // Never move or delete a player's own similarly named MPQ file.
 root:=t.TempDir();data:=filepath.Join(root,"Data")
 if e:=os.MkdirAll(data,0755);e!=nil{t.Fatal(e)}
 if e:=os.WriteFile(filepath.Join(root,"WoW.exe"),[]byte("stub"),0600);e!=nil{t.Fatal(e)}
 if e:=os.WriteFile(filepath.Join(data,"patch-A.mpq"),[]byte("custom original"),0600);e!=nil{t.Fatal(e)}
 m:=Manager{GameDir:root,Manifest:fixture()}
 err:=m.Classic()
 if err==nil||!strings.Contains(err.Error(),"unknown"){t.Fatalf("expected a safe refusal, got %v",err)}
 b,e:=os.ReadFile(filepath.Join(data,"patch-A.mpq"))
 if e!=nil||string(b)!="custom original"{t.Fatal("unknown MPQ was modified")}
}

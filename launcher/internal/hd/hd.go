// Package hd safely downloads and activates optional Vanilla 1.12.1 HD MPQ patches.
// No R2 API credentials are needed: the manifest contains a public HTTPS base URL.
package hd

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "net/url"
 "os"
 "path/filepath"
 "strconv"
 "strings"
 "time"
)

var PatchNames = []string{"patch-A.mpq","patch-B.mpq","patch-C.mpq","patch-D.mpq","patch-E.mpq","patch-G.mpq","patch-I.mpq","patch-M.mpq","patch-N.mpq","patch-P.mpq","patch-S.mpq","patch-T.mpq","patch-U.mpq"}

type File struct {
 Name string `json:"name"`
 Size int64 `json:"size"`
 SHA256 string `json:"sha256"`
}
type Manifest struct {
 Version string `json:"version"`
 BaseURL string `json:"base_url"`
 Files []File `json:"files"`
}
type Progress func(filename string, written, total int64)
type Manager struct {
 Client *http.Client
 GameDir string
 Manifest Manifest
 Progress Progress
}
func (m Manifest) Validate() error {
 if len(m.Files)!=len(PatchNames) {return fmt.Errorf("expected 13 HD patches, got %d",len(m.Files))}
 u,e:=url.Parse(m.BaseURL)
 if e!=nil || u.Scheme!="https" || u.Hostname()=="" || u.User!=nil || u.RawQuery!="" || u.Fragment!="" {return errors.New("R2 public base_url must be an HTTPS URL without credentials")}
 // The API URL is private S3, not a public download URL.
 if strings.HasSuffix(strings.ToLower(u.Hostname()),".r2.cloudflarestorage.com") {return errors.New("S3 API endpoint cannot be used for public patch downloads")}
 allowed:=map[string]bool{}
 for _,n:=range PatchNames {allowed[n]=true}
 seen:=map[string]bool{}
 for _,f:=range m.Files {
  if !allowed[f.Name] || seen[f.Name] {return fmt.Errorf("unknown or duplicate patch: %q",f.Name)}
  seen[f.Name]=true
  if f.Size<=0 || f.Size>4<<30 {return fmt.Errorf("invalid size for %s",f.Name)}
  b,err:=hex.DecodeString(f.SHA256)
  if err!=nil || len(b)!=sha256.Size {return fmt.Errorf("invalid SHA256 for %s",f.Name)}
 }
 return nil
}
func ParseManifest(data []byte)(Manifest,error){
 var m Manifest
 if len(data)>65536{return m,errors.New("manifest too large")}
 if err:=json.Unmarshal(data,&m);err!=nil{return m,err}
 return m,m.Validate()
}
func (m *Manager) client()*http.Client{
 if m.Client!=nil{return m.Client}
 return &http.Client{Timeout:0,CheckRedirect:func(r *http.Request,via []*http.Request)error{
  if len(via)>5 || r.URL.Scheme!="https"{return errors.New("unsafe download redirect")}
  return nil
 }}
}
func (m *Manager) locations()(data,disabled string,err error){
 if err=m.Manifest.Validate();err!=nil{return}
 if m.GameDir=="" {err=errors.New("game directory not set");return}
 root,e:=filepath.Abs(m.GameDir);if e!=nil{err=e;return}
 // Require an existing base game. This code NEVER downloads or modifies Vanilla itself.
 _,a:=os.Stat(filepath.Join(root,"WoW.exe"))
 _,b:=os.Stat(filepath.Join(root,"wow.exe"))
 if a!=nil && b!=nil {err=errors.New("Vanilla WoW.exe not found; install the normal client first");return}
 data=filepath.Join(root,"Data")
 var st os.FileInfo
 st,err=os.Stat(data)
 if err!=nil{return}
 if !st.IsDir(){err=errors.New("WoW Data is not a directory");return}
 disabled=filepath.Join(data,"_DawnForge_HD_DISABLED")
 err=os.MkdirAll(disabled,0755)
 return
}
func verify(path string,f File)(bool,error){
 st,err:=os.Stat(path)
 if os.IsNotExist(err){return false,nil}
 if err!=nil{return false,err}
 if st.Size()!=f.Size{return false,nil}
 in,err:=os.Open(path);if err!=nil{return false,err};defer in.Close()
 h:=sha256.New()
 if _,err=io.Copy(h,in);err!=nil{return false,err}
 return strings.EqualFold(hex.EncodeToString(h.Sum(nil)),f.SHA256),nil
}
func (m *Manager) fetch(ctx context.Context,f File, dst string) error{
 target:=strings.TrimRight(m.Manifest.BaseURL,"/")+"/hd/"+url.PathEscape(f.Name)
 part:=dst+".part"
 offset:=int64(0)
 if st,err:=os.Stat(part);err==nil{
  offset=st.Size()
  if offset>f.Size {os.Remove(part);offset=0}
 }
 req,err:=http.NewRequestWithContext(ctx,"GET",target,nil)
 if err!=nil{return err}
 if offset>0 {req.Header.Set("Range",fmt.Sprintf("bytes=%d-",offset))}
 resp,err:=m.client().Do(req)
 if err!=nil{return err}
 defer resp.Body.Close()
 if resp.Request.URL.Scheme!="https"{return errors.New("non-HTTPS download denied")}
 if offset>0 && resp.StatusCode==http.StatusPartialContent {
  if !strings.HasPrefix(resp.Header.Get("Content-Range"),"bytes "+strconv.FormatInt(offset,10)+"-") {return fmt.Errorf("invalid Content-Range for %s",f.Name)}
 }else if resp.StatusCode==http.StatusOK{
  offset=0
 }else{return fmt.Errorf("HTTP %d downloading %s",resp.StatusCode,f.Name)}
 flags:=os.O_CREATE|os.O_WRONLY
 if offset>0 {flags|=os.O_APPEND}else{flags|=os.O_TRUNC}
 out,err:=os.OpenFile(part,flags,0644);if err!=nil{return err}
 buf:=make([]byte,1024*1024)
 received:=offset
 for {
  n,readErr:=resp.Body.Read(buf)
  if n>0{
   if received+int64(n)>f.Size{out.Close();return fmt.Errorf("%s exceeds expected size",f.Name)}
   k,writeErr:=out.Write(buf[:n]);received+=int64(k)
   if writeErr!=nil{out.Close();return writeErr}
   if k!=n{out.Close();return io.ErrShortWrite}
   if m.Progress!=nil{m.Progress(f.Name,received,f.Size)}
  }
  if readErr==io.EOF{break}
  if readErr!=nil{out.Close();return readErr}
  if ctx.Err()!=nil{out.Close();return ctx.Err()}
 }
 if err:=out.Sync();err!=nil{out.Close();return err}
 if err:=out.Close();err!=nil{return err}
 ok,err:=verify(part,f)
 if err!=nil{return err}
 if !ok{return fmt.Errorf("SHA256 or size mismatch for %s",f.Name)}
 return os.Rename(part,dst)
}
// Activate downloads into the disabled staging directory, verifies all thirteen,
// then moves them into Data. Never replaces an unknown existing MPQ file.
func (m *Manager) Activate(ctx context.Context)error{
 data,disabled,err:=m.locations();if err!=nil{return err}
 for _,f:=range m.Manifest.Files{
  active:=filepath.Join(data,f.Name)
  stored:=filepath.Join(disabled,f.Name)
  ok,e:=verify(active,f);if e!=nil{return e}
  if ok{continue}
  if _,e=os.Stat(active);e==nil{return fmt.Errorf("refusing to overwrite an existing unrecognized %s",f.Name)}
  if !os.IsNotExist(e){return e}
  ok,e=verify(stored,f);if e!=nil{return e}
  if !ok {
   if e=m.fetch(ctx,f,stored);e!=nil{return e}
  }
 }
 // Preflight before touching any active files.
 for _,f:=range m.Manifest.Files {
  active:=filepath.Join(data,f.Name)
  if _,e:=os.Stat(active);e==nil {
   ok,e:=verify(active,f);if e!=nil{return e}
   if !ok{return fmt.Errorf("unexpected file %s",f.Name)}
  }else if !os.IsNotExist(e){return e}
 }
 moved:=[]string{}
 for _,f:=range m.Manifest.Files{
  active,stored:=filepath.Join(data,f.Name),filepath.Join(disabled,f.Name)
  if _,e:=os.Stat(active);e==nil{continue}
  if e:=os.Rename(stored,active);e!=nil{
   for i:=len(moved)-1;i>=0;i--{
    n:=moved[i];_ = os.Rename(filepath.Join(data,n),filepath.Join(disabled,n))
   }
   return e
  }
  moved=append(moved,f.Name)
 }
 return nil
}
// Classic keeps verified HD patches on disk, but out of the live Data directory.
func (m *Manager) Classic()error{
 data,disabled,err:=m.locations();if err!=nil{return err}
 // Validate all changes first: never move unrelated player patches.
 for _,f:=range m.Manifest.Files {
  active:=filepath.Join(data,f.Name)
  if _,e:=os.Stat(active);os.IsNotExist(e){continue}else if e!=nil{return e}
  ok,e:=verify(active,f);if e!=nil{return e}
  if !ok{return fmt.Errorf("refusing to move unknown patch %s",f.Name)}
  if _,e:=os.Stat(filepath.Join(disabled,f.Name));e==nil{return fmt.Errorf("HD archive already exists for %s",f.Name)}else if !os.IsNotExist(e){return e}
 }
 moved:=[]string{}
 for _,f:=range m.Manifest.Files{
  active,stored:=filepath.Join(data,f.Name),filepath.Join(disabled,f.Name)
  if _,e:=os.Stat(active);os.IsNotExist(e){continue}else if e!=nil{return e}
  if e:=os.Rename(active,stored);e!=nil{
   for i:=len(moved)-1;i>=0;i--{
    n:=moved[i];_=os.Rename(filepath.Join(disabled,n),filepath.Join(data,n))
   }
   return e
  }
  moved=append(moved,f.Name)
 }
 return nil
}
// Load a release manifest from a trusted HTTPS distribution URL.
func FetchManifest(ctx context.Context, link string)(Manifest,error){
 var m Manifest
 u,e:=url.Parse(link)
 if e!=nil || u.Scheme!="https"{return m,errors.New("manifest URL must use HTTPS")}
 client:=&http.Client{Timeout:20*time.Second}
 req,e:=http.NewRequestWithContext(ctx,"GET",link,nil);if e!=nil{return m,e}
 response,e:=client.Do(req);if e!=nil{return m,e};defer response.Body.Close()
 if response.StatusCode!=200{return m,fmt.Errorf("manifest HTTP %d",response.StatusCode)}
 if response.Request.URL.Scheme!="https"{return m,errors.New("manifest redirect is not HTTPS")}
 b,e:=io.ReadAll(io.LimitReader(response.Body,65537));if e!=nil{return m,e}
 return ParseManifest(b)
}

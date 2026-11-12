//go:build windows

package main

import (
 "context"
 "crypto/sha256"
 "encoding/hex"
 "encoding/json"
 "errors"
 "fmt"
 "io"
 "net/http"
 "os"
 "path/filepath"
 "regexp"
 "strings"
 "time"
)
type manifest struct {
 SchemaVersion int
 Launcher *launcherVersionInfo
 Files []manifestFile
}
type launcherVersionInfo struct {Version,URL,SHA256 string}
type manifestFile struct {Path,URL,SHA256 string; Size *int64}

const maxManifestSize=4<<20
const maxGameFileSize=int64(80)<<30
var checksumPattern=regexp.MustCompile("^[0-9a-fA-F]{64}$")
var versionPattern=regexp.MustCompile("^[0-9]+(\\.[0-9]+){1,3}$")

func secureURL(raw string)bool{
 return strings.HasPrefix(raw,"https://")&&!strings.ContainsAny(raw,"\x00\r\n\t")
}
func readManifest(ctx context.Context,url string)(manifest,error){
 var m manifest
 if !secureURL(url){return m,errors.New("Manifest must use HTTPS")}
 req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil);if err!=nil{return m,err}
 req.Header.Set("User-Agent","DawnForgeForeverLauncher/"+launcherVersion)
 resp,err:=webClient.Do(req);if err!=nil{return m,err}
 defer resp.Body.Close()
 if resp.StatusCode!=http.StatusOK{return m,fmt.Errorf("Client manifest unavailable (HTTP %d); no files were changed",resp.StatusCode)}
 raw,err:=io.ReadAll(io.LimitReader(resp.Body,maxManifestSize+1));if err!=nil{return m,err}
 if len(raw)>maxManifestSize{return m,errors.New("Manifest is too large")}
 if err=json.Unmarshal(raw,&m);err!=nil{return m,fmt.Errorf("Malformed manifest: %w",err)}
 if m.SchemaVersion!=1{return m,errors.New("Unsupported manifest format")}
 return m,nil
}
func safeFileTarget(root,relative string)(string,error){
 if root==""||relative==""||filepath.IsAbs(relative)||strings.Contains(relative,":")||
  strings.HasPrefix(relative,"\\")||strings.HasPrefix(relative,"/"){
  return "",errors.New("Unsafe file path in manifest")
 }
 relative=filepath.Clean(strings.ReplaceAll(relative,"/",string(os.PathSeparator)))
 if relative=="."||relative==".."||strings.HasPrefix(relative,".."+string(os.PathSeparator)){
  return "",errors.New("Manifest attempted to leave the game folder")
 }
 absRoot,err:=filepath.Abs(root);if err!=nil{return "",err}
 target,err:=filepath.Abs(filepath.Join(root,relative));if err!=nil{return "",err}
 if !strings.HasPrefix(strings.ToLower(target),strings.ToLower(absRoot+string(os.PathSeparator))){
  return "",errors.New("Manifest attempted to write outside game folder")
 }
 parent:=absRoot
 for _,part:=range strings.Split(filepath.Dir(relative),string(os.PathSeparator)){
  if part==""||part=="."{continue}
  parent=filepath.Join(parent,part)
  st,err:=os.Lstat(parent)
  if err==nil&&st.Mode()&os.ModeSymlink!=0{return "",errors.New("Symlink in game download path")}
  if err!=nil&&!errors.Is(err,os.ErrNotExist){return "",err}
 }
 return target,nil
}
func fileSHA256(path string)(string,error){
 f,err:=os.Open(path);if err!=nil{return "",err}
 defer f.Close()
 h:=sha256.New()
 if _,err=io.Copy(h,f);err!=nil{return "",err}
 return hex.EncodeToString(h.Sum(nil)),nil
}
func downloadResponse(ctx context.Context,url string,offset int64)(*http.Response,int64,error){
 for attempts:=0;attempts<2;attempts++{
  req,err:=http.NewRequestWithContext(ctx,http.MethodGet,url,nil);if err!=nil{return nil,0,err}
  req.Header.Set("User-Agent","DawnForgeForeverLauncher/"+launcherVersion)
  if offset>0{req.Header.Set("Range",fmt.Sprintf("bytes=%d-",offset))}
  response,err:=webClient.Do(req);if err!=nil{return nil,0,err}
  if offset>0{
   expectedPrefix:=fmt.Sprintf("bytes %d-",offset)
   if response.StatusCode!=http.StatusPartialContent||!strings.HasPrefix(response.Header.Get("Content-Range"),expectedPrefix){
    response.Body.Close();offset=0;continue
   }
  }
  if offset==0&&response.StatusCode!=http.StatusOK{
   response.Body.Close()
   return nil,0,fmt.Errorf("File download returned HTTP %d",response.StatusCode)
  }
  return response,offset,nil
 }
 return nil,0,errors.New("Failed to resume download")
}
func verifiedDownload(ctx context.Context,url,sha,target string,maxBytes int64,expected *int64)error{
 if !secureURL(url){return errors.New("File URL must use HTTPS")}
 if !checksumPattern.MatchString(sha){return errors.New("Invalid SHA-256 in manifest")}
 if expected!=nil&&(*expected<0||*expected>maxBytes){return errors.New("Invalid expected file size")}
 if existing,err:=fileSHA256(target);err==nil&&strings.EqualFold(existing,sha){return nil}
 if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{return err}
 part:=target+".dfpart"
 offset:=int64(0)
 if st,err:=os.Stat(part);err==nil{offset=st.Size()}
 if offset>maxBytes||(expected!=nil&&offset>*expected){os.Remove(part);offset=0}
 response,realOffset,err:=downloadResponse(ctx,url,offset)
 if err!=nil{return err}
 defer response.Body.Close()
 if realOffset==0{os.Remove(part)}
 if response.ContentLength>=0&&response.ContentLength+realOffset>maxBytes{return errors.New("File is too large")}
 flags:=os.O_WRONLY|os.O_CREATE
 if realOffset==0{flags|=os.O_TRUNC}else{flags|=os.O_APPEND}
 out,err:=os.OpenFile(part,flags,0600);if err!=nil{return err}
 n,copyErr:=io.Copy(out,io.LimitReader(response.Body,maxBytes-realOffset+1))
 closeErr:=out.Close()
 if copyErr!=nil{return copyErr}
 if closeErr!=nil{return closeErr}
 if realOffset+n>maxBytes{return errors.New("File exceeds download limit")}
 if expected!=nil&&realOffset+n!=*expected{return errors.New("Partial file kept for download resume")}
 computed,err:=fileSHA256(part);if err!=nil{return err}
 if !strings.EqualFold(computed,sha){os.Remove(part);return errors.New("SHA-256 mismatch: downloaded file rejected")}
 backup:=target+".dfbackup"
 os.Remove(backup)
 oldExists:=false
 if _,err:=os.Stat(target);err==nil{
  if err=os.Rename(target,backup);err!=nil{return err}
  oldExists=true
 }
 if err=os.Rename(part,target);err!=nil{
  if oldExists{os.Rename(backup,target)}
  return err
 }
 if oldExists{os.Remove(backup)}
 return nil
}
func syncClientFiles()error{
 dir:=gameDir()
 if dir==""{return errors.New("Choose your existing Forever game folder first")}
 setStatus("Checking Forever game download manifest...")
 ctx,cancel:=context.WithTimeout(context.Background(),45*time.Minute)
 defer cancel()
 m,err:=readManifest(ctx,clientManifestURL);if err!=nil{return err}
 if len(m.Files)>30000{return errors.New("Too many files in manifest")}
 if len(m.Files)==0{
  setStatus("No downloadable game files currently published.")
  showInfo("Forever game files are not yet published in the download manifest. Choose your existing legally acquired client in CHOOSE GAME FOLDER.")
  return nil
 }
 marker:=filepath.Join(dir,".dawnforge-forever-download-pending")
 if err=os.WriteFile(marker,[]byte("Download or repair in progress"),0600);err!=nil{return err}
 for index,item:=range m.Files{
  target,err:=safeFileTarget(dir,item.Path);if err!=nil{return err}
  setStatus(fmt.Sprintf("Checking / downloading %d of %d: %s",index+1,len(m.Files),item.Path))
  if err=verifiedDownload(ctx,item.URL,item.SHA256,target,maxGameFileSize,item.Size);err!=nil{return err}
 }
 if err=os.Remove(marker);err!=nil{return err}
 setStatus("All published client files verified.")
 return nil
}
func compareVersions(a,b string)int{
 parse:=func(s string)[4]int{
  var result [4]int
  parts:=strings.Split(s,".")
  for i:=0;i<len(parts)&&i<4;i++{fmt.Sscanf(parts[i],"%d",&result[i])}
  return result
 }
 x,y:=parse(a),parse(b)
 for i:=0;i<4;i++{if x[i]>y[i]{return 1};if x[i]<y[i]{return -1}}
 return 0
}
func checkLauncherUpdates()error{
 setStatus("Checking launcher updates...")
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Minute)
 defer cancel()
 m,err:=readManifest(ctx,updateManifestURL);if err!=nil{return err}
 if m.Launcher==nil||m.Launcher.Version==""||compareVersions(m.Launcher.Version,launcherVersion)<=0{
  setStatus("Launcher "+launcherVersion+" is up to date.")
  return nil
 }
 if !versionPattern.MatchString(m.Launcher.Version){return errors.New("Invalid update version")}
 filename:="DawnForgeForeverLauncher-"+m.Launcher.Version+".exe"
 target:=filepath.Join(appFolder,"Updates",filename)
 setStatus("Downloading new launcher and verifying SHA-256...")
 if err=verifiedDownload(ctx,m.Launcher.URL,m.Launcher.SHA256,target,100<<20,nil);err!=nil{return err}
 setStatus("Verified launcher update downloaded.")
 showInfo("A newer launcher was verified and saved to:\n"+target+"\n\nClose the current launcher and open the downloaded EXE. The running EXE was not overwritten.")
 return nil
}

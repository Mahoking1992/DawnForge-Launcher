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
 ClientArchive *archiveSpec `json:"clientArchive"`
 Files []manifestFile
}
type launcherVersionInfo struct {Version,URL,SHA256 string}
type manifestFile struct {Path,URL,SHA256 string; Size *int64}
// The optional ZIP archive is supported for old deployment manifests.
type archiveSpec struct {Type,URL,SHA256 string; Size *int64}

const maxManifestSize=4<<20
const maxGameFileSize=int64(80)<<30
var checksumPattern=regexp.MustCompile("^[0-9a-fA-F]{64}$")
var versionPattern=regexp.MustCompile("^[0-9]+(\\.[0-9]+){1,3}$")

// Large 49GB client downloads must not be constrained by the 40-second
// request timeout used for the small status and manifest API requests.
var transferClient=&http.Client{
 Transport:&http.Transport{Proxy:http.ProxyFromEnvironment, ResponseHeaderTimeout:25*time.Second, TLSHandshakeTimeout:15*time.Second, DisableCompression:true},
 CheckRedirect:webClient.CheckRedirect,
}

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
  req.Header.Set("Accept-Encoding","identity")
  response,err:=transferClient.Do(req);if err!=nil{return nil,0,err}
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
// verifiedDownload retains the existing update/ZIP callers unchanged.
func verifiedDownload(ctx context.Context,url,sha,target string,maxBytes int64,expected *int64)error{
 return verifiedDownloadWithProgress(ctx,url,sha,target,maxBytes,expected,nil)
}
// The callback reports bytes already valid/resumed within the current file,
// plus newly transferred network bytes. The UI never guesses a file size.
type progressWriter struct {
 dst io.Writer
 position int64
 report func(int64,int64)
}
func (p *progressWriter)Write(chunk []byte)(int,error){
 n,err:=p.dst.Write(chunk)
 if n>0 {
  p.position+=int64(n)
  if p.report!=nil{p.report(p.position,int64(n))}
 }
 return n,err
}
func verifiedDownloadWithProgress(ctx context.Context,url,sha,target string,maxBytes int64,expected *int64,report func(int64,int64))error{
 if !secureURL(url){return errors.New("File URL must use HTTPS")}
 if !checksumPattern.MatchString(sha){return errors.New("Invalid SHA-256 in manifest")}
 if expected!=nil&&(*expected<0||*expected>maxBytes){return errors.New("Invalid expected file size")}
 if existing,err:=fileSHA256(target);err==nil&&strings.EqualFold(existing,sha){
  if report!=nil {
   if st,err:=os.Stat(target);err==nil{report(st.Size(),0)}
  }
  return nil
 }
 if err:=os.MkdirAll(filepath.Dir(target),0700);err!=nil{return err}
 part:=target+".dfpart"
 offset:=int64(0)
 if st,err:=os.Stat(part);err==nil{offset=st.Size()}
 if offset>maxBytes||(expected!=nil&&offset>*expected){os.Remove(part);offset=0}
 response,realOffset,err:=downloadResponse(ctx,url,offset)
 if err!=nil{return err}
 defer response.Body.Close()
 if realOffset==0{os.Remove(part)}
 if realOffset>0&&report!=nil{report(realOffset,0)}
 if response.ContentLength>=0&&response.ContentLength+realOffset>maxBytes{return errors.New("File is too large")}
 flags:=os.O_WRONLY|os.O_CREATE
 if realOffset==0{flags|=os.O_TRUNC}else{flags|=os.O_APPEND}
 out,err:=os.OpenFile(part,flags,0600);if err!=nil{return err}
 output:=&progressWriter{dst:out,position:realOffset,report:report}
 n,copyErr:=io.CopyBuffer(output,io.LimitReader(response.Body,maxBytes-realOffset+1),make([]byte,256<<10))
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
// downloadByteLabel displays MB for small transfers and GB for full clients.
func downloadByteLabel(n int64)string{
 if n<0{n=0}
 if n<1<<20{return fmt.Sprintf("%.1f KB",float64(n)/1024)}
 if n<1<<30{return fmt.Sprintf("%.1f MB",float64(n)/float64(1<<20))}
 return fmt.Sprintf("%.2f GB",float64(n)/float64(1<<30))
}
func syncClientFiles()error{
 dir:=gameDir()
 if dir==""{return errors.New("Choose your Forever game folder first")}
 setProgress(0)
 setDownloadDetails("Loading download manifest...")
 setStatus("Checking Forever game download manifest...")
 ctx,cancel:=context.WithTimeout(context.Background(),24*time.Hour)
 defer cancel()
 m,err:=readManifest(ctx,clientManifestURL);if err!=nil{return err}
 if len(m.Files)>30000{return errors.New("Too many files in manifest")}
 if len(m.Files)==0{
  if m.ClientArchive!=nil{return installClientArchive(ctx,dir,*m.ClientArchive)}
  setDownloadDetails("No client downloads are published.")
  setStatus("No downloadable game files currently published.")
  showInfo("Forever client download is not currently published in the HTTPS manifest. Existing installations may be selected in Options.")
  return nil
 }
 var totalBytes int64
 allSizesKnown:=true
 for _,file:=range m.Files{
  if file.Size==nil||*file.Size<0||*file.Size>maxGameFileSize{
   allSizesKnown=false
  }else if totalBytes<=int64(^uint64(0)>>1)-*file.Size{
   totalBytes+=*file.Size
  }else{allSizesKnown=false}
 }
 var verifiedDone,transferred int64
 started:=time.Now()
 lastRefresh:=time.Time{}
 display:=func(current int64,final bool){
  now:=time.Now()
  if !final&&!lastRefresh.IsZero()&&now.Sub(lastRefresh)<250*time.Millisecond{return}
  lastRefresh=now
  overall:=verifiedDone+current
  if allSizesKnown&&totalBytes>0{
   if overall>totalBytes{overall=totalBytes}
   setProgress(int(float64(overall)*100/float64(totalBytes)))
  }
  elapsed:=now.Sub(started).Seconds()
  speed:=0.0
  if elapsed>0{speed=float64(transferred)/elapsed/float64(1<<20)}
  if allSizesKnown&&totalBytes>0{
   setDownloadDetails(fmt.Sprintf("Overené: %s / %s | Prenesené: %s | %.1f MB/s",
    downloadByteLabel(overall),downloadByteLabel(totalBytes),downloadByteLabel(transferred),speed))
  }else{
   setDownloadDetails(fmt.Sprintf("Overené: %s | Prenesené: %s | %.1f MB/s",
    downloadByteLabel(overall),downloadByteLabel(transferred),speed))
  }
 }
 marker:=filepath.Join(dir,".dawnforge-forever-download-pending")
 if err=os.WriteFile(marker,[]byte("Download or repair in progress"),0600);err!=nil{return err}
 for index,item:=range m.Files{
  target,err:=safeFileTarget(dir,item.Path);if err!=nil{return err}
  setStatus(fmt.Sprintf("Checking / downloading %d of %d: %s",index+1,len(m.Files),item.Path))
  var fileProgress int64
  report:=func(fileBytes,networkDelta int64){
   if fileBytes<0{fileBytes=0}
   fileProgress=fileBytes
   if networkDelta>0{transferred+=networkDelta}
   display(fileProgress,false)
  }
  if err=verifiedDownloadWithProgress(ctx,item.URL,item.SHA256,target,maxGameFileSize,item.Size,report);err!=nil{
   display(fileProgress,true)
   return err
  }
  if st,err:=os.Stat(target);err==nil{verifiedDone+=st.Size()}
  display(0,true)
 }
 if err=os.Remove(marker);err!=nil{return err}
 if !clientReady(dir){return errors.New("Downloaded manifest completed but _forever_\\Wow.exe or Data is missing; verify the published client manifest")}
 if allSizesKnown{setProgress(100)}
 display(0,true)
 setStatus("All client files verified. PLAY is ready to test.")
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

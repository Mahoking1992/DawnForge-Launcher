//go:build windows

package main

import (
 "archive/zip"
 "context"
 "errors"
 "fmt"
 "io"
 "os"
 "path/filepath"
 "strings"
)

// ZIP fallback preserves the old 2026-11-11 one-click installer behavior.
// The primary production flow uses individual VPS HTTPS files when published.
func installClientArchive(ctx context.Context,dir string,info archiveSpec)error{
 if strings.ToLower(info.Type)!="zip"{return errors.New("Only a SHA-256-verified ZIP client archive is supported")}
 if dir==""{return errors.New("Choose an installation folder first")}
 entries,err:=os.ReadDir(dir)
 if err!=nil{return err}
 if len(entries)>0{return errors.New("ZIP installation requires an empty destination; choose another folder or use file-by-file repair")}
 cache:=dir+".dfdownloads"
 if err=os.MkdirAll(cache,0700);err!=nil{return err}
 archivePath:=filepath.Join(cache,"DawnForgeForeverClient.zip")
 setStatus("Downloading Forever ZIP archive from HTTPS server...")
 if err=verifiedDownload(ctx,info.URL,info.SHA256,archivePath,100<<30,info.Size);err!=nil{return err}
 stage:=dir+".dfstage"
 if _,err=os.Stat(stage);err==nil{return errors.New("Previous staging folder exists; inspect it before retrying: "+stage)}
 if err=os.Mkdir(stage,0700);err!=nil{return err}
 setStatus("Verifying and extracting Forever client...")
 if err=extractSafeZIP(archivePath,stage);err!=nil{return err}
 if !clientReady(stage){return errors.New("ZIP is missing _forever_/Wow.exe or Data. Staging retained for inspection")}
 if err=os.Remove(dir);err!=nil{return fmt.Errorf("Could not remove empty destination: %w",err)}
 if err=os.Rename(stage,dir);err!=nil{
  _=os.Mkdir(dir,0700)
  return fmt.Errorf("Could not finish the same-drive client installation: %w",err)
 }
 // Remove only our known verified temporary ZIP; never delete existing files.
 _=os.Remove(archivePath)
 _=os.Remove(cache)
 setStatus("Forever client installed and verified. Use PLAY to test connectivity.")
 return nil
}
func extractSafeZIP(source,destination string)error{
 reader,err:=zip.OpenReader(source);if err!=nil{return err}
 defer reader.Close()
 if len(reader.File)>100000{return errors.New("ZIP has too many entries")}
 total:=uint64(0)
 for i,entry:=range reader.File{
  name:=strings.ReplaceAll(entry.Name,"\\","/")
  name=strings.TrimSuffix(name,"/")
  if name==""{continue}
  if entry.Mode()&os.ModeSymlink!=0||(!entry.Mode().IsRegular()&&!entry.FileInfo().IsDir()){
   return fmt.Errorf("ZIP contains unsupported file type: %q",entry.Name)
  }
  total+=entry.UncompressedSize64
  if total>160<<30{return errors.New("ZIP expands beyond 160 GiB")}
  target,err:=safeFileTarget(destination,name);if err!=nil{return err}
  if entry.FileInfo().IsDir(){
   if err=os.MkdirAll(target,0700);err!=nil{return err}
   continue
  }
  if err=os.MkdirAll(filepath.Dir(target),0700);err!=nil{return err}
  input,err:=entry.Open();if err!=nil{return err}
  output,err:=os.OpenFile(target,os.O_CREATE|os.O_EXCL|os.O_WRONLY,0600)
  if err!=nil{input.Close();return err}
  n,copyErr:=io.Copy(output,io.LimitReader(input,int64(entry.UncompressedSize64)+1))
  closeErr:=output.Close()
  input.Close()
  if copyErr!=nil{return copyErr}
  if closeErr!=nil{return closeErr}
  if uint64(n)!=entry.UncompressedSize64{return fmt.Errorf("Truncated or oversized ZIP entry: %s",name)}
  if i%20==0{setStatus(fmt.Sprintf("Unpacking Forever: %d/%d files",i+1,len(reader.File)))}
 }
 return nil
}

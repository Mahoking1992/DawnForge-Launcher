package main

// HD play support for the local TEST client only. No automatic game downloads,
// file deletion, registry changes, or modifications to the original client.
import (
 "bytes"
 "encoding/base64"
 "unicode/utf16"
 "crypto/sha256"
 "encoding/hex"
 "encoding/xml"
 "errors"
 "fmt"
 "io"
 "os"
 "os/exec"
 "path/filepath"
 "strings"
 "syscall"
 "time"
 "unsafe"
)

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
// Alias the HD test directory via the Unicode Win32 DOS-device API.
// The path contains an en dash and an accented character, so the ANSI
// console output from subst.exe is unreliable.
func dfSubst(root string) (string,error) {
 k32:=syscall.NewLazyDLL("kernel32.dll")
 getDrives:=k32.NewProc("GetLogicalDrives")
 queryDevice:=k32.NewProc("QueryDosDeviceW")
 defineDevice:=k32.NewProc("DefineDosDeviceW")
 root=filepath.Clean(root)
 target:="\\??\\"+root
 wantedFile:=filepath.Join(root,"WoW.exe")
 stWanted,err:=os.Stat(wantedFile)
 if err!=nil {return "",fmt.Errorf("HD WoW.exe unavailable: %w",err)}
 mask,_,e:=getDrives.Call()
 if mask==0 {return "",fmt.Errorf("GetLogicalDrives failed: %v",e)}
 // First reuse any existing subst mapping for precisely this client.
 for ch:='Z';ch>='D';ch-- {
  drive:=string(ch)+":"
  if mask&(1<<uint(ch-'A'))==0 {continue}
  ptr:=syscall.StringToUTF16Ptr(drive)
  buf:=make([]uint16,2048)
  count,_,_:=queryDevice.Call(uintptr(unsafe.Pointer(ptr)),uintptr(unsafe.Pointer(&buf[0])),uintptr(len(buf)))
  if count==0 {continue}
  native:=syscall.UTF16ToString(buf)
  native=strings.TrimPrefix(native,"\\??\\")
  if !strings.EqualFold(filepath.Clean(native),root) {continue}
  stMapped,mappingError:=os.Stat(drive+"\\WoW.exe")
  if mappingError==nil && os.SameFile(stMapped,stWanted) {
   dfHDLog("Reusing HD alias "+drive+" -> "+root)
   return drive,nil
  }
 }
 errorsSeen:=[]string{}
 for _,ch:=range "ZYXWVUTSRQPONMLKJIHGFED" {
  if mask&(1<<uint(ch-'A'))!=0 {continue}
  drive:=string(ch)+":"
  drivePtr:=syscall.StringToUTF16Ptr(drive)
  pathPtr:=syscall.StringToUTF16Ptr(target)
  ok,_,createErr:=defineDevice.Call(0x1,
   uintptr(unsafe.Pointer(drivePtr)),uintptr(unsafe.Pointer(pathPtr)))
  if ok==0 {
   if len(errorsSeen)<3 {errorsSeen=append(errorsSeen,drive+" "+createErr.Error())}
   continue
  }
  stMapped,readErr:=os.Stat(drive+"\\WoW.exe")
  if readErr==nil && os.SameFile(stMapped,stWanted) {
   dfHDLog("Created HD alias "+drive+" -> "+root)
   return drive,nil
  }
  // Roll back only the virtual alias we just created, never an existing drive.
  defineDevice.Call(0x1|0x2|0x4,
   uintptr(unsafe.Pointer(drivePtr)),uintptr(unsafe.Pointer(pathPtr)))
  if len(errorsSeen)<3 {errorsSeen=append(errorsSeen,drive+" verification: "+fmt.Sprint(readErr))}
 }
 if len(errorsSeen)==0 {
  return "",errors.New("No free drive letter. Check current virtual drives.")
 }
 return "",fmt.Errorf("Cannot create HD alias: %s",strings.Join(errorsSeen,"; "))
}
func dfHDLog(entry string) {
 dir:=filepath.Join(os.Getenv("LOCALAPPDATA"),"DawnForge","LauncherTest")
 if err:=os.MkdirAll(dir,0700);err!=nil{return}
 f,err:=os.OpenFile(filepath.Join(dir,"hd-launch.log"),os.O_CREATE|os.O_APPEND|os.O_WRONLY,0600)
 if err!=nil{return}
 defer f.Close()
 fmt.Fprintf(f,"%s %s\n",time.Now().Format(time.RFC3339),entry)
}

// wowreeb 3.5 uses narrow filesystem paths in several Windows calls.
// Never launch it from the accented HD game directory. Build a read-only-to-game
// copy of the official loader in a per-version ASCII-only test directory.
// This deliberately does not restore or overwrite any previous game config.
func dfIsASCII(s string) bool {
 for _,r:=range s {if r>127{return false}}
 return true
}
func dfStageFile(source,destination string) error {
 data,err:=os.ReadFile(source)
 if err!=nil{return err}
 if existing,err:=os.ReadFile(destination);err==nil {
  if bytes.Equal(existing,data){return nil}
  return fmt.Errorf("staged file differs; refusing to overwrite %s",destination)
 } else if !os.IsNotExist(err){return err}
 f,err:=os.OpenFile(destination,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return err}
 n,e:=f.Write(data)
 ce:=f.Close()
 if e!=nil || ce!=nil || n!=len(data) {
  _=os.Remove(destination)
  if e!=nil{return e}
  if ce!=nil{return ce}
  return io.ErrShortWrite
 }
 return nil
}
type dfWowreebConfig struct {
 XMLName xml.Name `xml:"wowreeb"`
 Config []struct {Name string `xml:"Name,attr"`; Value string `xml:"Value,attr"`} `xml:"Config"`
 Realms []dfWowreebRealm `xml:"Realm"`
}
type dfWowreebRealm struct {
 Name string `xml:"Name,attr"`
 Exe struct {Path string `xml:"Path,attr"`; SHA256 string `xml:"SHA256,attr"`} `xml:"Exe"`
 DLLs []struct {Path string `xml:"Path,attr"`} `xml:"DLL"`
}
func dfPrepareASCIILoader(sourceSetup,gameRoot,drive string) (string,string,error) {
 sourceEXE:=filepath.Join(sourceSetup,"wowreeb32.exe")
 sourceDLL:=filepath.Join(sourceSetup,"wowreeb32.dll")
 exeHash,err:=dfHash(sourceEXE)
 if err!=nil{return "","",err}
 dllHash,err:=dfHash(sourceDLL)
 if err!=nil{return "","",err}
 gameHash,err:=dfHash(filepath.Join(gameRoot,"WoW.exe"))
 if err!=nil{return "","",err}
 // No secrets, usernames, spaces, accents, Unicode punctuation, or redirects.
 local:=os.Getenv("LOCALAPPDATA")
 if !dfIsASCII(local) || local=="" {
  return "","",errors.New("Windows LOCALAPPDATA path is not ASCII; choose an ASCII-only test directory before HD launcher testing")
 }
 session:=filepath.Join(local,"DawnForge","LauncherTest","HDLoader",exeHash[:12]+"-"+dllHash[:12]+"-"+gameHash[:8])
 if !dfIsASCII(session){return "","",errors.New("Test loader path contains unsupported characters")}
 if err:=os.MkdirAll(session,0700);err!=nil{return "","",err}
 if err:=dfStageFile(sourceEXE,filepath.Join(session,"wowreeb32.exe"));err!=nil{return "","",err}
 if err:=dfStageFile(sourceDLL,filepath.Join(session,"wowreeb32.dll"));err!=nil{return "","",err}
 const entry="DawnForge HD Test"
 cfg:=dfWowreebConfig{}
 cfg.Config=append(cfg.Config,struct {
  Name string `xml:"Name,attr"`
  Value string `xml:"Value,attr"`
 }{Name:"ClearWDB",Value:"0"})
 realm:=dfWowreebRealm{Name:entry}
 realm.Exe.Path=drive+`\WoW.exe`
 realm.Exe.SHA256=gameHash
 realm.DLLs=append(realm.DLLs,struct{Path string `xml:"Path,attr"`}{Path:drive+`\VanillaHelpers.dll`})
 cfg.Realms=[]dfWowreebRealm{realm}
 data,err:=xml.MarshalIndent(cfg,"","  ")
 if err!=nil{return "","",err}
 data=append([]byte(xml.Header),data...)
 configPath:=filepath.Join(session,"config.xml")
 if err:=dfStageBytes(data,configPath);err!=nil{return "","",err}
 dfHDLog("Created isolated ASCII wowreeb session "+session)
 return session,entry,nil
}
func dfStageBytes(data []byte,destination string) error {
 if old,err:=os.ReadFile(destination);err==nil {
  if bytes.Equal(old,data){return nil}
  return fmt.Errorf("isolated config already exists with different content: %s",destination)
 } else if !os.IsNotExist(err){return err}
 f,err:=os.OpenFile(destination,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600)
 if err!=nil{return err}
 _,e:=f.Write(data)
 ce:=f.Close()
 if e!=nil || ce!=nil {
  _=os.Remove(destination)
  if e!=nil{return e}
  return ce
 }
 return nil
}
func dfLaunchHD() error {
 dfHDLog("HD PLAY pressed - replaying working Z reuse + config + MPQ + PowerShell launch sequence")
 ps:= "$ErrorActionPreference = 'Stop'\n$ProgressPreference = 'SilentlyContinue'\n$ids = @('A','B','C','D','E','G','I','M','N','P','S','T','U')\n$helperSha = '1e74ddf6484c23482550d5f9e5f9d291085c4ac538d3b3ed9d87155521841196'\nfunction Fail([string]$message) { throw $message }\nfunction Sha([string]$path) {\n $h = [System.Security.Cryptography.SHA256]::Create()\n $stream = [System.IO.File]::OpenRead($path)\n try { return ([BitConverter]::ToString($h.ComputeHash($stream))).Replace('-', '').ToLowerInvariant() }\n finally { $stream.Dispose(); $h.Dispose() }\n}\nfunction IsHDProcess([string]$original,[string]$mapped) {\n foreach($p in @(Get-CimInstance Win32_Process -Filter \"Name='WoW.exe'\" -ErrorAction Stop)) {\n  if (-not $p.ExecutablePath) { continue }\n  if ([string]::Equals($p.ExecutablePath,$original,[StringComparison]::OrdinalIgnoreCase) -or\n      [string]::Equals($p.ExecutablePath,$mapped,[StringComparison]::OrdinalIgnoreCase)) {return $true}\n }\n return $false\n}\n$copies = @(Get-ChildItem -LiteralPath 'C:\\' -Directory | Where-Object {\n $_.Name -match '(?i)^wow\\s*[-\\u2013\\u2014]\\s*k[o\\u00f3]pia$' -and\n (Test-Path -LiteralPath (Join-Path $_.FullName 'WoW.exe') -PathType Leaf)\n})\nif ($copies.Count -ne 1) {Fail 'Expected exactly one HD TEST client (C:\\wow - kopia).'}\n$game = [string]$copies[0].FullName\nif ($game -ieq 'C:\\wow') {Fail 'Original C:\\wow cannot be used as HD TEST'}\n$gameExe = Join-Path $game 'WoW.exe'\n$setup = Join-Path $game '_DawnForge_HD_Wowreeb_3_5'\n$loader = Join-Path $setup 'wowreeb32.exe'\n$injection = Join-Path $setup 'wowreeb32.dll'\n$vanillaHelper = Join-Path $game 'VanillaHelpers.dll'\n$disabled = Join-Path $game '_DawnForge_HD_DISABLED'\n$data = Join-Path $game 'Data'\nforeach($f in @($gameExe,$loader,$injection,$vanillaHelper)) {\n if (!(Test-Path -LiteralPath $f -PathType Leaf)){Fail ('Missing HD test file: '+$f)}\n}\nif ((Sha $vanillaHelper) -ne $helperSha) {Fail 'VanillaHelpers.dll SHA256 mismatch'}\n$mpqs = @()\nforeach($id in $ids) {\n $name = 'patch-'+$id+'.mpq'\n $active = Join-Path $data $name\n $off = Join-Path $disabled $name\n $a = Test-Path -LiteralPath $active -PathType Leaf\n $b = Test-Path -LiteralPath $off -PathType Leaf\n if ($a -eq $b){Fail ('Missing or duplicate HD module: '+$name)}\n $mpqs += [pscustomobject]@{Name=$name;Active=$active;Off=$off;Enable=$b}\n}\nAdd-Type -TypeDefinition @'\nusing System;\nusing System.Text;\nusing System.Runtime.InteropServices;\npublic static class DfDriveQueryHDTest {\n [DllImport(\"kernel32.dll\", CharSet=CharSet.Unicode, SetLastError=true)]\n public static extern uint QueryDosDevice(string name,StringBuilder target,int size);\n}\n'@\nfunction Get-AliasTarget([string]$letter) {\n $buffer = New-Object System.Text.StringBuilder 4096\n $count = [DfDriveQueryHDTest]::QueryDosDevice($letter+':',$buffer,$buffer.Capacity)\n if ($count -eq 0){return $null}\n $native = $buffer.ToString().Split([char]0)[0]\n if ($native.StartsWith('\\??\\')) {$native=$native.Substring(4)}\n if ($native.StartsWith('\\?\\')) {$native=$native.Substring(4)}\n return $native.TrimEnd('\\')\n}\n$mapped = $null\n$allLetters = @('Z','Y','X','W','V','U','T','S','R','Q','P','O','N','M','L','K','J','I','H','G','F','E','D')\nforeach($letter in $allLetters) {\n $target = Get-AliasTarget $letter\n if ($target -and [string]::Equals($target,$game.TrimEnd('\\'),[StringComparison]::OrdinalIgnoreCase)) {\n  $mapped = $letter+':'; Write-Output ('REUSED_HD_DRIVE='+$mapped);break\n }\n}\nif (-not $mapped) {\n $subst = Join-Path $env:SystemRoot 'System32\\subst.exe'\n foreach($letter in $allLetters) {\n  if (Get-AliasTarget $letter) {continue}\n  if (Test-Path -LiteralPath ($letter+':\\')) {continue}\n  if (Get-PSDrive -Name $letter -PSProvider FileSystem -ErrorAction SilentlyContinue) {continue}\n  & $subst ($letter+':') $game | Out-Null\n  if ($LASTEXITCODE -ne 0) {continue}\n  $target = Get-AliasTarget $letter\n  if ([string]::Equals($target,$game.TrimEnd('\\'),[StringComparison]::OrdinalIgnoreCase)){\n   $mapped=$letter+':';Write-Output ('CREATED_HD_DRIVE='+$mapped);break\n  }\n  Fail ('New HD virtual drive could not be verified: '+$letter)\n }\n}\nif (-not $mapped) {Fail 'No HD virtual drive is available'}\n$aliasExe = $mapped+'\\WoW.exe'\n$aliasHelper = $mapped+'\\VanillaHelpers.dll'\nif (!(Test-Path -LiteralPath $aliasExe -PathType Leaf)) {Fail ('No mapped WoW.exe at '+$aliasExe)}\nif (!(Test-Path -LiteralPath $aliasHelper -PathType Leaf)) {Fail ('No mapped VanillaHelpers at '+$aliasHelper)}\nif ((Sha $aliasExe) -ne (Sha $gameExe)){Fail 'Mapped WoW.exe checksum mismatch'}\nif ((Sha $aliasHelper) -ne $helperSha){Fail 'Mapped helper checksum mismatch'}\nif (IsHDProcess $gameExe $aliasExe) {\n Write-Output 'HD_ALREADY_RUNNING';exit 0\n}\n$gameHash = Sha $gameExe\n$config = Join-Path $setup 'config.xml'\n$newXml = @\"\n<?xml version=\"1.0\" encoding=\"utf-8\"?>\n<wowreeb>\n  <Config Name=\"ClearWDB\" Value=\"0\" />\n  <Realm Name=\"DawnForge HD Test\">\n    <Exe Path=\"$aliasExe\" SHA256=\"$gameHash\" />\n    <DLL Path=\"$aliasHelper\" />\n  </Realm>\n</wowreeb>\n\"@\nif (Test-Path -LiteralPath $config -PathType Leaf) {\n $old = [IO.File]::ReadAllText($config)\n if ($old.Trim() -ne $newXml.Trim()) {\n  $backup=Join-Path $setup ('config-before-hd-play-'+(Get-Date -Format 'yyyyMMdd-HHmmssfff')+'.xml')\n  Copy-Item -LiteralPath $config -Destination $backup -ErrorAction Stop\n }\n}\n[IO.File]::WriteAllText($config,$newXml,(New-Object Text.UTF8Encoding($false)))\n[xml]$check = [IO.File]::ReadAllText($config)\nif ($check.wowreeb.Realm.Exe.Path -cne $aliasExe) {Fail 'Wowreeb XML validation failed'}\n$moved = @()\ntry {\n foreach($item in $mpqs) {\n  if ($item.Enable) {\n   if(Test-Path -LiteralPath $item.Active){Fail ('Unexpected duplicate HD module: '+$item.Name)}\n   Move-Item -LiteralPath $item.Off -Destination $item.Active -ErrorAction Stop\n   $moved += $item\n  }\n }\n} catch {\n foreach($item in $moved) {if(Test-Path -LiteralPath $item.Active){Move-Item -LiteralPath $item.Active -Destination $item.Off -ErrorAction SilentlyContinue}}\n throw\n}\nWrite-Output 'ALL_13_HD_PATCHES_ACTIVE'\n$env:WOWREEB_ENTRY = 'DawnForge HD Test'\n$launched = Start-Process -FilePath $loader -WorkingDirectory $setup -PassThru -ErrorAction Stop\nWrite-Output ('WOWREEB_PID='+$launched.Id)\n# WoW can appear after wowreeb has already exited successfully.\n# Do not claim failure after a fixed three-second interval.\n$seen = $false\nfor($n=0; $n -lt 20; $n++) {\n Start-Sleep -Milliseconds 500\n if (IsHDProcess $gameExe $aliasExe) {$seen=$true;break}\n}\nif ($seen) {\n Write-Output 'CONFIRMED_HD_GAME_RUNNING'\n} else {\n $launched.Refresh()\n if ($launched.HasExited) {\n  Write-Output ('WOWREEB_EXIT='+$launched.ExitCode)\n  if ($launched.ExitCode -ne 0) {Fail ('wowreeb reported exit code '+$launched.ExitCode)}\n }\n Write-Output 'HD_GAME_NOT_YET_VERIFIED_CHECK_WINDOW'\n}\n"
 encoded:=utf16.Encode([]rune(ps))
 bytesUTF16:=make([]byte,len(encoded)*2)
 for i,v:=range encoded { bytesUTF16[2*i]=byte(v); bytesUTF16[2*i+1]=byte(v>>8) }
 cmd:=exec.Command("powershell.exe","-NoProfile","-NonInteractive","-EncodedCommand",base64.StdEncoding.EncodeToString(bytesUTF16))
 cmd.SysProcAttr=&syscall.SysProcAttr{HideWindow:true,CreationFlags:0x08000000}
 output,err:=cmd.CombinedOutput()
 result:=strings.TrimSpace(string(output))
 dfHDLog("HD PowerShell sequence: "+result)
 if err!=nil {return fmt.Errorf("HD launch sequence failed: %w; %s",err,result)}
 dfHDLog("HD launch sequence finished - game visibility must be confirmed by user")
 return nil
}

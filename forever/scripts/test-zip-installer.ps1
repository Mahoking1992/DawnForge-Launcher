$ErrorActionPreference = 'Stop'
Add-Type -AssemblyName System.IO.Compression,System.IO.Compression.FileSystem
$launcherPath = Join-Path $PSScriptRoot '../app/launcher.ps1'
$tokens = $null
$parseErrors = $null
$ast = [System.Management.Automation.Language.Parser]::ParseFile((Resolve-Path $launcherPath),[ref]$tokens,[ref]$parseErrors)
if($parseErrors.Count -gt 0){throw 'Forever launcher PowerShell syntax is invalid.'}
$names = @('Safe-Target','Install-ClientArchive')
$functions = @($ast.FindAll({param($n) $n -is [System.Management.Automation.Language.FunctionDefinitionAst] -and $n.Name -in $names},$true))
if($functions.Count -ne 2){throw 'ZIP installer functions are missing.'}
foreach($function in $functions){. ([scriptblock]::Create($function.Extent.Text))}
$root = Join-Path $env:RUNNER_TEMP ('forever-installer-test-'+[guid]::NewGuid().ToString('N'))
[IO.Directory]::CreateDirectory($root) | Out-Null
try {
  $zip = Join-Path $root 'mock-client.zip'
  $staged = Join-Path $root 'input'
  [IO.Directory]::CreateDirectory((Join-Path $staged '_forever_')) | Out-Null
  [IO.File]::WriteAllText((Join-Path $staged '_forever_/Wow.exe'),'synthetic-fixture-not-real-client')
  [IO.File]::WriteAllText((Join-Path $staged '_forever_/readme.txt'),'test only')
  [IO.Compression.ZipFile]::CreateFromDirectory($staged,$zip)
  $script:cfg = @{ GameDir = '' }
  function Save-Settings { }
  $dest=Join-Path $root 'installed'
  Install-ClientArchive $zip $dest
  if(-not (Test-Path (Join-Path $dest '_forever_/Wow.exe'))){throw 'Installer failed to extract a valid fixture.'}
  if($cfg.GameDir -ne $dest){throw 'Installer did not register the game folder.'}
  Write-Host 'PASS: synthetic ZIP extracts and registers game directory'
  $malicious=Join-Path $root 'unsafe.zip'
  $f=[IO.File]::Open($malicious,[IO.FileMode]::Create)
  try{
    $z=New-Object IO.Compression.ZipArchive($f,[IO.Compression.ZipArchiveMode]::Create,$true)
    try{
      $e=$z.CreateEntry('../escape.txt')
      $w=New-Object IO.StreamWriter($e.Open())
      try{$w.Write('should-not-extract')}finally{$w.Dispose()}
    }finally{$z.Dispose()}
  }finally{$f.Dispose()}
  $caught=$false
  try{ Install-ClientArchive $malicious (Join-Path $root 'unsafe-install') }catch{$caught=$true}
  if(-not $caught){throw 'Installer allowed ZIP path traversal.'}
  if(Test-Path (Join-Path $root 'escape.txt')){throw 'Unsafe entry escaped staging directory.'}
  Write-Host 'PASS: unsafe ZIP path traversal rejected'
}finally{Remove-Item -LiteralPath $root -Recurse -Force -ErrorAction SilentlyContinue}

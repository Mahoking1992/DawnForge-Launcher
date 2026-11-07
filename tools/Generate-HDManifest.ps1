# Generate the immutable HD manifest from the ORIGINAL 13 MPQ files.
# Example:
# .\\Generate-HDManifest.ps1 -DataDir 'C:\\wow – kópia\\Data' -PublicBaseUrl 'https://YOUR-PUBLIC-R2-DOMAIN'
# Copy output hd-manifest.json to download/hd-manifest.json in the repository.
param(
 [Parameter(Mandatory=$true)][string]$DataDir,
 [Parameter(Mandatory=$true)][string]$PublicBaseUrl,
 [string]$OutputPath = (Join-Path $PSScriptRoot 'hd-manifest.json')
)
$ErrorActionPreference='Stop'
if(-not $PublicBaseUrl.StartsWith('https://')) {throw 'Use HTTPS public R2 domain, never S3 API endpoint.'}
if($PublicBaseUrl -match '\\.r2\\.cloudflarestorage\\.com') {throw 'Cannot use S3 API endpoint for public downloads.'}
$names=@('patch-A.mpq','patch-B.mpq','patch-C.mpq','patch-D.mpq','patch-E.mpq','patch-G.mpq','patch-I.mpq','patch-M.mpq','patch-N.mpq','patch-P.mpq','patch-S.mpq','patch-T.mpq','patch-U.mpq')
$files=@(foreach($name in $names){
 $path=Join-Path $DataDir $name
 if(!(Test-Path -LiteralPath $path -PathType Leaf)){throw "Missing: $path"}
 $item=Get-Item -LiteralPath $path
 $hash=(Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash.ToLowerInvariant()
 [ordered]@{name=$name;size=[int64]$item.Length;sha256=$hash}
})
$manifest=[ordered]@{version='1';base_url=$PublicBaseUrl.TrimEnd('/');files=$files}
$json = $manifest | ConvertTo-Json -Depth 5
[System.IO.File]::WriteAllText($OutputPath, $json, (New-Object System.Text.UTF8Encoding($false)))
Write-Host "Manifest created: $OutputPath"
Write-Host "Objects: $($files.Count); Total bytes: $(($files|Measure-Object -Property size -Sum).Sum)"
Write-Host 'VERIFY: Each public URL must be accessible without login before publishing the launcher.'

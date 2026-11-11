param(
    [Parameter(Mandatory=$true)][string]$AppDirectory,
    [Parameter(Mandatory=$true)][string]$LauncherExe
)
$ErrorActionPreference='Stop'
Add-Type -AssemblyName PresentationFramework,PresentationCore,WindowsBase,System.Windows.Forms,Microsoft.VisualBasic,System.IO.Compression,System.IO.Compression.FileSystem
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12
$settingsFile=Join-Path $AppDirectory 'settings.json'
$defaultManifest='https://raw.githubusercontent.com/Mahoking1992/DawnForge-Launcher/main/forever/update-manifest.json'
$defaultClientManifest='https://dawnforge.eu/forever-client/manifest.json'
$defaults=@{
    SiteUrl='https://dawnforge.eu/'
    ManifestUrl=$defaultManifest
    GameDir=''
    ConnectorExe=''
    ServerHost=''
    ServerPort=1119
}
function Load-Settings {
    $v=@{}
    foreach($k in $defaults.Keys) {$v[$k]=$defaults[$k]}
    if(Test-Path -LiteralPath $settingsFile) {
        try {
            $r=Get-Content -LiteralPath $settingsFile -Raw | ConvertFrom-Json
            foreach($k in $defaults.Keys) {if($null -ne $r.$k) {$v[$k]=$r.$k}}
        } catch { }
    }
    return $v
}
$cfg=Load-Settings
function Save-Settings {
    $json=$cfg | ConvertTo-Json -Depth 5
    $temp="$settingsFile.tmp"
    [IO.File]::WriteAllText($temp,$json,[Text.Encoding]::UTF8)
    Move-Item -LiteralPath $temp -Destination $settingsFile -Force
}
function Valid-Https([string]$url) {
    $uri=$null
    if(-not [Uri]::TryCreate($url,[UriKind]::Absolute,[ref]$uri)) {return $false}
    return $uri.Scheme -eq 'https' -and -not $uri.UserInfo -and $uri.Host
}
function Msg([string]$body,[string]$caption='DawnForge Forever') {
    [System.Windows.MessageBox]::Show($body,$caption,[System.Windows.MessageBoxButton]::OK,[System.Windows.MessageBoxImage]::Information) | Out-Null
}
function Err([string]$body) {
    [System.Windows.MessageBox]::Show($body,'DawnForge Forever',[System.Windows.MessageBoxButton]::OK,[System.Windows.MessageBoxImage]::Warning) | Out-Null
}
function Set-Status([string]$str,[int]$pct=-1) {
    $StatusText.Text=$str
    if($pct -ge 0){$Progress.Value=[Math]::Min(100,[Math]::Max(0,$pct))}
    [System.Windows.Forms.Application]::DoEvents()
}
function Get-Manifest([string]$url='') {
    if([string]::IsNullOrWhiteSpace($url)){$url=[string]$cfg.ManifestUrl}
    if(-not (Valid-Https $url)){throw 'Update manifest URL must be HTTPS.'}
    $req=[Net.HttpWebRequest][Net.WebRequest]::Create($url)
    $req.Timeout=12000; $req.ReadWriteTimeout=12000
    $req.UserAgent='DawnForgeForeverLauncher/0.2'
    $response=$req.GetResponse()
    if($response.ResponseUri.Scheme -ne 'https') { $response.Dispose(); throw 'Manifest redirected away from HTTPS.' }
    try {
        $stream=$response.GetResponseStream()
        $reader=New-Object IO.StreamReader($stream)
        $raw=$reader.ReadToEnd()
        if($raw.Length -gt 4194304){throw 'Manifest exceeds 4 MB.'}
        $obj=$raw | ConvertFrom-Json
    } finally {if($reader){$reader.Dispose()};$response.Dispose()}
    if($obj.schemaVersion -ne 1){throw 'Unsupported manifest schema.'}
    return $obj
}
function Safe-Target([string]$base,[string]$relative) {
    if([string]::IsNullOrWhiteSpace($relative) -or [IO.Path]::IsPathRooted($relative) -or $relative -match '(^|[\\/])\.\.([\\/]|$)|[:*?"<>|]') {throw "Unsafe file path in manifest: $relative"}
    $fullBase=[IO.Path]::GetFullPath($base).TrimEnd('\')+'\'
    $full=[IO.Path]::GetFullPath((Join-Path $fullBase $relative))
    if(-not $full.StartsWith($fullBase,[StringComparison]::OrdinalIgnoreCase)){throw 'Manifest attempted to escape the selected client directory.'}
    return $full
}
function Fetch-VerifiedFile([string]$url,[string]$sha,[string]$target,[Int64]$maxBytes=85899345920,[Int64]$expectedSize=-1) {
    if(-not (Valid-Https $url)){throw "Untrusted download address (HTTPS required): $url"}
    if($sha -notmatch '^[A-Fa-f0-9]{64}$'){throw 'SHA-256 missing or invalid in manifest.'}
    if($expectedSize -gt $maxBytes){throw 'File exceeds maximum permitted size.'}
    if(Test-Path -LiteralPath $target) {
        if($expectedSize -lt 0 -or ([IO.FileInfo]$target).Length -eq $expectedSize) {
            if((Get-FileHash -LiteralPath $target -Algorithm SHA256).Hash -ieq $sha){return $false}
        }
    }
    $dir=Split-Path -Parent $target
    [IO.Directory]::CreateDirectory($dir) | Out-Null
    $part="$target.dfpart"
    [Int64]$offset=0
    if(Test-Path -LiteralPath $part){$offset=([IO.FileInfo]$part).Length}
    if($offset -gt $maxBytes -or ($expectedSize -ge 0 -and $offset -gt $expectedSize)) {
        Remove-Item -LiteralPath $part -Force
        $offset=0
    }
    $response=$null
    for($attempt=0;$attempt -lt 2;$attempt++){
        $req=[Net.HttpWebRequest][Net.WebRequest]::Create($url)
        $req.Timeout=20000
        $req.ReadWriteTimeout=30000
        $req.UserAgent='DawnForgeForeverLauncher/0.2.4'
        $req.AutomaticDecompression=[Net.DecompressionMethods]::None
        if($offset -gt 0){$req.AddRange($offset)}
        try {
            $response=$req.GetResponse()
        } catch {
            if($offset -gt 0){
                Remove-Item -LiteralPath $part -Force -ErrorAction SilentlyContinue
                $offset=0
                continue
            }
            throw
        }
        if($response.ResponseUri.Scheme -ne 'https') {
            $response.Dispose()
            throw 'Download redirected away from HTTPS.'
        }
        if($offset -gt 0) {
            $range=[string]$response.Headers['Content-Range']
            if($response.StatusCode -ne [Net.HttpStatusCode]::PartialContent -or
               -not $range.StartsWith("bytes $offset-",[StringComparison]::OrdinalIgnoreCase)) {
                $response.Dispose()
                $response=$null
                Remove-Item -LiteralPath $part -Force
                $offset=0
                continue
            }
        }
        break
    }
    if($null -eq $response){throw 'Unable to resume or restart file download.'}
    $stream=$null
    $out=$null
    try {
        if($response.ContentLength -ge 0 -and $offset+$response.ContentLength -gt $maxBytes){
            throw 'File exceeds maximum permitted size.'
        }
        $stream=$response.GetResponseStream()
        $mode=if($offset -gt 0){[IO.FileMode]::Append}else{[IO.FileMode]::Create}
        $out=[IO.File]::Open($part,$mode,[IO.FileAccess]::Write,[IO.FileShare]::None)
        $buffer=New-Object byte[] 1048576
        [Int64]$total=$offset
        [Int64]$denominator=if($expectedSize -ge 0){$expectedSize}else{
            if($response.ContentLength -ge 0){$offset+$response.ContentLength}else{0}
        }
        while(($n=$stream.Read($buffer,0,$buffer.Length)) -gt 0) {
            $total+=$n
            if($total -gt $maxBytes){throw 'Download exceeded permitted size.'}
            $out.Write($buffer,0,$n)
            if($denominator -gt 0) {
                Set-Status ('Downloading '+[IO.Path]::GetFileName($target)+' - '+[Math]::Round($total/1MB,1)+' MB') ([int]([Math]::Min(100,100*$total/$denominator)))
            }
        }
    } finally {
        if($out){$out.Dispose()}
        if($stream){$stream.Dispose()}
        $response.Dispose()
    }
    if($expectedSize -ge 0 -and ([IO.FileInfo]$part).Length -ne $expectedSize) {
        throw "Download incomplete: $target (partial file kept for resume)"
    }
    $got=(Get-FileHash -LiteralPath $part -Algorithm SHA256).Hash
    if($got -ine $sha) {
        Remove-Item -LiteralPath $part -Force
        throw "SHA-256 mismatch: $target"
    }
    Move-Item -LiteralPath $part -Destination $target -Force
    return $true
}
function Ensure-GameFolder {
    if($cfg.GameDir -and (Test-Path -LiteralPath $cfg.GameDir -PathType Container)){return $true}
    $dialog=New-Object Windows.Forms.FolderBrowserDialog
    $dialog.Description='Choose your own WoW Forever client folder. Official client files are not provided by DawnForge.'
    if($dialog.ShowDialog() -ne [System.Windows.Forms.DialogResult]::OK){return $false}
    $selected=$dialog.SelectedPath
    if((Split-Path -Leaf $selected) -eq '_forever_' -and (Test-Path -LiteralPath (Join-Path $selected 'Wow.exe'))) { $selected=Split-Path -Parent $selected }
    $cfg.GameDir=$selected
    Save-Settings
    return $true
}
function Sync-Files([bool]$repair=$false) {
    if(-not (Ensure-GameFolder)){return}
    $manifest=Get-Manifest $defaultClientManifest
    $files=@($manifest.files | Where-Object { $null -ne $_ })
    if($files.Count -gt 30000){throw 'Manifest file count exceeds safety limit.'}
    if($files.Count -eq 0) {
        Set-Status 'No downloadable files have been published yet.' 0
        Msg 'No DawnForge patches are currently published. Select your existing legitimate Forever client in Options. Full game files are not distributed by this launcher.'
        return
    }
    $pending=Join-Path ([string]$cfg.GameDir) '.dawnforge-forever-download-pending'
    [IO.File]::WriteAllText($pending,'Pending download or repair; resume via DOWNLOAD.')
    $count=0
    foreach($f in $files){
        $count++
        if(-not $f.path -or -not $f.url -or -not $f.sha256){throw 'Incomplete file entry in manifest.'}
        $dest=Safe-Target ([string]$cfg.GameDir) ([string]$f.path)
        Set-Status "Checking $count / $($files.Count): $($f.path)" 0
        $fileSize=if($null -ne $f.size){[Int64]$f.size}else{[Int64]-1}
        Fetch-VerifiedFile ([string]$f.url) ([string]$f.sha256) $dest 85899345920 $fileSize | Out-Null
    }
    Remove-Item -LiteralPath $pending -Force
    Set-Status 'Client files verified and up to date.' 100
    Refresh-PlayState
}
function Check-Server {
    if([string]::IsNullOrWhiteSpace($cfg.ServerHost)){
        $ServerTitle.Text='Gateway not configured';$ServerTitle.Foreground=([System.Windows.Media.BrushConverter]::new().ConvertFromString('#FFDBB774'));return
    }
    $client=New-Object Net.Sockets.TcpClient
    try {
        $async=$client.BeginConnect([string]$cfg.ServerHost,[int]$cfg.ServerPort,$null,$null)
        if($async.AsyncWaitHandle.WaitOne(2000,$false)){
            $client.EndConnect($async)
            $ServerTitle.Text='Gateway reachable';$ServerTitle.Foreground=([System.Windows.Media.BrushConverter]::new().ConvertFromString('#FF76E789'))
        } else {
            $ServerTitle.Text='Gateway unavailable';$ServerTitle.Foreground=([System.Windows.Media.BrushConverter]::new().ConvertFromString('#FFFFB570'))
        }
    } catch { $ServerTitle.Text='Gateway unavailable';$ServerTitle.Foreground=([System.Windows.Media.BrushConverter]::new().ConvertFromString('#FFFFB570')) }
    finally {$client.Close()}
}
function Update-Launcher {
    $manifest=Get-Manifest
    $remote=$manifest.launcher
    if(-not $remote -or -not $remote.version -or -not $remote.url -or -not $remote.sha256){return $false}
    $current=[Version]'0.2.4';$latest=[Version]([string]$remote.version)
    if($latest -le $current){return $false}
    $res=[System.Windows.MessageBox]::Show("A newer launcher ($latest) is available. Install verified update now?",'Launcher update',[System.Windows.MessageBoxButton]::YesNo,[System.Windows.MessageBoxImage]::Question)
    if($res -ne 'Yes'){return $true}
    $next="$LauncherExe.dfnew"
    Fetch-VerifiedFile ([string]$remote.url) ([string]$remote.sha256) $next 104857600 | Out-Null
    $updater=Join-Path $AppDirectory 'apply-update.ps1'
    $script=@'
param([string]$OldExe,[string]$NewExe)
for($i=0;$i -lt 40;$i++) {
    try {
        Copy-Item -LiteralPath $NewExe -Destination $OldExe -Force -ErrorAction Stop
        Remove-Item -LiteralPath $NewExe -Force -ErrorAction SilentlyContinue
        Start-Process -FilePath $OldExe
        exit
    } catch {Start-Sleep -Milliseconds 500}
}
'@
    [IO.File]::WriteAllText($updater,$script)
    Start-Process powershell.exe -ArgumentList @('-NoProfile','-ExecutionPolicy','Bypass','-WindowStyle','Hidden','-File',('"'+$updater+'"'),('"'+$LauncherExe+'"'),('"'+$next+'"')) -WindowStyle Hidden
    $window.Close()
    return $true
}
function Client-Installed {
    if([string]::IsNullOrWhiteSpace($cfg.GameDir)){return $false}
    $gameRoot=[string]$cfg.GameDir
    if(Test-Path -LiteralPath (Join-Path $gameRoot '.dawnforge-forever-download-pending')){return $false}
    return (Test-Path -LiteralPath (Join-Path $gameRoot '_forever_\Wow.exe') -PathType Leaf) -and
           (Test-Path -LiteralPath (Join-Path $gameRoot 'Data') -PathType Container)
}
function Refresh-PlayState {
    if(Client-Installed) {
        $window.FindName('Play').Content='PLAY'
    } else {
        $window.FindName('Play').Content='DOWNLOAD'
    }
}
function Play-Game {
    if(-not (Ensure-GameFolder)){return}
    if(-not (Client-Installed)) {
        Sync-Files
        if(-not (Client-Installed)) {
            Refresh-PlayState
            return
        }
    }
    $connector=[string]$cfg.ConnectorExe
    if([string]::IsNullOrWhiteSpace($connector)){
        $bundled=Join-Path $AppDirectory 'DawnForgeForeverConnector.exe'
        if(Test-Path -LiteralPath $bundled -PathType Leaf){$connector=$bundled}
    }
    if([string]::IsNullOrWhiteSpace($connector)){
        Err 'The client is installed but the compatible Forever connection helper is not available yet. The game cannot be launched against this realm safely.'
        Set-Status 'Client installed; private-realm connection is not configured.' 100
        return
    }
    if(-not [IO.Path]::IsPathRooted($connector)){
        $connector=Safe-Target ([string]$cfg.GameDir) $connector
    }
    if(-not (Test-Path -LiteralPath $connector -PathType Leaf)){Err "Connection helper not found: $connector";return}
    Start-Process -FilePath $connector -WorkingDirectory (Split-Path -Parent $connector)
    Set-Status 'Started compatible Forever connection helper.' 100
}
function Show-Options {
    # Public options deliberately expose only the user's local game folder.
    # Server routing, website and update endpoints are managed by the release configuration.
    $xaml=@'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation" Title="DawnForge Forever - Options" Width="720" Height="222" Background="#0C1829" Foreground="#FFF0D8A7" WindowStartupLocation="CenterOwner" ResizeMode="NoResize">
 <Grid Margin="22">
  <Grid.RowDefinitions><RowDefinition Height="58"/><RowDefinition Height="*"/></Grid.RowDefinitions>
  <TextBlock Grid.Row="0" Text="Game folder" VerticalAlignment="Center" FontSize="16"/>
  <TextBox Grid.Row="0" Name="Game" Margin="135,9,116,9" VerticalContentAlignment="Center" FontSize="14" Background="#15263B" Foreground="White" BorderBrush="#A88A4E"/>
  <Button Grid.Row="0" Name="Browse" Content="BROWSE" Width="104" Height="39" HorizontalAlignment="Right" VerticalAlignment="Center" Background="#173B65" BorderBrush="#A88A4E" Foreground="White" Cursor="Hand"/>
  <TextBlock Grid.Row="1" Text="Choose the folder containing your existing WoW Forever game." FontSize="13" Foreground="#B8C6D5" VerticalAlignment="Top" Margin="0,8,0,0"/>
  <Button Grid.Row="1" Name="Save" Content="SAVE SETTINGS" Height="40" Width="180" HorizontalAlignment="Right" VerticalAlignment="Bottom" Background="#173B65" BorderBrush="#A88A4E" Foreground="White" Cursor="Hand"/>
 </Grid>
</Window>
'@
    $o=[System.Windows.Markup.XamlReader]::Parse($xaml);$o.Owner=$window
    $game=$o.FindName('Game')
    $game.Text=[string]$cfg.GameDir
    $o.FindName('Browse').Add_Click({
        $picker=New-Object Windows.Forms.FolderBrowserDialog
        $picker.Description='Select your existing WoW Forever game folder'
        if($game.Text -and (Test-Path -LiteralPath $game.Text -PathType Container)){$picker.SelectedPath=$game.Text}
        try {
            if($picker.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){$game.Text=$picker.SelectedPath}
        } finally {$picker.Dispose()}
    })
    $o.FindName('Save').Add_Click({
        $chosen=$game.Text.Trim()
        if($chosen -and -not (Test-Path -LiteralPath $chosen -PathType Container)){
            Err 'The selected game folder does not exist.'
            return
        }
        if((Split-Path -Leaf $chosen) -eq '_forever_' -and (Test-Path -LiteralPath (Join-Path $chosen 'Wow.exe'))) { $chosen=Split-Path -Parent $chosen }
        $cfg.GameDir=$chosen
        Save-Settings
        $o.DialogResult=$true
        $o.Close()
    })
    $o.ShowDialog() | Out-Null
    Check-Server
    Refresh-PlayState
}
$xaml=@'
<Window xmlns="http://schemas.microsoft.com/winfx/2006/xaml/presentation" xmlns:x="http://schemas.microsoft.com/winfx/2006/xaml" Title="DawnForge Forever Launcher" Width="1229" Height="819" MinWidth="900" MinHeight="590" Background="#060C16" WindowStyle="None" ResizeMode="CanResizeWithGrip" WindowStartupLocation="CenterScreen" ShowInTaskbar="True">
 <Grid>
 <Viewbox Stretch="Uniform" StretchDirection="Both"><Canvas Width="1536" Height="1024" Background="#081020">
 <Image x:Name="Art" Width="1536" Height="1024" Canvas.Left="0" Canvas.Top="0" Stretch="Fill"/>
 <Border Canvas.Left="1046" Canvas.Top="501" Width="465" Height="264" BorderBrush="#A88A4E" BorderThickness="2" CornerRadius="9" Background="#FF081524">
  <StackPanel Margin="28,16"><TextBlock Text="DAWNFORGE FOREVER" FontWeight="Bold" FontFamily="Georgia" FontSize="25" Foreground="#FFE4B769"/><TextBlock x:Name="ServerTitle" Text="Checking gateway..." Foreground="#FFDBB774" FontSize="25" Margin="0,14,0,12"/><TextBlock Text="Website: dawnforge.eu" Foreground="#FFCFD7E1" FontSize="19"/><TextBlock Text="One permanent realm" Foreground="#FFAFB5BF" FontSize="18" Margin="0,12,0,0"/></StackPanel>
 </Border>
 <Border Canvas.Left="35" Canvas.Top="768" Width="998" Height="135" Background="#FF081524" CornerRadius="8" BorderBrush="#806A4B" BorderThickness="2">
  <Grid Margin="24,20"><Grid.RowDefinitions><RowDefinition Height="*"/><RowDefinition Height="26"/></Grid.RowDefinitions><TextBlock x:Name="Status" Text="Select your existing WoW Forever game folder in Options." Foreground="#FFEBD3AA" FontSize="22" VerticalAlignment="Center" TextWrapping="Wrap"/><ProgressBar x:Name="Progress" Grid.Row="1" Maximum="100" Height="18" Background="#FF0A1320" Foreground="#FF40D36D" Value="0" /></Grid>
 </Border>
 <Button x:Name="Home" Canvas.Left="53" Canvas.Top="421" Width="298" Height="75" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Website" Canvas.Left="358" Canvas.Top="421" Width="280" Height="75" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Downloads" Canvas.Left="649" Canvas.Top="421" Width="276" Height="75" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Updates" Canvas.Left="929" Canvas.Top="421" Width="279" Height="75" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Options" Canvas.Left="1214" Canvas.Top="421" Width="280" Height="75" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Play" Canvas.Left="1065" Canvas.Top="773" Width="434" Height="125" Background="#FF12365B" BorderBrush="#FFD2AA61" BorderThickness="2" Foreground="#FFFFE7B7" FontSize="41" FontFamily="Georgia" FontWeight="Bold" Content="DOWNLOAD" Cursor="Hand"/>
 <Button x:Name="Close" Canvas.Left="1482" Canvas.Top="0" Width="54" Height="50" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Minimize" Canvas.Left="1355" Canvas.Top="0" Width="55" Height="50" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 <Button x:Name="Maximize" Canvas.Left="1422" Canvas.Top="0" Width="55" Height="50" Background="Transparent" BorderThickness="0" Cursor="Hand"/>
 
 </Canvas></Viewbox>
 </Grid>
</Window>
'@
$window=[System.Windows.Markup.XamlReader]::Parse($xaml)
$Art=$window.FindName('Art');$StatusText=$window.FindName('Status');$Progress=$window.FindName('Progress');$ServerTitle=$window.FindName('ServerTitle')
$img=Join-Path $AppDirectory 'artwork.jpg'
if(Test-Path -LiteralPath $img){
    $bitmap=New-Object System.Windows.Media.Imaging.BitmapImage
    $bitmap.BeginInit();$bitmap.UriSource=[Uri]::new($img);$bitmap.CacheOption=[System.Windows.Media.Imaging.BitmapCacheOption]::OnLoad;$bitmap.EndInit();$bitmap.Freeze()
    $Art.Source=$bitmap
}
$window.FindName('Close').Add_Click({$window.Close()})
$window.FindName('Minimize').Add_Click({$window.WindowState=[System.Windows.WindowState]::Minimized})
$window.FindName('Maximize').Add_Click({if($window.WindowState -eq 'Maximized'){$window.WindowState='Normal'}else{$window.WindowState='Maximized'}})
$window.Add_MouseLeftButtonDown({if([System.Windows.Input.Mouse]::GetPosition($window).Y -lt 48 -and $_.ChangedButton -eq [System.Windows.Input.MouseButton]::Left){$window.DragMove()}})
$window.FindName('Home').Add_Click({Check-Server;Set-Status 'Ready. Choose your existing client in Options, then check updates.' 0})
$window.FindName('Website').Add_Click({if(Valid-Https $cfg.SiteUrl){Start-Process ([string]$cfg.SiteUrl)}else{Err 'Website URL is invalid.'}})
$window.FindName('Options').Add_Click({try{Show-Options}catch{Err $_.Exception.Message}})
$window.FindName('Downloads').Add_Click({try{Sync-Files}catch{Set-Status 'Download or repair failed.' 0;Err $_.Exception.Message}})
$window.FindName('Updates').Add_Click({try{if(-not (Update-Launcher)){Sync-Files}}catch{Set-Status 'Update check failed.' 0;Err $_.Exception.Message}})
$window.FindName('Play').Add_Click({try{Play-Game}catch{Err $_.Exception.Message}})
$window.Add_ContentRendered({Check-Server;Refresh-PlayState})
$window.ShowDialog() | Out-Null
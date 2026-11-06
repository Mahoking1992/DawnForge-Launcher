param([string]$Launcher)
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
[System.Windows.Forms.Application]::EnableVisualStyles()
$f=New-Object System.Windows.Forms.Form
$f.Text='DawnForge Graphics Manager'
$f.Size=New-Object System.Drawing.Size(440,285)
$f.StartPosition='CenterScreen'
$f.FormBorderStyle='FixedDialog'
$f.MaximizeBox=$false
$lbl=New-Object System.Windows.Forms.Label
$lbl.Text='Choose the Vanilla WoW 1.12.1 folder:'
$lbl.Location=New-Object System.Drawing.Point(18,20)
$lbl.AutoSize=$true
$f.Controls.Add($lbl)
$path=New-Object System.Windows.Forms.TextBox
$path.Location=New-Object System.Drawing.Point(18,47)
$path.Size=New-Object System.Drawing.Size(300,24)
$settings=Join-Path $env:LOCALAPPDATA 'DawnForge\Launcher\hd-settings.json'
if(Test-Path -LiteralPath $settings){ try{$path.Text=(Get-Content -Raw -LiteralPath $settings|ConvertFrom-Json).game_dir}catch{} }
$f.Controls.Add($path)
$browse=New-Object System.Windows.Forms.Button
$browse.Text='Browse'
$browse.Location=New-Object System.Drawing.Point(325,45)
$browse.Size=New-Object System.Drawing.Size(83,27)
$browse.Add_Click({
 $fd=New-Object System.Windows.Forms.FolderBrowserDialog
 if([System.IO.Directory]::Exists($path.Text)){$fd.SelectedPath=$path.Text}
 if($fd.ShowDialog() -eq [System.Windows.Forms.DialogResult]::OK){$path.Text=$fd.SelectedPath}
})
$f.Controls.Add($browse)
$status=New-Object System.Windows.Forms.Label
$status.Text='HD installation requires a published R2 manifest and a standard WoW client.'
$status.Location=New-Object System.Drawing.Point(18,88)
$status.Size=New-Object System.Drawing.Size(390,42)
$f.Controls.Add($status)
$hd=New-Object System.Windows.Forms.Button
$hd.Text='ENABLE / DOWNLOAD HD'
$hd.Location=New-Object System.Drawing.Point(18,145)
$hd.Size=New-Object System.Drawing.Size(190,48)
$f.Controls.Add($hd)
$classic=New-Object System.Windows.Forms.Button
$classic.Text='VANILLA GRAPHICS'
$classic.Location=New-Object System.Drawing.Point(218,145)
$classic.Size=New-Object System.Drawing.Size(190,48)
$f.Controls.Add($classic)
$run={
 param($mode)
 $folder=$path.Text.Trim()
 if(!(Test-Path -LiteralPath (Join-Path $folder 'Data'))){
  [System.Windows.Forms.MessageBox]::Show('Choose the folder containing WoW.exe and Data.', 'DawnForge')|Out-Null
  return
 }
 $save=Start-Process -FilePath $Launcher -ArgumentList @('--hd-set-game', ('"'+$folder+'"')) -Wait -PassThru
 if($save.ExitCode -ne 0){$status.Text='Could not save game path';return}
 $status.Text='Operation started in a new terminal; keep it open until completion.'
 Start-Process -FilePath 'powershell.exe' -ArgumentList @('-NoExit','-Command', ('& "'+$Launcher+'" '+$mode+'; Read-Host "Press Enter to close"')) -WorkingDirectory (Split-Path $Launcher)
}
$hd.Add_Click({& $run '--hd-install'})
$classic.Add_Click({& $run '--vanilla-mode'})
[void]$f.ShowDialog()

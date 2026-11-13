param([switch]$VerifyTls)
$ErrorActionPreference='Stop'
$gateway='play.dawnforge.eu'
$portal='trinity.actual.battle.net'
$failed=$false
foreach($port in @(1119,8081,8090)) {
    $tcp=New-Object Net.Sockets.TcpClient
    try {
        $pending=$tcp.BeginConnect($gateway,$port,$null,$null)
        if(-not $pending.AsyncWaitHandle.WaitOne(5000)){throw 'connection timed out'}
        $tcp.EndConnect($pending)
        Write-Host "PASS TCP ${gateway}:$port"
    } catch {
        $failed=$true
        Write-Host "FAIL TCP ${gateway}:$port - $($_.Exception.Message)"
    } finally {$tcp.Dispose()}
}
if($VerifyTls) {
    foreach($port in @(1119,8081)) {
        $tcp=New-Object Net.Sockets.TcpClient
        $tls=$null
        try {
            $pending=$tcp.BeginConnect($gateway,$port,$null,$null)
            if(-not $pending.AsyncWaitHandle.WaitOne(5000)){throw 'connection timed out'}
            $tcp.EndConnect($pending)
            $tcp.ReceiveTimeout=5000;$tcp.SendTimeout=5000
            # Default Windows certificate chain and hostname validation; no bypass callback.
            $tls=New-Object Net.Security.SslStream($tcp.GetStream(),$false)
            $tls.AuthenticateAsClient($portal)
            Write-Host "PASS TLS ${portal}:$port - $($tls.SslProtocol)"
        } catch {
            $failed=$true
            Write-Host "FAIL TLS ${portal}:$port - $($_.Exception.Message)"
        } finally {if($tls){$tls.Dispose()};$tcp.Dispose()}
    }
}
if($failed){exit 1}
exit 0

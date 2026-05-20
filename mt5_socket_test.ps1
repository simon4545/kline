param(
  [string]$HostName = "127.0.0.1",
  [int]$Port = 18555,
  [string]$Cmd = "PING"
)

$ErrorActionPreference = "Stop"

function Send-Line {
  param(
    [System.Net.Sockets.TcpClient]$Client,
    [string]$Line
  )

  $stream = $Client.GetStream()
  $reader = New-Object System.IO.StreamReader($stream, [System.Text.Encoding]::ASCII)

  $payload = [System.Text.Encoding]::ASCII.GetBytes($Line + "`n")

  Write-Host "--> $Line"
  Write-Host ("--> bytes: " + (($payload | ForEach-Object { $_.ToString("X2") }) -join " "))

  # 避免对端第一次 recv 读到不完整数据，先小延迟再一次性写入
  Start-Sleep -Milliseconds 100
  $stream.Write($payload, 0, $payload.Length)
  $stream.Flush()

  $stream.ReadTimeout = 3000
  try {
    $resp = $reader.ReadLine()
    if ($null -ne $resp) {
      Write-Host "<-- $resp"
    } else {
      Write-Host "<-- (no response)"
    }
  }
  catch {
    Write-Host "<-- (timeout/no response)"
  }
}

$client = New-Object System.Net.Sockets.TcpClient
try {
  Write-Host "Connecting to $HostName`:$Port ..."
  $client.Connect($HostName, $Port)
  Write-Host "Connected"

  switch ($Cmd.ToUpperInvariant()) {
    "PING" {
      Send-Line -Client $client -Line "PING"
    }
    "BUY" {
      Send-Line -Client $client -Line "BUY,XAUUSD.c,0.01,0,0,ps-test-buy"
    }
    "SELL" {
      Send-Line -Client $client -Line "SELL,XAUUSD.c,0.01,0,0,ps-test-sell"
    }
    "CLOSE" {
      Send-Line -Client $client -Line "CLOSE,XAUUSD.c,0,0,0,ps-test-close"
    }
    default {
      # 允许自定义整行命令
      Send-Line -Client $client -Line $Cmd
    }
  }
}
finally {
  if ($client -and $client.Connected) {
    $client.Close()
    Write-Host "Disconnected"
  }
}

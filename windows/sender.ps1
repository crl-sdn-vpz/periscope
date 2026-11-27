# Periscope sender - streams one monitor of device 1 to the Linux viewer over encrypted SRT.
# Runs headless: no window, no tray icon, no notifications, no capture border.
# Compatible with Windows PowerShell 5.1 and PowerShell 7.

$ErrorActionPreference = "Stop"
$here = Split-Path -Parent $MyInvocation.MyCommand.Path
. (Join-Path $here "periscope.config.ps1")

$stateDir = Join-Path $env:LOCALAPPDATA "periscope"
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
$pidFile  = Join-Path $stateDir "sender.pid"
$stopFile = Join-Path $stateDir "stop"
$logFile  = Join-Path $stateDir "sender.log"
$ffLog    = Join-Path $stateDir "ffmpeg.log"
$ffOut    = Join-Path $stateDir "ffmpeg.out"

# Keep logs small: rotate sender.log at 1 MB (one previous copy kept).
if ((Test-Path $logFile) -and ((Get-Item $logFile).Length -gt 1MB)) {
    Move-Item -Force $logFile "$logFile.1"
}

function Write-Log([string]$msg) {
    $line = "{0:yyyy-MM-dd HH:mm:ss}  {1}" -f (Get-Date), $msg
    Add-Content -Path $logFile -Value $line
}

# --- single instance --------------------------------------------------------
$created = $false
$mutex = New-Object System.Threading.Mutex($true, "Local\PeriscopeSender", [ref]$created)
if (-not $created) { Write-Log "Another sender is already running; exiting."; exit 0 }
Set-Content -Path $pidFile -Value $PID
Remove-Item -Path $stopFile -ErrorAction SilentlyContinue

# --- locate ffmpeg ----------------------------------------------------------
function Resolve-Ffmpeg {
    $p = $Config.FfmpegPath
    if ($p -and (Test-Path $p)) { return (Resolve-Path $p).Path }
    $cmd = Get-Command $p -ErrorAction SilentlyContinue
    if ($cmd) { return $cmd.Source }
    $candidates = @(
        (Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Links\ffmpeg.exe"),
        "C:\ffmpeg\bin\ffmpeg.exe",
        (Join-Path $env:ProgramFiles "ffmpeg\bin\ffmpeg.exe")
    )
    $wingetPkgs = Join-Path $env:LOCALAPPDATA "Microsoft\WinGet\Packages"
    if (Test-Path $wingetPkgs) {
        $found = Get-ChildItem -Path $wingetPkgs -Recurse -Filter ffmpeg.exe -ErrorAction SilentlyContinue | Select-Object -First 1
        if ($found) { $candidates += $found.FullName }
    }
    foreach ($c in $candidates) { if ($c -and (Test-Path $c)) { return $c } }
    throw "ffmpeg.exe not found. Install it with: winget install Gyan.FFmpeg  (or set FfmpegPath in periscope.config.ps1)"
}

# --- build ffmpeg arguments -------------------------------------------------
function Build-Args {
    $fps = [int]$Config.Fps
    $gop = $fps * 2
    $q   = [int]$Config.Quality
    $enc = $Config.Encoder.ToLower()

    $use444 = $Config.Chroma444 -and ($enc -eq "x264" -or $enc -eq "nvenc")
    if ($use444) { $pix = "yuv444p" } elseif ($enc -eq "x264") { $pix = "yuv420p" } else { $pix = "nv12" }

    $a = @("-hide_banner", "-loglevel", "warning", "-nostdin")

    if ($Config.Capture -eq "gdigrab") {
        $a += @("-f", "gdigrab", "-framerate", "$fps", "-draw_mouse", "1", "-i", "desktop")
    } else {
        $src = "ddagrab=output_idx=$($Config.Monitor):framerate=$($fps):draw_mouse=1,hwdownload,format=bgra"
        $a += @("-f", "lavfi", "-i", "`"$src`"")
    }

    $vf = @()
    if ([int]$Config.MaxWidth -gt 0) { $vf += "scale=w='min($($Config.MaxWidth),iw)':h=-2:flags=lanczos" }
    $vf += "format=$pix"
    $a += @("-vf", "`"$($vf -join ',')`"", "-an")

    switch ($enc) {
        "nvenc" {
            $a += @("-c:v", "h264_nvenc", "-preset", "p1", "-tune", "ull", "-zerolatency", "1",
                    "-rc", "constqp", "-qp", "$q", "-g", "$gop", "-bf", "0")
            if ($use444) { $a += @("-profile:v", "high444p") }
        }
        "qsv" {
            $a += @("-c:v", "h264_qsv", "-preset", "veryfast", "-global_quality", "$q",
                    "-look_ahead", "0", "-async_depth", "1", "-g", "$gop", "-bf", "0")
        }
        "amf" {
            $a += @("-c:v", "h264_amf", "-usage", "ultralowlatency", "-rc", "cqp",
                    "-qp_i", "$q", "-qp_p", "$q", "-g", "$gop", "-bf", "0")
        }
        default {
            $a += @("-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
                    "-crf", "$q", "-g", "$gop", "-bf", "0")
        }
    }

    $latUs = [int]$Config.LatencyMs * 1000
    $pass  = [uri]::EscapeDataString($Config.Passphrase)
    $url   = "srt://$($Config.TargetHost):$($Config.Port)?mode=caller&latency=$latUs&passphrase=$pass&pbkeylen=16&connect_timeout=3000"
    $a += @("-f", "mpegts", "-flush_packets", "1", "-muxdelay", "0",
            "-metadata", "service_name=periscope", "`"$url`"")
    return $a
}

if ($Config.Passphrase.Length -lt 10 -or $Config.Passphrase.Length -gt 79) {
    Write-Log "Passphrase must be 10-79 characters."; exit 1
}

$ffmpeg  = Resolve-Ffmpeg
$argLine = (Build-Args) -join " "
Write-Log "Sender started (pid $PID). ffmpeg: $ffmpeg"
Write-Log ("Target srt://{0}:{1}  monitor={2} fps={3} encoder={4}" -f $Config.TargetHost, $Config.Port, $Config.Monitor, $Config.Fps, $Config.Encoder)

# --- run loop: reconnects automatically when the viewer isn't up yet or drops --
$backoff = 2
try {
    while (-not (Test-Path $stopFile)) {
        $started = Get-Date
        $proc = Start-Process -FilePath $ffmpeg -ArgumentList $argLine -NoNewWindow -PassThru `
                              -RedirectStandardError $ffLog -RedirectStandardOutput $ffOut
        $proc.WaitForExit()
        if (Test-Path $stopFile) { break }
        $ran = ((Get-Date) - $started).TotalSeconds
        if ($ran -gt 20) { $backoff = 2 } else { $backoff = [Math]::Min($backoff * 2, 15) }
        $tail = (Get-Content $ffLog -Tail 2 -ErrorAction SilentlyContinue) -join " | "
        Write-Log ("ffmpeg exited (code {0}, ran {1:N0}s). Retrying in {2}s. {3}" -f $proc.ExitCode, $ran, $backoff, $tail)
        Start-Sleep -Seconds $backoff
    }
} finally {
    Write-Log "Sender stopped."
    Remove-Item -Path $pidFile -ErrorAction SilentlyContinue
    $mutex.ReleaseMutex()
}

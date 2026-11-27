# Periscope sender config (device 1 / Windows). Edit, then run start.cmd.
$Config = @{
    # Linux viewer (device n) LAN IP. Give it a fixed IP / DHCP reservation.
    TargetHost = "192.168.1.50"
    Port       = 9000

    # Must match PASSPHRASE in ~/.config/periscope/config on device n (10-79 chars).
    Passphrase = "change-me-to-a-long-random-string"

    # Which monitor of device 1 to capture (0 = primary).
    Monitor    = 0
    Fps        = 30

    # x264 (CPU, best text quality) | nvenc (NVIDIA) | qsv (Intel) | amf (AMD)
    Encoder    = "x264"
    # Lower = sharper. 16-20 keeps code/terminal text crisp for screenshots.
    Quality    = 18
    # 4:4:4 chroma keeps colored text sharp (x264 / nvenc only). Costs ~30% more bandwidth.
    Chroma444  = $true
    # 0 = native resolution. Set e.g. 2560 to downscale a 4K monitor.
    MaxWidth   = 0

    # SRT buffer. 60-120 ms on wired LAN; raise to 200+ on Wi-Fi if you see glitches.
    LatencyMs  = 80

    # ddagrab (Desktop Duplication, fast, per-monitor) | gdigrab (fallback, whole desktop)
    Capture    = "ddagrab"

    # "ffmpeg" if it's on PATH (winget install Gyan.FFmpeg), or a full path to ffmpeg.exe
    FfmpegPath = "ffmpeg"
}
